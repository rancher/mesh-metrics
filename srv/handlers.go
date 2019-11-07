package srv

import (
	"fmt"
	"net/http"

	"github.com/linkerd/linkerd2/pkg/k8s"
	"github.com/prometheus/client_golang/api"
)

type (
	handler struct {
		apiClient           api.Client
		k8sAPI              *k8s.KubernetesAPI
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
