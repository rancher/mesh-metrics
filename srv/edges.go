package srv

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/sirupsen/logrus"
)

// processEdgeMetrics gives the metrics for a given namespace unless the namespace is not given, in that cases it returns
// edges from all namespaces visible to prometheus
func processEdgeMetrics(ctx context.Context, promAPI v1.API, inbound, outbound model.Vector, selectedNamespace string) ([]Edge, error) {
	var edges []Edge
	dstIndex := map[model.LabelValue]model.Metric{}
	srcIndex := map[model.LabelValue][]model.Metric{}
	resourceType := "deployment"
	resourceReplacementInbound := resourceType
	resourceReplacementOutbound := "dst_" + resourceType

	for _, sample := range inbound {
		// skip inbound results without a clientID because we cannot construct edge
		// information
		clientID, ok := sample.Metric[model.LabelName("client_id")]
		if ok {
			dstResource := string(sample.Metric[model.LabelName(resourceReplacementInbound)])

			// format of clientId is id.namespace.serviceaccount.cluster.local
			clientIDSlice := strings.Split(string(clientID), ".")
			srcNs := clientIDSlice[1]
			key := model.LabelValue(fmt.Sprintf("%s.%s", dstResource, srcNs))
			dstIndex[key] = sample.Metric
		} else {
			logrus.Debugf("dropped metric: %s", sample.Metric)
		}
	}

	for _, sample := range outbound {
		dstResource := sample.Metric[model.LabelName(resourceReplacementOutbound)]
		srcNs := sample.Metric[model.LabelName("namespace")]

		key := model.LabelValue(fmt.Sprintf("%s.%s", dstResource, srcNs))
		if _, ok := srcIndex[key]; !ok {
			srcIndex[key] = []model.Metric{}
		}
		srcIndex[key] = append(srcIndex[key], sample.Metric)
	}

	for key, sources := range srcIndex {
		for _, src := range sources {
			srcNamespace := string(src[model.LabelName("namespace")])

			dst, ok := dstIndex[key]

			// if no destination, attempt to build from source data
			if !ok {
				dstNamespace := string(src[model.LabelName("dst_namespace")])

				// skip if selected namespace is given and neither the source nor
				// destination is in the selected namespace
				if selectedNamespace != "" && srcNamespace != selectedNamespace &&
					dstNamespace != selectedNamespace {
					continue
				}
			}

			dstNamespace := string(dst[model.LabelName("namespace")])

			// skip if selected namespace is given and neither the source nor
			// destination is in the selected namespace
			if selectedNamespace != "" && srcNamespace != selectedNamespace &&
				dstNamespace != selectedNamespace {
				continue
			}
			//TODO if fromApp would be empty we should also skip it

			edge := Edge{
				FromNamespace: srcNamespace,
				FromApp:       string(src[model.LabelName("app")]),
				FromVersion:   string(src[model.LabelName("version")]),
				ToNamespace:   dstNamespace,
				ToApp:         string(dst[model.LabelName("app")]),
				ToVersion:     string(dst[model.LabelName("version")]),
			}
			stats, err := statQuery(ctx, promAPI, edge.FromApp, edge.FromVersion, windowDefault, "outbound")
			if err != nil {
				return nil, err
			}
			edge.Stats = stats
			edges = append(edges, edge)
		}
	}

	return edges, nil
}

// async Process edges does the same as processEdges but does not perform any stats queries itself
func asyncProcessEdges(ctx context.Context, promAPI v1.API, inbound, outbound model.Vector, selectedNamespace string) ([]Edge, error) {
	var edges []Edge
	dstIndex := map[model.LabelValue]model.Metric{}
	srcIndex := map[model.LabelValue][]model.Metric{}
	resourceType := "deployment"
	resourceReplacementInbound := resourceType
	resourceReplacementOutbound := "dst_" + resourceType

	for _, sample := range inbound {
		// skip inbound results without a clientID because we cannot construct edge
		// information
		clientID, ok := sample.Metric[model.LabelName("client_id")]
		if ok {
			dstResource := string(sample.Metric[model.LabelName(resourceReplacementInbound)])

			// format of clientId is id.namespace.serviceaccount.cluster.local
			clientIDSlice := strings.Split(string(clientID), ".")
			srcNs := clientIDSlice[1]
			key := model.LabelValue(fmt.Sprintf("%s.%s", dstResource, srcNs))
			dstIndex[key] = sample.Metric
		} else {
			logrus.Debugf("dropped metric: %s", sample.Metric)
		}
	}

	for _, sample := range outbound {
		dstResource := sample.Metric[model.LabelName(resourceReplacementOutbound)]
		srcNs := sample.Metric[model.LabelName("namespace")]

		key := model.LabelValue(fmt.Sprintf("%s.%s", dstResource, srcNs))
		if _, ok := srcIndex[key]; !ok {
			srcIndex[key] = []model.Metric{}
		}
		srcIndex[key] = append(srcIndex[key], sample.Metric)
	}

	for key, sources := range srcIndex {
		for _, src := range sources {
			srcNamespace := string(src[model.LabelName("namespace")])

			dst, ok := dstIndex[key]

			// if no destination, attempt to build from source data
			if !ok {
				dstNamespace := string(src[model.LabelName("dst_namespace")])

				// skip if selected namespace is given and neither the source nor
				// destination is in the selected namespace
				if selectedNamespace != "" && srcNamespace != selectedNamespace &&
					dstNamespace != selectedNamespace {
					continue
				}
			}

			dstNamespace := string(dst[model.LabelName("namespace")])

			// skip if selected namespace is given and neither the source nor
			// destination is in the selected namespace
			if selectedNamespace != "" && srcNamespace != selectedNamespace &&
				dstNamespace != selectedNamespace {
				continue
			}
			//TODO if fromApp would be empty we should also skip it

			edge := Edge{
				FromNamespace: srcNamespace,
				FromApp:       string(src[model.LabelName("app")]),
				FromVersion:   string(src[model.LabelName("version")]),
				ToNamespace:   dstNamespace,
				ToApp:         string(dst[model.LabelName("app")]),
				ToVersion:     string(dst[model.LabelName("version")]),
			}
			edges = append(edges, edge)
		}
	}
	// no stats
	return edges, nil
}
