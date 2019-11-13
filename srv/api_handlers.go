package srv

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/linkerd/linkerd2/pkg/k8s"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
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

var (
	defaultResourceType = k8s.Deployment
	maxMessageSize      = 2048
	websocketUpgrader   = websocket.Upgrader{
		ReadBufferSize:  maxMessageSize,
		WriteBufferSize: maxMessageSize,
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

		promAPI := v1.NewAPI(h.apiClient)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result, warnings, err := promAPI.Query(ctx, clusterMemoryUsage, time.Now())
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
		result, warnings, err = promAPI.Query(ctx, clusterCPUUsage1MinAvg, time.Now())
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
		result, warnings, err = promAPI.Query(ctx, clusterFilesytemUse, time.Now())
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
		result, warnings, err = promAPI.Query(ctx, overallSuccessRateQuery, time.Now())
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
	App       string             `json:"app"`
	Version   string             `json:"version"`
	Namespace string             `json:"namespace"`
	Stats     map[string]float64 `json:"stats"`
}

type Edge struct {
	FromNamespace string             `json:"fromNamespace"`
	FromApp       string             `json:"fromApp"`
	FromVersion   string             `json:"fromVersion"`
	ToNamespace   string             `json:"toNamespace"`
	ToApp         string             `json:"toApp"`
	ToVersion     string             `json:"toVersion"`
	Stats         map[string]float64 `json:"stats"`
}

func (h *handler) handleAPIEdges() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		vars := mux.Vars(req)
		ns, ok := vars["namespace"]
		if !ok {
			ns = "default"
		}

		promAPI := v1.NewAPI(h.apiClient)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		incomingResp, warn, err := promAPI.Query(ctx, IncomingIdentityQuery, time.Now())

		if warn != nil {
			logrus.Warnf("%v", warn)
		}
		if err != nil {
			renderJSONError(w, err, http.StatusInternalServerError)
		}
		outgoingResp, warn, err := promAPI.Query(ctx, OutgoingIdentityQuery, time.Now())
		if warn != nil {
			logrus.Warnf("%v", warn)
		}
		if err != nil {
			renderJSONError(w, err, http.StatusInternalServerError)
		}
		logrus.Debugf("incomging resp: %+v", incomingResp)
		logrus.Debugf("outgoing resp: %+v", outgoingResp)

		if outgoingResp.Type() != model.ValVector {
			err = fmt.Errorf("Unexpected query result type (expected Vector): %s", outgoingResp.Type())
			log.Error(err)
			panic(err)
		}
		if incomingResp.Type() != model.ValVector {
			err = fmt.Errorf("Unexpected query result type (expected Vector): %s", incomingResp.Type())
			log.Error(err)
			panic(err)
		}
		//TODO use ErrGroup here
		EdgeList, err := processEdgeMetrics(ctx, promAPI, incomingResp.(model.Vector), outgoingResp.(model.Vector), ns)
		if err != nil {
			logrus.Errorf("%v", err)
			renderJSONError(w, err, http.StatusInternalServerError)
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
			}
		}

		resp := EdgeResp{
			Nodes:     NodeList,
			Edges:     EdgeList,
			Integrity: "full",
		}
		renderJSON(w, resp)
	})

}

func HandleEdges(api v1.API) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		vars := mux.Vars(req)
		ns, ok := vars["namespace"]
		if !ok {
			ns = "default"
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
		}
		outgoingResp, warn, err := promAPI.Query(ctx, OutgoingIdentityQuery, time.Now())
		if warn != nil {
			logrus.Warnf("%v", warn)
		}
		if err != nil {
			renderJSONError(w, err, http.StatusInternalServerError)
		}
		logrus.Debugf("incomging resp: %+v", incomingResp)
		logrus.Debugf("outgoing resp: %+v", outgoingResp)

		if outgoingResp.Type() != model.ValVector {
			err = fmt.Errorf("Unexpected query result type (expected Vector): %s", outgoingResp.Type())
			log.Error(err)
			panic(err)
		}
		if incomingResp.Type() != model.ValVector {
			err = fmt.Errorf("Unexpected query result type (expected Vector): %s", incomingResp.Type())
			log.Error(err)
			panic(err)
		}
		//TODO use ErrGroup here
		EdgeList, err := processEdgeMetrics(ctx, promAPI, incomingResp.(model.Vector), outgoingResp.(model.Vector), ns)
		if err != nil {
			logrus.Errorf("%v", err)
			renderJSONError(w, err, http.StatusInternalServerError)
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
			}
		}

		resp := EdgeResp{
			Nodes:     NodeList,
			Edges:     EdgeList,
			Integrity: "full",
		}
		renderJSON(w, resp)
	})

}

func processEdgeMetrics(ctx context.Context, promAPI v1.API, inbound, outbound model.Vector, selectedNamespace string) ([]Edge, error) {
	var edges []Edge
	dstIndex := map[model.LabelValue]model.Metric{}
	srcIndex := map[model.LabelValue][]model.Metric{}
	resourceType := "deployment"
	resourceReplacementInbound := resourceType
	resourceReplacementOutbound := "dst_" + resourceType

	for _, sample := range inbound {
		// skip inbound results without a clientID because we cannot construct edge
		// information
		clientID, ok := sample.Metric[model.LabelName("client_id")]
		if ok {
			dstResource := string(sample.Metric[model.LabelName(resourceReplacementInbound)])

			// format of clientId is id.namespace.serviceaccount.cluster.local
			clientIDSlice := strings.Split(string(clientID), ".")
			srcNs := clientIDSlice[1]
			key := model.LabelValue(fmt.Sprintf("%s.%s", dstResource, srcNs))
			dstIndex[key] = sample.Metric
		} else {
			logrus.Debug("dropped metric: %s", sample.Metric)
		}
	}

	for _, sample := range outbound {
		dstResource := sample.Metric[model.LabelName(resourceReplacementOutbound)]
		srcNs := sample.Metric[model.LabelName("namespace")]

		key := model.LabelValue(fmt.Sprintf("%s.%s", dstResource, srcNs))
		if _, ok := srcIndex[key]; !ok {
			srcIndex[key] = []model.Metric{}
		}
		srcIndex[key] = append(srcIndex[key], sample.Metric)
	}

	for key, sources := range srcIndex {
		for _, src := range sources {
			srcNamespace := string(src[model.LabelName("namespace")])

			dst, ok := dstIndex[key]

			// if no destination, don't try
			if !ok {
				continue
			}

			dstNamespace := string(dst[model.LabelName("namespace")])

			// skip if selected namespace is given and neither the source nor
			// destination is in the selected namespace
			if selectedNamespace != "" && srcNamespace != selectedNamespace &&
				dstNamespace != selectedNamespace {
				continue
			}
			//TODO if fromApp would be empty we should also skip it

			edge := Edge{
				FromNamespace: srcNamespace,
				FromApp:       string(src[model.LabelName("app")]),
				FromVersion:   string(src[model.LabelName("version")]),
				ToNamespace:   dstNamespace,
				ToApp:         string(dst[model.LabelName("app")]),
				ToVersion:     string(dst[model.LabelName("version")]),
			}

			stats, err := statQuery(ctx, promAPI, edge.FromApp, edge.FromVersion, windowDefault, "outbound")
			if err != nil {
				return nil, err
			}
			edge.Stats = stats
			edges = append(edges, edge)
		}
	}

	return edges, nil
}

func buildNodeList(edgeList []Edge, targetNamespace string) []Node {
	NodeList := make([]Node, 0, len(edgeList))
	appSet := make(map[AppVersionNamespace]bool, len(edgeList))

	for _, edge := range edgeList {

		// check From node hasn't been seen and is in the right namespace
		if _, seen := appSet[AppVersionNamespace(edge.FromApp+edge.FromVersion+edge.FromNamespace)]; !seen {
			if edge.FromNamespace == targetNamespace {
				node := Node{App: edge.FromApp, Version: edge.FromVersion, Namespace: edge.FromNamespace}
				NodeList = append(NodeList, node)
				appSet[AppVersionNamespace(edge.FromApp+edge.FromVersion+edge.FromNamespace)] = true
			}
		}

		// check To node hasn't been added and is in the right namespace
		if _, seen := appSet[AppVersionNamespace(edge.ToApp+edge.ToVersion+edge.ToNamespace)]; !seen {
			if edge.ToNamespace == targetNamespace {
				node := Node{App: edge.ToApp, Version: edge.ToVersion, Namespace: edge.ToNamespace}
				NodeList = append(NodeList, node)
				appSet[AppVersionNamespace(edge.ToApp+edge.ToVersion+edge.ToNamespace)] = true
			}
		}

	}
	return NodeList
}
