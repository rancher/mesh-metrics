package srv

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"

	"github.com/prometheus/common/model"
	"github.com/sirupsen/logrus"
)

type promResult struct {
	prom string
	vec  model.Vector
	err  error
}

var quantiles = [3]string{promLatencyP50, promLatencyP95, promLatencyP99}

func statQuery(ctx context.Context, promAPI v1.API, appName, version, window, direction string) (map[string]float64, error) {
	resultChan := make(chan promResult)
	var queryLabels string
	if version != "" {
		queryLabels = fmt.Sprintf("{direction=\"%s\",app=\"%s\",version=\"%s\"}", direction, appName, version)
	} else {
		queryLabels = fmt.Sprintf("{direction=\"%s\",app=\"%s\"}", direction, appName)
	}

	for _, quantile := range quantiles {
		go func(quantile string) {
			latencyQuery := fmt.Sprintf(latencyQuantileQuery, quantile, queryLabels, window, "app")
			logrus.Debugf("Performing stat query: %v", latencyQuery)
			latencyResult, warnings, err := promAPI.Query(ctx, latencyQuery, time.Now())
			if err != nil {
				resultChan <- promResult{
					prom: "",
					vec:  nil,
					err:  err,
				}
				return
			}
			if latencyResult.Type() != model.ValVector {
				err := fmt.Errorf("unexpected query result type (expected Vector): %s", latencyResult.Type())
				logrus.Errorf("%v", err)
				resultChan <- promResult{
					prom: "",
					vec:  nil,
					err:  err,
				}
				return
			}

			if warnings != nil {
				logrus.Warnf("%v", warnings)
			}

			resultChan <- promResult{
				prom: quantile,
				vec:  latencyResult.(model.Vector),
				err:  err,
			}
		}(quantile)
	}

	resp := make(map[string]float64)
	var err error
	for i := 0; i < len(quantiles); i++ {
		result := <-resultChan
		if result.err != nil {
			logrus.Errorf("query failed with %s", result.err)
			err = result.err
		} else {
			var label string
			switch result.prom {
			case promLatencyP50:
				label = "p50ms"
			case promLatencyP95:
				label = "p90ms"
			case promLatencyP99:
				label = "p99ms"
			default:
				label = result.prom
			}
			if len(result.vec) == 0 {
				resp[label] = 0.0
			} else {
				resp[label] = float64(result.vec[0].Value)
			}
		}
	}
	if err != nil {
		// only return if all queries succeeded?
		return nil, err
	}

	rpsQuery := fmt.Sprintf(RPS, queryLabels, window)
	logrus.Debugf("Performing rps query: %v", rpsQuery)
	result, warnings, err := promAPI.Query(ctx, rpsQuery, time.Now())
	if err != nil {
		return nil, err
	}
	if len(warnings) > 0 {
		logrus.Warnf("%v", warnings)
	}
	resultScalar, ok := result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("unexpected query result type (expected Vector): %s", result.Type())
	}
	if len(resultScalar) == 0 {
		resp["rps"] = float64(model.SampleValue(0.0))
	} else {
		resp["rps"] = float64(resultScalar[0].Value)
	}

	var nonSuccesRateLabels string
	if version != "" {
		queryLabels = fmt.Sprintf("{classification=\"success\",direction=\"%s\",app=\"%s\",version=\"%s\"}", direction, appName, version)
		nonSuccesRateLabels = fmt.Sprintf("{direction=\"%s\",app=\"%s\",version=\"%s\"}", direction, appName, version)
	} else {
		queryLabels = fmt.Sprintf("{classification=\"success\",direction=\"%s\",app=\"%s\"}", direction, appName)
		nonSuccesRateLabels = fmt.Sprintf("{direction=\"%s\",app=\"%s\"}", direction, appName)
	}

	successRateQuery := fmt.Sprintf(SUCCESSRATE, queryLabels, window, nonSuccesRateLabels, window)
	logrus.Debugf("Performing success rate query: %v", successRateQuery)
	result, warnings, err = promAPI.Query(ctx, successRateQuery, time.Now())
	if err != nil {
		return nil, err
	}
	if len(warnings) > 0 {
		logrus.Warnf("%v", warnings)
	}
	resultScalar, ok = result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("unexpected query result type (expected Vector): %s", result.Type())
	}
	if len(resultScalar) == 0 {
		resp["successRate"] = float64(model.SampleValue(0.0))
	} else {
		resp["successRate"] = float64(resultScalar[0].Value)
	}
	return resp, nil
}
