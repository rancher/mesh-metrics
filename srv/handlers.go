package srv

import (
	"fmt"
	"net/http"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

type (
	handler struct {
		promAPI             v1.API
		uuid                string
		controllerNamespace string
		clusterDomain       string
	}
)

func (h *handler) handleHello() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Hello there! \n")
	})
}
