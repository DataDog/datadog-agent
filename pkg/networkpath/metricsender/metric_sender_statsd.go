package metricsender

import "github.com/DataDog/datadog-agent/pkg/process/statsd"

type metricSenderStatsd struct {
}

func NewMetricSenderStatsd() MetricSender {
	return &metricSenderStatsd{}
}

func (s metricSenderStatsd) Gauge(metricName string, value float64, tags []string) {
	statsd.Client.Gauge(metricName, value, tags, 1) //nolint:errcheck
}
