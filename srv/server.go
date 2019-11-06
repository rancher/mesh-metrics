package srv

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/api"

	"github.com/julienschmidt/httprouter"
	pb "github.com/linkerd/linkerd2/controller/gen/public"
	"github.com/linkerd/linkerd2/pkg/k8s"
	"github.com/linkerd/linkerd2/pkg/prometheus"
)

const (
	timeout = 10 * time.Second
)

type (
	// Server encapsulates the code for talking with prometheus
	Server struct {
		reload bool
		router *httprouter.Router
	}
	// might be used for creating clients
	appParams struct {
		Data                pb.VersionInfo
		UUID                string
		ControllerNamespace string
		Error               bool
		ErrorMessage        string
		PathPrefix          string
	}
)

// this is called by the HTTP server to actually respond to a request
func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	s.router.ServeHTTP(w, req)
}

// NewServer returns an initialized `http.Server`, configured to listen on an
// address, render templates, and serve static assets, for a given Linkerd
// control plane.
func NewServer(
	addr string,
	grafanaAddr string,
	controllerNamespace string,
	clusterDomain string,
	reload bool,
	uuid string,
	apiClient api.Client,
	k8sAPI *k8s.KubernetesAPI,
) *http.Server {
	server := &Server{
		reload: reload,
	}

	server.router = &httprouter.Router{
		RedirectTrailingSlash:  true,
		RedirectFixedPath:      true,
		HandleMethodNotAllowed: true, // give 405 if you are using something we don't support
	}

	wrappedServer := prometheus.WithTelemetry(server)
	handler := &handler{
		apiClient:           apiClient,
		k8sAPI:              k8sAPI,
		uuid:                uuid,
		controllerNamespace: controllerNamespace,
		clusterDomain:       clusterDomain,
		grafanaProxy:        newGrafanaProxy(grafanaAddr),
	}

	httpServer := &http.Server{
		Addr:         addr,
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		Handler:      wrappedServer,
	}

	// webapp routes
	server.router.GET("/", handler.handleIndex)
	server.router.GET("/hello", handler.handleHello)

	// webapp api routes
	server.router.GET("/api/version", handler.handleAPIVersion)

	server.router.GET("/api/v0/cluster", handler.handleClusterInfo)
	//server.router.GET("/api/v0/app/:app", handler.handleAppStats)
	server.router.GET("/api/v0/namespace/:namespace", handler.handleAPIEdges)

	// grafana proxy
	server.router.DELETE("/grafana/*grafanapath", handler.handleGrafana)
	server.router.GET("/grafana/*grafanapath", handler.handleGrafana)
	server.router.HEAD("/grafana/*grafanapath", handler.handleGrafana)
	server.router.OPTIONS("/grafana/*grafanapath", handler.handleGrafana)
	server.router.PATCH("/grafana/*grafanapath", handler.handleGrafana)
	server.router.POST("/grafana/*grafanapath", handler.handleGrafana)
	server.router.PUT("/grafana/*grafanapath", handler.handleGrafana)

	return httpServer
}
