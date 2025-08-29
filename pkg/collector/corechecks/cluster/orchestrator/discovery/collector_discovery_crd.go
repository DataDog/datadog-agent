// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package discovery

import (
	"fmt"
	"strings"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/inventory"
	k8sCollectors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s"
	utilTypes "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DiscoveryCache is a cache for the discovery collector.
type DiscoveryCache struct {
	Groups              []*v1.APIGroup
	Resources           []*v1.APIResourceList
	Filled              bool
	CollectorForVersion map[CollectorVersion]struct{}
}

// CollectorVersion represents a version of a collector with its name.
type CollectorVersion struct {
	Version string
	Name    string
}

// DiscoveryCollector stores all the discovered resources and their versions.
type DiscoveryCollector struct {
	cache DiscoveryCache
}

// NewDiscoveryCollectorForInventory returns a new DiscoveryCollector instance.
func NewDiscoveryCollectorForInventory() *DiscoveryCollector {
	dc := &DiscoveryCollector{
		cache: DiscoveryCache{CollectorForVersion: map[CollectorVersion]struct{}{}},
	}
	err := dc.fillCache()
	if err != nil {
		log.Errorc(fmt.Sprintf("Fail to init discovery collector : %s", err.Error()), orchestrator.ExtraLogContext...)
	}
	return dc
}

// fillCache adds all the discovered resources and their versions to the cache.
func (d *DiscoveryCollector) fillCache() error {
	if !d.cache.Filled {
		var err error
		d.cache.Groups, d.cache.Resources, err = GetServerGroupsAndResources()
		if err != nil {
			return err
		}

		if len(d.cache.Resources) == 0 {
			return fmt.Errorf("failed to discover resources from API groups")
		}
		for _, list := range d.cache.Resources {
			for _, resource := range list.APIResources {
				cv := CollectorVersion{
					Version: list.GroupVersion,
					Name:    resource.Name,
				}
				d.cache.CollectorForVersion[cv] = struct{}{}
			}
		}
		d.cache.Filled = true
	}
	return nil
}

// SetCache sets the cache for the DiscoveryCollector. This is useful for testing purposes
func (d *DiscoveryCollector) SetCache(cache DiscoveryCache) {
	d.cache = cache
}

//nolint:revive // TODO(CAPP) Fix revive linter
func (d *DiscoveryCollector) VerifyForCRDInventory(resource string, groupVersion string) (collectors.K8sCollector, error) {
	collector, err := d.DiscoverCRDResource(resource, groupVersion)
	if err != nil {
		return nil, err
	}
	return collector, nil
}

//nolint:revive // TODO(CAPP) Fix revive linter
func (d *DiscoveryCollector) VerifyForInventory(resource string, groupVersion string, collectorInventory *inventory.CollectorInventory) (collectors.K8sCollector, error) {
	collector, err := d.DiscoverRegularResource(resource, groupVersion, collectorInventory)
	if err != nil {
		return nil, err
	}
	return collector, nil
}

//nolint:revive // TODO(CAPP) Fix revive linter
func (d *DiscoveryCollector) DiscoverCRDResource(resource string, groupVersion string) (collectors.K8sCollector, error) {
	collector, err := k8sCollectors.NewCRCollectorVersion(resource, groupVersion)
	if err != nil {
		return nil, err
	}

	return d.isSupportCollector(collector)
}

//nolint:revive // TODO(CAPP) Fix revive linter
func (d *DiscoveryCollector) DiscoverRegularResource(resource string, groupVersion string, collectorInventory *inventory.CollectorInventory) (collectors.K8sCollector, error) {
	var collector collectors.K8sCollector
	var err error

	if groupVersion == "" {
		collector, err = collectorInventory.CollectorForDefaultVersion(resource)
	} else {
		collector, err = collectorInventory.CollectorForVersion(resource, groupVersion)
	}
	if err != nil {
		return nil, err
	}
	if resource == utilTypes.ClusterName {
		return d.isSupportClusterCollector(collector, collectorInventory)
	}
	if resource == utilTypes.TerminatedPodName {
		return d.isSupportTerminatedPodCollector(collector, collectorInventory)
	}

	return d.isSupportCollector(collector)
}

func (d *DiscoveryCollector) isSupportCollector(collector collectors.K8sCollector) (collectors.K8sCollector, error) {
	if _, ok := d.cache.CollectorForVersion[CollectorVersion{
		Version: collector.Metadata().Version,
		Name:    collector.Metadata().Name,
	}]; ok {
		return collector, nil
	}
	return nil, fmt.Errorf("failed to discover resource %s", collector.Metadata().Name)
}

func (d *DiscoveryCollector) isSupportClusterCollector(collector collectors.K8sCollector, collectorInventory *inventory.CollectorInventory) (collectors.K8sCollector, error) {
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

func (d *DiscoveryCollector) isSupportTerminatedPodCollector(collector collectors.K8sCollector, collectorInventory *inventory.CollectorInventory) (collectors.K8sCollector, error) {
	podCollector, err := collectorInventory.CollectorForDefaultVersion("pods")
	if err != nil {
		return nil, fmt.Errorf("failed to discover pod resource %w", err)
	}
	_, err = d.isSupportCollector(podCollector)
	if err != nil {
		return nil, fmt.Errorf("failed to discover resource %s", collector.Metadata().Name)
	}
	return collector, nil

}

// List returns a list of CollectorVersion for the given group.
func (d *DiscoveryCollector) List(group string) []CollectorVersion {
	var versions []CollectorVersion
	for cv := range d.cache.CollectorForVersion {
		if strings.HasPrefix(cv.Version, fmt.Sprintf("%s/", group)) && !strings.HasSuffix(cv.Name, "/status") {
			versions = append(versions, cv)
		}
	}
	return versions
}
