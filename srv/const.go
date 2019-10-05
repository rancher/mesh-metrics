package srv

const (
	windowDefault          = "30s"
	clusterMemoryUsage     = `sum (container_memory_working_set_bytes{id="/"}) / sum (machine_memory_bytes{}) * 100`
	clusterCPUUsage1MinAvg = `sum (rate (container_cpu_usage_seconds_total{id="/",}[1m])) / sum (machine_cpu_cores) * 100`
	clusterFilesytemUse    = `sum (container_fs_usage_bytes{device=~"^/dev/[sv]d[a-z][1-9]$",id="/"}) / sum (container_fs_limit_bytes{device=~"^/dev/[sv]d[a-z][1-9]$",id="/",}) * 100`

	overallRespSuccessRate = `sum(irate(response_total{classification="success", direction="inbound"}[%s])) / sum(irate(response_total{direction="inbound"}[%s])) `

	latencyQuantileQuery = "histogram_quantile(%s, sum(irate(response_latency_ms_bucket%s[%s])) by (le, %s))"

	promLatencyP50 = "0.5"
	promLatencyP95 = "0.95"
	promLatencyP99 = "0.99"

	RPS         = `sum(irate(request_total%s[%s])) by (app)`
	SUCCESSRATE = `sum(irate(response_total%s[%s])) by (app) / sum(irate(response_total%s[%s])) by (app)`

	DEBUG2 = `sum(irate(response_total{classification="success",  direction="inbound"}[30s])) by (app) / sum(irate(response_total{ direction="inbound"}[30s])) by (app)`
	DEBUG3 = `histogram_quantile(0.5, sum(irate(response_latency_ms_bucket{direction="inbound"}[30s])) by (le,app))`
)
