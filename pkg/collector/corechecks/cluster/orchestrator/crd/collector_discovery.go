// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
