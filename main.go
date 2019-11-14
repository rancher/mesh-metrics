package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/daxmc99/prometheus-scraper/srv"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/api"
	"github.com/sirupsen/logrus"
)

// TODO Don't hardcode this
const version = "v0.1.0"

func main() {

	cmd := flag.NewFlagSet("public-api", flag.ExitOnError)
	addr := cmd.String("addr", "127.0.0.1:8084", "address to serve on")
	apiAddr := cmd.String("api-addr", "http://linkerd-prometheus.linkerd.svc.cluster.local:9090", "address of the prometheus service")
	prometheusNamespace := cmd.String("controller-namespace", "linkerd", "namespace in which Linkerd is installed")

	// Deprecated: use "log-level" instead
	debug := cmd.Bool("debug", false, "enable debug logging")

	configureAndParse(cmd, os.Args[1:])

	if *debug {
		logrus.SetLevel(logrus.DebugLevel)
	}
	// TODO Sanity check we can connect here and verify promtheus server has correct config

	client, err := api.NewClient(api.Config{
		Address: *apiAddr,
	})
	if err != nil {
		logrus.Fatalf("failed to construct client for API server URL %s", *apiAddr)
	}

	//TODO Figure out a better way of determin cluster domain
	clusterDomain := "cluster.local"

	// k8sAPI, err := k8s.NewAPI(*kubeConfigPath, "", "", 0)
	// if err != nil {
	// 	logrus.Fatalf("failed to construct Kubernetes API client: [%s]", err)
	// }

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	uuidstring := uuid.New().String()

	server := srv.NewServer(*addr, *prometheusNamespace, clusterDomain, true, uuidstring, client)

	go func() {
		logrus.Infof("starting HTTP server on %+v", *addr)
		server.ListenAndServe()
	}()

	<-stop

	logrus.Infof("shutting down HTTP server on %+v", *addr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}

func configureAndParse(cmd *flag.FlagSet, args []string) {
	flag.Set("stderrthreshold", "FATAL")
	flag.Set("logtostderr", "false")
	flag.Set("log_file", "/dev/null")
	flag.Set("v", "0")

	logLevel := cmd.String("log-level", logrus.InfoLevel.String(),
		"log level, must be one of: panic, fatal, error, warn, info, debug")
	printVersion := cmd.Bool("version", false, "print version and exit")

	cmd.Parse(args)

	// set log timestamps
	formatter := &logrus.TextFormatter{FullTimestamp: true}
	logrus.SetFormatter(formatter)
	setLogLevel(*logLevel)
	maybePrintVersionAndExit(*printVersion)

}

func setLogLevel(logLevel string) {
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		logrus.Fatalf("invalid log-level: %s", logLevel)
	}
	logrus.SetLevel(level)

	if level == logrus.DebugLevel {
		flag.Set("stderrthreshold", "INFO")
		flag.Set("logtostderr", "true")
		flag.Set("v", "6") // At 7 and higher, authorization tokens get logged.

		// TODO Pipe klog to logrus
	}
}

func maybePrintVersionAndExit(printVersion bool) {
	if printVersion {
		fmt.Println(version)
		os.Exit(0)
	}
	logrus.Infof("running version %s", version)
}
