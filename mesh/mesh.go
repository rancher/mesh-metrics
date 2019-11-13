package lib

import (
	"net/http"

	"github.com/daxmc99/prometheus-scraper/srv"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/sirupsen/logrus"
)

type MeshMetrics interface {
	SummaryHandler() http.Handler
}

func NewMeshMetrics(promEndpoint string) MeshMetrics {

	client, err := api.NewClient(api.Config{
		Address:      promEndpoint,
		RoundTripper: nil,
	})
	if err != nil {
		logrus.Info("Failed to build prometheus client:", err)
		return nil
	}
	promAPI := v1.NewAPI(client)
	return edgesMetrics{promAPI}
}

type edgesMetrics struct {
	v1.API
}

func (e edgesMetrics) SummaryHandler() http.Handler {
	return srv.HandleEdges(e.API)
}
