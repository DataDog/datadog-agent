// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package inventory

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	k8sCollectors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s"
)

// CollectorInventory is used to store and retrieve available collectors.
type CollectorInventory struct {
	collectors   []collectors.CollectorVersions
	crCollectors map[string]collectors.Collector
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
		crCollectors: map[string]collectors.Collector{},
	}
}

// CollectorForCustomResource ...
func (ci *CollectorInventory) CollectorForCustomResource(collectorName string) (collectors.Collector, error) {
	if _, ok := ci.crCollectors[collectorName]; ok {
		return nil, fmt.Errorf("collector %s has already been added", collectorName)
	}
	// TODO: maybe check discover here
	collector := k8sCollectors.NewCRCollectorVersions(collectorName)
	ci.crCollectors[collectorName] = collector.Collectors[0]
	return collector.Collectors[0], nil // TODO: there is no play to have more than one collector versions per collector, but this is hacky and should be revisited
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
