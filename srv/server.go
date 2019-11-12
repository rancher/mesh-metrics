package srv

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/linkerd/linkerd2/pkg/prometheus"
	"github.com/prometheus/client_golang/api"
)

const (
	timeout = 10 * time.Second
)

type (
	// Server encapsulates the code for talking with prometheus
	Server struct {
		reload bool
		router *mux.Router
	}
	// might be used for creating clients
	appParams struct {
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
	controllerNamespace string,
	clusterDomain string,
	reload bool,
	uuid string,
	apiClient api.Client,
	// k8sAPI *k8s.KubernetesAPI,
) *http.Server {
	server := &Server{
		reload: reload,
	}

	server.router = mux.NewRouter()
	server.router.UseEncodedPath()

	wrappedServer := prometheus.WithTelemetry(server)
	handler := &handler{
		apiClient:           apiClient,
		uuid:                uuid,
		controllerNamespace: controllerNamespace,
		clusterDomain:       clusterDomain,
	}

	httpServer := &http.Server{
		Addr:         addr,
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		Handler:      wrappedServer,
	}

	// webapp routes
	server.router.Handle("/", handler.handleHello())
	server.router.Handle("/hello", handler.handleHello())

	server.router.Handle("/api/version", handler.handleAPIVersion())
	server.router.Handle("/api/v0/cluster", handler.handleClusterInfo())
	//server.router.GET("/api/v0/app/:app", handler.handleAppStats)
	server.router.Handle("/api/v0/namespace/{namespace}", handler.handleAPIEdges())

	return httpServer
}
