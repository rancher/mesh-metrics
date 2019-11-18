package srv

import (
	"fmt"
	"math"
)

// TODO: check earlier in the code if we find a NaN
// 	so that we debug the query
func checkNanBlock(block EdgeResp) error {

	for _, node := range block.Nodes {
		for key, value := range node.Stats {
			if math.IsNaN(value) {
				return fmt.Errorf("found NaN inside struct in Node: [app:%s version:%s ns: %s], for stats key: [%s]", node.App, node.Version, node.Namespace, key)
			}
		}
	}
	for _, edge := range block.Edges {
		for key, value := range edge.Stats {
			if math.IsNaN(value) {
				return fmt.Errorf("found NaN inside struct in edge from: [app:%s version:%s ns: %s], for stats key: [%s]", edge.FromApp, edge.FromVersion, edge.FromNamespace, key)
			}
		}
	}
	return nil
}
