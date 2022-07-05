package discoverycollector

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DiscoveryCollector TODO
type DiscoveryCollector struct {
	sender   aggregator.Sender
	hostname string
}

// NewDiscoveryCollector TODO
func NewDiscoveryCollector(sender aggregator.Sender, hostname string) *DiscoveryCollector {
	return &DiscoveryCollector{
		sender:   sender,
		hostname: hostname,
	}
}

// Collect TODO
func (dc *DiscoveryCollector) Collect() {
	log.Info("Collector: collect")
}
