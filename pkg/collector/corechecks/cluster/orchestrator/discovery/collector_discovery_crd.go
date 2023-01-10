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
	k8sCollectors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s"
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
	return &DiscoveryCollector{
		cache: discoveryCache{collectorForVersion: map[collectorVersion]struct{}{}},
	}
}

func (d *DiscoveryCollector) VerifyForInventory(resource string, groupVersion string) (collectors.Collector, error) {
	collector, err := d.DiscoverCRDResource(resource, groupVersion)
	if err != nil {
		return nil, err
	}
	return collector, nil
}

func (d *DiscoveryCollector) DiscoverCRDResource(resource string, groupVersion string) (*k8sCollectors.CRCollector, error) {
	if !d.cache.filled {
		var err error
		d.cache.groups, d.cache.resources, err = GetServerGroupsAndResources()
		if err != nil {
			return nil, err
		}

		if len(d.cache.resources) == 0 {
			return nil, fmt.Errorf("failed to discover resources from API groups")
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

	collector, err := k8sCollectors.NewCRCollectorVersion(resource, groupVersion)
	if err != nil {
		return nil, err
	}
	if _, ok := d.cache.collectorForVersion[collectorVersion{
		version: collector.Metadata().Version,
		name:    collector.Metadata().Name,
	}]; ok {
		return collector, nil
	}
	return nil, fmt.Errorf("failed to discover resource %s", resource)
}
