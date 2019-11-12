package srv

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/api"
)

type (
	handler struct {
		apiClient           api.Client
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
