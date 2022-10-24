// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package inventory

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	k8sCollectors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s"
)

// CollectorInventory is used to store and retrieve available collectors.
type CollectorInventory struct {
	collectors          []collectors.CollectorVersions
	activatedCollectors map[string]collectors.Collector
}

// NewCollectorInventory returns a new inventory containing all known
// collectors.
func NewCollectorInventory() *CollectorInventory {
	return &CollectorInventory{
		collectors: []collectors.CollectorVersions{
			k8sCollectors.NewClusterCollectorVersions(),
			k8sCollectors.NewClusterRoleCollectorVersions(),
			k8sCollectors.NewClusterRoleBindingCollectorVersions(),
			k8sCollectors.NewCronJobCollectorVersions(),
			k8sCollectors.NewDaemonSetCollectorVersions(),
			k8sCollectors.NewDeploymentCollectorVersions(),
			k8sCollectors.NewIngressCollectorVersions(),
			k8sCollectors.NewJobCollectorVersions(),
			k8sCollectors.NewNodeCollectorVersions(),
			k8sCollectors.NewPersistentVolumeCollectorVersions(),
			k8sCollectors.NewPersistentVolumeClaimCollectorVersions(),
			k8sCollectors.NewReplicaSetCollectorVersions(),
			k8sCollectors.NewRoleCollectorVersions(),
			k8sCollectors.NewRoleBindingCollectorVersions(),
			k8sCollectors.NewServiceCollectorVersions(),
			k8sCollectors.NewServiceAccountCollectorVersions(),
			k8sCollectors.NewStatefulSetCollectorVersions(),
			k8sCollectors.NewUnassignedPodCollectorVersions(),
			k8sCollectors.NewCRDCollectorVersions(),
		},
		activatedCollectors: map[string]collectors.Collector{},
	}
}

// CollectorForCustomResource creates a custom resource collector given its name. It returns an error if the resource does not exist
func (ci *CollectorInventory) CollectorForCustomResource(collectorName string, discovery *orchestrator.APIServerDiscoveryProvider) (collectors.Collector, error) {
	// discover resources on the api server
	// cache them
	// use the cache and check grv against
	grv := strings.Split(collectorName, "/")
	if len(grv) < 3 { // resources can have sub slashes, so we need at least 3
		return nil, fmt.Errorf("GRV needs to be of the following format: <apigroup_and_version>/<collector_name")
	}

	collector, err := discovery.DiscoverCRDResource(collectorName)
	if err != nil {
		return nil, err
	}

	return collector, nil
}

// CollectorForDefaultVersion retrieves a collector given its name. It returns an error if the
// name is not known.
func (ci *CollectorInventory) CollectorForDefaultVersion(collectorName string) (collectors.Collector, error) {
	for _, cv := range ci.collectors {
		for _, c := range cv.Collectors {
			if c.Metadata().Name == collectorName && c.Metadata().IsDefaultVersion {
				return c, nil
			}
		}
	}
	return nil, fmt.Errorf("no collector found for name %s", collectorName)
}

// CollectorForVersion gets a collector given its name and version. It returns
// an error if the collector name or version is not known.
func (ci *CollectorInventory) CollectorForVersion(collectorName, collectorVersion string) (collectors.Collector, error) {
	for _, cv := range ci.collectors {
		for _, c := range cv.Collectors {
			if c.Metadata().Name == collectorName && c.Metadata().Version == collectorVersion {
				return c, nil
			}
		}
	}
	return nil, fmt.Errorf("no collector found for name %s and version %s", collectorName, collectorVersion)
}

// StableCollectors get a list of all stable collectors in the inventory.
func (ci *CollectorInventory) StableCollectors() []collectors.Collector {
	var stableCollectors []collectors.Collector
	for _, cv := range ci.collectors {
		for _, c := range cv.Collectors {
			if c.Metadata().IsStable && c.Metadata().IsDefaultVersion {
				stableCollectors = append(stableCollectors, c)
			}
		}
	}
	return stableCollectors
}
