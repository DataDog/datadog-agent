package crd

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/discovery"
)

type DiscoveryCollector struct {
	*discovery.APIServerDiscoveryProvider
}

func NewDiscoveryCollectorForInventory() *DiscoveryCollector {
	return &DiscoveryCollector{
		APIServerDiscoveryProvider: discovery.NewAPIServerDiscoveryProvider(),
	}
}

func (d *DiscoveryCollector) VerifyForInventory(collectorName string) (collectors.Collector, error) {
	collector, err := d.APIServerDiscoveryProvider.DiscoverCRDResource(collectorName)
	if err != nil {
		return nil, err
	}
	return collector, nil
}
