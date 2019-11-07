package main

import (
	"context"
	"flag"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/daxmc99/prometheus-scraper/srv"
	"github.com/google/uuid"
	"github.com/linkerd/linkerd2/pkg/admin"
	"github.com/linkerd/linkerd2/pkg/flags"
	"github.com/linkerd/linkerd2/pkg/k8s"
	"github.com/prometheus/client_golang/api"

	"github.com/sirupsen/logrus"
)

func main() {

	cmd := flag.NewFlagSet("public-api", flag.ExitOnError)
	addr := cmd.String("addr", "127.0.0.1:8084", "address to serve on")
	metricsAddr := cmd.String("metrics-addr", "127.0.0.1:9958", "address to serve scrapable metrics on")
	apiAddr := cmd.String("api-addr", "127.0.0.1:9090", "address of the prometheus service")
	grafanaAddr := cmd.String("grafana-addr", "127.0.0.1:3000", "address of the linkerd-grafana service")
	prometheusNamespace := cmd.String("controller-namespace", "linkerd", "namespace in which Linkerd is installed")
	debug := cmd.Bool("debug", false, "enable debug logging")
	kubeConfigPath := cmd.String("kubeconfig", "", "path to kube config")

	flags.ConfigureAndParse(cmd, os.Args[1:])

	if *debug {
		logrus.SetLevel(logrus.DebugLevel)
	}
	// TODO maybe sanity check we can connect here?
	_, _, err := net.SplitHostPort(*apiAddr) // Verify apiAddr is of the form host:port.
	//if err != nil {
	//	logrus.Fatalf("failed to parse API server address: %s", *apiAddr)
	//}

	//client, err := public.NewInternalClient(*prometheusNamespace, *apiAddr)
	//if err != nil {
	//	logrus.Fatalf("failed to construct client for API server URL %s", *apiAddr)
	//}
	client, err := api.NewClient(api.Config{
		Address: *apiAddr,
	})

	k8sAPI, err := k8s.NewAPI(*kubeConfigPath, "", "", 0)
	if err != nil {
		logrus.Fatalf("failed to construct Kubernetes API client: [%s]", err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	uuidstring := uuid.New().String()

	server := srv.NewServer(*addr, *grafanaAddr, *prometheusNamespace, "127.0.0.1", true, uuidstring, client, k8sAPI)

	go func() {
		logrus.Infof("starting HTTP server on %+v", *addr)
		server.ListenAndServe()
	}()

	go admin.StartServer(*metricsAddr)

	<-stop

	logrus.Infof("shutting down HTTP server on %+v", *addr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}
