package nvidia

import "github.com/DataDog/datadog-agent/pkg/aggregator"

type metricsSender interface {
	Init() error
	SendMetrics(sender aggregator.Sender, field string) error
}
