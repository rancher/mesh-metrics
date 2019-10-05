package srv

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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

type promResult struct {
	prom string
	vec  model.Value
	err  error
}

func (h *handler) handleAppStats(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
	resultChan := make(chan promResult)
	quantiles := [3]string{promLatencyP50, promLatencyP95, promLatencyP99}
	promAPI := v1.NewAPI(h.apiClient)

	appName := p.ByName("app")
	if strings.Contains(appName, "app") {
		logrus.Warnf("possible bad query, app is (literal): [%s]", appName)
	}
	queryValues := req.URL.Query()
	window := queryValues.Get("window")
	if window == "" {
		window = windowDefault
	}

	queryLabels := fmt.Sprintf("{direction=\"inbound\",app=\"%s\"}", appName)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	for _, quantile := range quantiles {
		go func(quantile string) {
			latencyQuery := fmt.Sprintf(latencyQuantileQuery, quantile, queryLabels, window, "app")
			latencyResult, warnings, err := promAPI.Query(ctx, latencyQuery, time.Now())

			if warnings != nil {
				logrus.Warnf("%v", warnings)
			}

			resultChan <- promResult{
				prom: quantile,
				vec:  latencyResult,
				err:  err,
			}
		}(quantile)
	}

	resp := make(map[string]model.Value)
	var err error
	for i := 0; i < len(quantiles); i++ {
		result := <-resultChan
		if result.err != nil {
			logrus.Errorf("query failed with %s", result.err)
			err = result.err
		} else {
			resp[result.prom] = result.vec
		}
	}
	if err != nil {
		// only return if all queries succeeded?
		renderJSONError(w, err, http.StatusInternalServerError)
		return
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rpsQuery := fmt.Sprintf(RPS, queryLabels, window)
	result, warnings, err := promAPI.Query(ctx, rpsQuery, time.Now())
	if err != nil {
		logrus.Errorf("error querying prometheus: %v", err)
		renderJSONError(w, err, http.StatusInternalServerError)
		return
	}
	if len(warnings) > 0 {
		logrus.Warnf("%v", warnings)
	}
	resp["RPS"] = result

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	queryLabels = fmt.Sprintf("{classification=\"success\",direction=\"inbound\",app=\"%s\"}", appName)
	successRateQuery := fmt.Sprintf(SUCCESSRATE, queryLabels, window, queryLabels, window)
	result, warnings, err = promAPI.Query(ctx, successRateQuery, time.Now())
	if err != nil {
		logrus.Errorf("error querying prometheus: %v", err)
		renderJSONError(w, err, http.StatusInternalServerError)
		return
	}
	if len(warnings) > 0 {
		logrus.Warnf("%v", warnings)
	}
	resp["SUCCESS_RATE"] = result
	renderJSON(w, resp)
}



// func websocketError(ws *websocket.Conn, wsError int, err error) {
// 	msg := validateControlFrameMsg(err)

// 	err = ws.WriteControl(websocket.CloseMessage,
// 		websocket.FormatCloseMessage(wsError, msg),
// 		time.Time{})
// 	if err != nil {
// 		log.Errorf("Unexpected websocket error: %s", err)
// 	}
// }

func (h *handler) handleAPITap(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
	panic("not implemented")
}

func (h *handler) handleAPIEdges(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
	panic("not implemented")
	//requestParams := util.EdgesRequestParams{
	//	Namespace:    req.FormValue("namespace"),
	//	ResourceType: req.FormValue("resource_type"),
	//}

	//edgesRequest, err := util.BuildEdgesRequest(requestParams)
	//if err != nil {
	//	renderJSONError(w, err, http.StatusInternalServerError)
	//	return
	//}

	//result, err := h.apiClient.Edges(req.Context(), edgesRequest)
	//if err != nil {
	//	renderJSONError(w, err, http.StatusInternalServerError)
	//	return
	//}
	//renderJSON(w, result)
}
