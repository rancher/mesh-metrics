module github.com/daxmc99/mesh-metrics

go 1.13

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.0.0+incompatible
	github.com/deislabs/smi-sdk-go => github.com/deislabs/smi-sdk-go v0.2.1-0.20191014150154-546a4a4075e2
)

require (
	github.com/daxmc99/prometheus-scraper v0.0.1
	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.7.3
	github.com/json-iterator/go v1.1.8 // indirect
	github.com/prometheus/client_golang v1.1.0
	github.com/prometheus/common v0.6.0
	github.com/sirupsen/logrus v1.4.2
	go.opencensus.io v0.22.2
	golang.org/x/net v0.0.0-20191004110552-13f9640d40b9 // indirect
	golang.org/x/sys v0.0.0-20190826190057-c7b8b68b1456 // indirect
)
