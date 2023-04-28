// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package discovery

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/inventory"
	k8sCollectors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type discoveryCache struct {
	groups              []*v1.APIGroup
	resources           []*v1.APIResourceList
	filled              bool
	collectorForVersion map[collectorVersion]struct{}
}

type collectorVersion struct {
	version string
	name    string
}

type DiscoveryCollector struct {
	cache discoveryCache
}

func NewDiscoveryCollectorForInventory() *DiscoveryCollector {
	dc := &DiscoveryCollector{
		cache: discoveryCache{collectorForVersion: map[collectorVersion]struct{}{}},
	}
	err := dc.fillCache()
	if err != nil {
		log.Error("Fail to init discovery collector", err)
	}
	return dc
}
func (d *DiscoveryCollector) fillCache() error {
	if !d.cache.filled {
		var err error
		d.cache.groups, d.cache.resources, err = GetServerGroupsAndResources()
		if err != nil {
			return err
		}

		if len(d.cache.resources) == 0 {
			return fmt.Errorf("failed to discover resources from API groups")
		}
		for _, list := range d.cache.resources {
			for _, resource := range list.APIResources {
				cv := collectorVersion{
					version: list.GroupVersion,
					name:    resource.Name,
				}
				d.cache.collectorForVersion[cv] = struct{}{}
			}
		}
		d.cache.filled = true
	}
	return nil
}

func (d *DiscoveryCollector) VerifyForCRDInventory(resource string, groupVersion string) (collectors.Collector, error) {
	collector, err := d.DiscoverCRDResource(resource, groupVersion)
	if err != nil {
		return nil, err
	}
	return collector, nil
}

func (d *DiscoveryCollector) VerifyForInventory(resource string, groupVersion string, collectorInventory *inventory.CollectorInventory) (collectors.Collector, error) {
	collector, err := d.DiscoverRegularResource(resource, groupVersion, collectorInventory)
	if err != nil {
		return nil, err
	}
	return collector, nil
}

func (d *DiscoveryCollector) DiscoverCRDResource(resource string, groupVersion string) (collectors.Collector, error) {
	collector, err := k8sCollectors.NewCRCollectorVersion(resource, groupVersion)
	if err != nil {
		return nil, err
	}

	return d.isSupportCollector(collector)
}

func (d *DiscoveryCollector) DiscoverRegularResource(resource string, groupVersion string, collectorInventory *inventory.CollectorInventory) (collectors.Collector, error) {
	var collector collectors.Collector
	var err error

	if groupVersion == "" {
		collector, err = collectorInventory.CollectorForDefaultVersion(resource)
	} else {
		collector, err = collectorInventory.CollectorForVersion(resource, groupVersion)
	}
	if err != nil {
		return nil, err
	}
	if resource == "clusters" {
		return d.isSupportClusterCollector(collector, collectorInventory)
	}

	return d.isSupportCollector(collector)
}

func (d *DiscoveryCollector) isSupportCollector(collector collectors.Collector) (collectors.Collector, error) {
	if _, ok := d.cache.collectorForVersion[collectorVersion{
		version: collector.Metadata().Version,
		name:    collector.Metadata().Name,
	}]; ok {
		return collector, nil
	}
	return nil, fmt.Errorf("failed to discover resource %s", collector.Metadata().Name)
}

func (d *DiscoveryCollector) isSupportClusterCollector(collector collectors.Collector, collectorInventory *inventory.CollectorInventory) (collectors.Collector, error) {
	nodeCollector, err := collectorInventory.CollectorForDefaultVersion("nodes")
	if err != nil {
		return nil, fmt.Errorf("failed to discover cluster resource %w", err)
	}
	_, err = d.isSupportCollector(nodeCollector)
	if err != nil {
		return nil, fmt.Errorf("failed to discover resource %s", collector.Metadata().Name)
	}
	return collector, nil

}
