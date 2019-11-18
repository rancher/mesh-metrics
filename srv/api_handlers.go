package srv

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"

	"github.com/gorilla/mux"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/sirupsen/logrus"
)

var apiVersions = [1]string{"v0"}

type (
	jsonError struct {
		Error string `json:"error"`
	}
)

func renderJSONError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json")
	logrus.Error(err.Error())
	rsp, _ := json.Marshal(jsonError{Error: err.Error()})
	w.WriteHeader(status)
	w.Write(rsp)
}

func renderJSON(w http.ResponseWriter, resp interface{}) {
	w.Header().Set("Content-Type", "application/json")
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		renderJSONError(w, err, http.StatusInternalServerError)
		return
	}
	w.Write(jsonResp)
}

func (h *handler) handleAPIVersion() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		resp := map[string]interface{}{
			"version": apiVersions,
		}
		renderJSON(w, resp)
	})
}

// return toplevel cAdvisor metrics
func (h *handler) handleClusterInfo() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		resp := make(map[string]model.Value)
		queryValues := req.URL.Query()
		window := queryValues.Get("window")
		if window == "" {
			window = windowDefault
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result, warnings, err := h.promAPI.Query(ctx, clusterMemoryUsage, time.Now())
		if err != nil {
			logrus.Errorf("error querying prometheus: %v", err)
			renderJSONError(w, err, http.StatusInternalServerError)
			return
		}
		if len(warnings) > 0 {
			logrus.Warnf("%v", warnings)
		}
		resp["memory"] = result

		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result, warnings, err = h.promAPI.Query(ctx, clusterCPUUsage1MinAvg, time.Now())
		if err != nil {
			logrus.Errorf("error querying prometheus: %v", err)
			renderJSONError(w, err, http.StatusInternalServerError)
			return
		}
		if len(warnings) > 0 {
			logrus.Warnf("%v", warnings)
		}
		resp["cpu"] = result

		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result, warnings, err = h.promAPI.Query(ctx, clusterFilesytemUse, time.Now())
		if err != nil {
			logrus.Errorf("error querying prometheus: %v", err)
			renderJSONError(w, err, http.StatusInternalServerError)
			return
		}
		if len(warnings) > 0 {
			logrus.Warnf("%v", warnings)
		}
		resp["filesystem"] = result

		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		overallSuccessRateQuery := fmt.Sprintf(overallRespSuccessRate, window, window)
		result, warnings, err = h.promAPI.Query(ctx, overallSuccessRateQuery, time.Now())
		if err != nil {
			logrus.Errorf("error querying prometheus: %v", err)
			renderJSONError(w, err, http.StatusInternalServerError)
			return
		}
		if len(warnings) > 0 {
			logrus.Warnf("%v", warnings)
		}
		resp["overallSuccessRate"] = result

		renderJSON(w, resp)
	})

}

type EdgeResp struct {
	Nodes     []Node `json:"nodes"`
	Edges     []Edge `json:"edges,omitempty"`
	Integrity string `json:"integrity"`
}

type AppVersionNamespace string

type Node struct {
	App       string `json:"app"`
	Version   string `json:"version"`
	Namespace string `json:"namespace"`
	// use float64 to avoid custom types unmarshalling from prometheus
	Stats map[string]float64 `json:"stats"`
}

type Edge struct {
	FromNamespace string `json:"fromNamespace"`
	FromApp       string `json:"fromApp"`
	FromVersion   string `json:"fromVersion"`
	ToNamespace   string `json:"toNamespace"`
	ToApp         string `json:"toApp"`
	ToVersion     string `json:"toVersion"`
	// use float64 to avoid custom types unmarshalling from prometheus
	Stats map[string]float64 `json:"stats"`
}

func (h *handler) handleAPIEdges() http.Handler {
	return HandleEdges(h.promAPI)

}

func HandleEdges(api v1.API) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		vars := mux.Vars(req)
		ns, ok := vars["namespace"]
		if !ok {
			ns = ""
		}

		promAPI := api
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		incomingResp, warn, err := promAPI.Query(ctx, IncomingIdentityQuery, time.Now())

		if warn != nil {
			logrus.Warnf("%v", warn)
		}
		if err != nil {
			renderJSONError(w, err, http.StatusInternalServerError)
			return
		}
		outgoingResp, warn, err := promAPI.Query(ctx, OutgoingIdentityQuery, time.Now())
		if warn != nil {
			logrus.Warnf("%v", warn)
		}
		if err != nil {
			renderJSONError(w, err, http.StatusInternalServerError)
			return
		}
		logrus.Debugf("incomging resp: %+v", incomingResp)
		logrus.Debugf("outgoing resp: %+v", outgoingResp)

		if outgoingResp.Type() != model.ValVector {
			err = fmt.Errorf("Unexpected query result type (expected Vector): %s", outgoingResp.Type())
			log.Error(err)
			renderJSONError(w, err, http.StatusInternalServerError)
			return
		}
		if incomingResp.Type() != model.ValVector {
			err = fmt.Errorf("Unexpected query result type (expected Vector): %s", incomingResp.Type())
			log.Error(err)
			renderJSONError(w, err, http.StatusInternalServerError)
			return
		}
		//TODO use ErrGroup here
		EdgeList, err := processEdgeMetrics(ctx, promAPI, incomingResp.(model.Vector), outgoingResp.(model.Vector), ns)
		if err != nil {
			logrus.Errorf("%v", err)
			renderJSONError(w, err, http.StatusInternalServerError)
			return
		}
		logrus.Infof("%+v", EdgeList)

		// create Nodes based upon all seen apps
		NodeList := buildNodeList(EdgeList, ns)

		for i := range NodeList {
			NodeList[i].Stats, err = statQuery(ctx, promAPI, NodeList[i].App, NodeList[i].Version, windowDefault, "inbound")
			if err != nil {
				err = fmt.Errorf("unable to populate node list: %v", err)
				logrus.Errorf("%v", err)
				renderJSONError(w, err, http.StatusInternalServerError)
				return
			}
		}

		resp := EdgeResp{
			Nodes:     NodeList,
			Edges:     EdgeList,
			Integrity: "full",
		}
		err = checkNan(resp)
		if err != nil {
			renderJSONError(w, err, http.StatusInternalServerError)
		}
		renderJSON(w, resp)
	})

}

func (h *handler) SummaryHandler() http.Handler {
	return HandleSummary(h.promAPI)

}

func HandleSummary(api v1.API) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {

		promAPI := api
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		ch := make(chan StatResp, 2)
		go asyncStatQuery(ctx, promAPI, directionInbound, windowDefault, ch)
		go asyncStatQuery(ctx, promAPI, directionOutbound, windowDefault, ch)

		incomingResp, warn, err := promAPI.Query(ctx, IncomingIdentityQuery, time.Now())

		if warn != nil {
			logrus.Warnf("%v", warn)
		}
		if err != nil {
			renderJSONError(w, err, http.StatusInternalServerError)
			return
		}
		outgoingResp, warn, err := promAPI.Query(ctx, OutgoingIdentityQuery, time.Now())
		if warn != nil {
			logrus.Warnf("%v", warn)
		}
		if err != nil {
			renderJSONError(w, err, http.StatusInternalServerError)
			return
		}
		logrus.Debugf("incomging resp: %+v", incomingResp)
		logrus.Debugf("outgoing resp: %+v", outgoingResp)

		if outgoingResp.Type() != model.ValVector {
			err = fmt.Errorf("Unexpected query result type (expected Vector): %s", outgoingResp.Type())
			log.Error(err)
			renderJSONError(w, err, http.StatusInternalServerError)
			return
		}
		if incomingResp.Type() != model.ValVector {
			err = fmt.Errorf("Unexpected query result type (expected Vector): %s", incomingResp.Type())
			log.Error(err)
			renderJSONError(w, err, http.StatusInternalServerError)
			return
		}

		//TODO use ErrGroup here
		EdgeList, err := asyncProcessEdges(ctx, promAPI, incomingResp.(model.Vector), outgoingResp.(model.Vector), "")
		if err != nil {
			logrus.Errorf("%v", err)
			renderJSONError(w, err, http.StatusInternalServerError)
			return
		}
		logrus.Debugf("%+v", EdgeList)

		// create Nodes based upon all seen apps
		NodeList := buildNodeList(EdgeList, "")

		//for i := range NodeList {
		//	NodeList[i].Stats, err = statQuery(ctx, promAPI, NodeList[i].App, NodeList[i].Version, windowDefault, "inbound")
		//	if err != nil {
		//		err = fmt.Errorf("unable to populate node list: %v", err)
		//		logrus.Errorf("%v", err)
		//		renderJSONError(w, err, http.StatusInternalServerError)
		//		return
		//	}
		//}

		// no stats have been inserted yet
		resp := EdgeResp{
			Nodes:     NodeList,
			Edges:     EdgeList,
			Integrity: "partial",
		}
		var nodes []Node
		var edges []Edge
		for i := 0; i < 2; i++ {
			statResp := <-ch
			if statResp.err != nil {
				//TODO provide support for partial queries w/o stats
				renderJSONError(w, statResp.err, http.StatusInternalServerError)
				return
			}
			if statResp.direction == directionInbound {
				nodes, err = processInboundStats(resp.Nodes, statResp.result)
				if err != nil {
					renderJSONError(w, err, http.StatusInternalServerError)
					return
				}
			}
			if statResp.direction == directionOutbound {
				edges, err = processOutboundStats(resp.Edges, statResp.result)
				if err != nil {
					renderJSONError(w, err, http.StatusInternalServerError)
					return
				}
			}
		}

		resp = EdgeResp{Nodes: nodes, Edges: edges, Integrity: "full"}
		renderJSON(w, resp)
	})
}

// buildNodeList creates a node list from slice of edges, optionally creating a nodelist from a single namespace
// if targetNamesace is not ""
func buildNodeList(edgeList []Edge, targetNamespace string) []Node {
	NodeList := make([]Node, 0, len(edgeList))
	appSet := make(map[AppVersionNamespace]bool, len(edgeList))

	for _, edge := range edgeList {

		// check From node hasn't been seen and is in the right namespace
		if _, seen := appSet[AppVersionNamespace(edge.FromApp+edge.FromVersion+edge.FromNamespace)]; !seen {
			if targetNamespace == "" || edge.FromNamespace == targetNamespace {
				node := Node{App: edge.FromApp, Version: edge.FromVersion, Namespace: edge.FromNamespace}
				NodeList = append(NodeList, node)
				appSet[AppVersionNamespace(edge.FromApp+edge.FromVersion+edge.FromNamespace)] = true
			}
		}

		// check To node hasn't been added and is in the right namespace
		if _, seen := appSet[AppVersionNamespace(edge.ToApp+edge.ToVersion+edge.ToNamespace)]; !seen {
			if targetNamespace == "" || edge.ToNamespace == targetNamespace {
				node := Node{App: edge.ToApp, Version: edge.ToVersion, Namespace: edge.ToNamespace}
				NodeList = append(NodeList, node)
				appSet[AppVersionNamespace(edge.ToApp+edge.ToVersion+edge.ToNamespace)] = true
			}
		}

	}
	return NodeList
}
