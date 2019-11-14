package srv

import (
	"net/http"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opencensus.io/plugin/ochttp"
)

const (
	timeout = 10 * time.Second
)

var (

	// RequestLatencyBucketsSeconds represents latency buckets to record (seconds)
	RequestLatencyBucketsSeconds = append(append(append(append(
		prometheus.LinearBuckets(0.01, 0.01, 5),
		prometheus.LinearBuckets(0.1, 0.1, 5)...),
		prometheus.LinearBuckets(1, 1, 5)...),
		prometheus.LinearBuckets(10, 10, 5)...),
	)

	// ResponseSizeBuckets represents response size buckets (bytes)
	ResponseSizeBuckets = append(append(append(append(
		prometheus.LinearBuckets(100, 100, 5),
		prometheus.LinearBuckets(1000, 1000, 5)...),
		prometheus.LinearBuckets(10000, 10000, 5)...),
		prometheus.LinearBuckets(1000000, 1000000, 5)...),
	)
	// server metrics
	serverCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_server_requests_total",
			Help: "A counter for requests to the wrapped handler.",
		},
		[]string{"code", "method"},
	)

	serverLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_server_request_latency_seconds",
			Help:    "A histogram of latencies for requests in seconds.",
			Buckets: RequestLatencyBucketsSeconds,
		},
		[]string{"code", "method"},
	)

	serverResponseSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_server_response_size_bytes",
			Help:    "A histogram of response sizes for requests.",
			Buckets: ResponseSizeBuckets,
		},
		[]string{"code", "method"},
	)
)

type (
	// Server encapsulates the code for talking with prometheus
	Server struct {
		reload bool
		router *mux.Router
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
) *http.Server {
	server := &Server{
		reload: reload,
	}

	server.router = mux.NewRouter()
	server.router.UseEncodedPath()

	wrappedServer := WithTelemetry(server)

	//create prometheus API (safe for use with multiple go routines)
	promAPI := v1.NewAPI(apiClient)
	handler := &handler{
		promAPI:             promAPI,
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

// WithTelemetry instruments the HTTP server with prometheus and oc-http handler
func WithTelemetry(handler http.Handler) http.Handler {
	return &ochttp.Handler{
		Handler: promhttp.InstrumentHandlerDuration(serverLatency,
			promhttp.InstrumentHandlerResponseSize(serverResponseSize,
				promhttp.InstrumentHandlerCounter(serverCounter, handler))),
	}
}
