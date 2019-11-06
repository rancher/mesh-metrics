package srv

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/common/log"

	"github.com/prometheus/common/model"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"

	"github.com/golang/protobuf/jsonpb"
	"github.com/gorilla/websocket"
	"github.com/julienschmidt/httprouter"
	"github.com/linkerd/linkerd2/pkg/k8s"
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
	pbMarshaler         = jsonpb.Marshaler{EmitDefaults: true}
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

func (h *handler) handleAPIVersion(w http.ResponseWriter, req *http.Request, p httprouter.Params) {

	resp := map[string]interface{}{
		"version": apiVersions,
	}
	renderJSON(w, resp)
}

// return toplevel cAdvisor metrics
func (h *handler) handleClusterInfo(w http.ResponseWriter, req *http.Request, p httprouter.Params) {

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

}

type EdgeResp struct {
	Nodes     []Node
	Edges     []Edge
	integrity string
}

type Node struct {
	App       string                       `json:"app"`
	Version   string                       `json:"version"`
	Namespace string                       `json:"namespace"`
	Stats     map[string]model.SampleValue `json:"stats,omitempty"`
}

type Edge struct {
	FromNamespace string                       `json:"fromNamespace"`
	FromApp       string                       `json:"fromApp"`
	FromVersion   string                       `json:"fromVersion"`
	ToNamespace   string                       `json:"toNamespace"`
	ToApp         string                       `json:"toApp"`
	ToVersion     string                       `json:"toVersion"`
	Stats         map[string]model.SampleValue `json:"stats,omitempty"`
}

func (h *handler) handleAPIEdges(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
	ns := p.ByName("namespace")
	if ns == "" {
		//TODO what ns should we default to?
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
	logrus.Info("incomging resp: %+v", incomingResp)
	logrus.Info("outgoing resp: %+v", outgoingResp)

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

	NodeList := make([]Node, 0, len(EdgeList))
	appSet := make(map[string]bool, len(EdgeList))
	for _, edge := range EdgeList {
		if _, ok := appSet[edge.FromApp]; ok {
			continue
		}
		node := Node{App: edge.FromApp, Version: edge.FromVersion, Namespace: edge.FromNamespace}
		node.Stats, err = statQuery(ctx, promAPI, edge.FromApp, windowDefault, "inbound")
		if err != nil {
			err = fmt.Errorf("unable to populate node list: %v", err)
			logrus.Errorf("%v", err)
			renderJSONError(w, err, http.StatusInternalServerError)
		}
		NodeList = append(NodeList, node)
		appSet[edge.FromApp] = true
	}

	resp := EdgeResp{
		Nodes:     NodeList,
		Edges:     EdgeList,
		integrity: "full",
	}
	renderJSON(w, resp)

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
		if clientID, ok := sample.Metric[model.LabelName("client_id")]; ok {
			dstResource := string(sample.Metric[model.LabelName(resourceReplacementInbound)])

			// format of clientId is id.namespace.serviceaccount.cluster.local
			clientIDSlice := strings.Split(string(clientID), ".")
			srcNs := clientIDSlice[1]
			key := model.LabelValue(fmt.Sprintf("%s.%s", dstResource, srcNs))
			dstIndex[key] = sample.Metric
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

			stats, err := statQuery(ctx, promAPI, edge.FromApp, windowDefault, "outbound")
			if err != nil {
				return nil, err
			}
			edge.Stats = stats
			edges = append(edges, edge)
		}
	}

	return edges, nil
}
