package metricsender

type MetricSender interface {
	Gauge(metricName string, value float64, tags []string)
}
