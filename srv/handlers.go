package srv

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"

	"github.com/prometheus/common/log"

	"github.com/prometheus/client_golang/api"

	"github.com/julienschmidt/httprouter"
	"github.com/linkerd/linkerd2/pkg/k8s"
	"github.com/linkerd/linkerd2/pkg/profiles"
	"github.com/sirupsen/logrus"
)

var proxyPathRegexp = regexp.MustCompile("/api/v1/namespaces/.*/proxy/")

type (
	renderTemplate func(http.ResponseWriter, string, string, interface{}) error

	handler struct {
		apiClient           api.Client
		k8sAPI              *k8s.KubernetesAPI
		uuid                string
		controllerNamespace string
		clusterDomain       string
		grafanaProxy        *grafanaProxy
	}
)

func (h *handler) handleIndex(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
	// when running the dashboard via `linkerd dashboard`, serve the index bundle at the right path
	pathPfx := proxyPathRegexp.FindString(req.URL.Path)
	if pathPfx == "" {
		pathPfx = "/"
	}

	params := appParams{
		UUID:                h.uuid,
		ControllerNamespace: h.controllerNamespace,
		PathPrefix:          pathPfx,
	}

	_, err := fmt.Fprint(w, "You have reached the prom server in %v", params)
	if err != nil {
		logrus.Error(err)
	}
}

func (h *handler) handleHello(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Hello there! \n")
}

func (h *handler) handleProfileDownload(w http.ResponseWriter, req *http.Request, params httprouter.Params) {
	service := req.FormValue("service")
	namespace := req.FormValue("namespace")

	if service == "" || namespace == "" {
		err := fmt.Errorf("Service and namespace must be provided to create a new profile")
		log.Error(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	profileYaml := &bytes.Buffer{}
	err := profiles.RenderProfileTemplate(namespace, service, h.clusterDomain, profileYaml)

	if err != nil {
		log.Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	dispositionHeaderVal := fmt.Sprintf("attachment; filename=%s-profile.yml", service)

	w.Header().Set("Content-Type", "text/yaml")
	w.Header().Set("Content-Disposition", dispositionHeaderVal)

	w.Write(profileYaml.Bytes())
}

func (h *handler) handleGrafana(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
	h.grafanaProxy.ServeHTTP(w, req)
}
