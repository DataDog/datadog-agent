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
	collectors []collectors.Collector
}

// NewCollectorInventory returns a new inventory containing all known
// collectors.
func NewCollectorInventory() *CollectorInventory {
	return &CollectorInventory{
		collectors: []collectors.Collector{
			k8sCollectors.NewClusterCollector(),
			k8sCollectors.NewClusterRoleCollector(),
			k8sCollectors.NewClusterRoleBindingCollector(),
			k8sCollectors.NewCronJobCollector(),
			k8sCollectors.NewDaemonSetCollector(),
			k8sCollectors.NewDeploymentCollector(),
			k8sCollectors.NewJobCollector(),
			k8sCollectors.NewNodeCollector(),
			k8sCollectors.NewPersistentVolumeCollector(),
			k8sCollectors.NewPersistentVolumeClaimCollector(),
			k8sCollectors.NewReplicaSetCollector(),
			k8sCollectors.NewRoleCollector(),
			k8sCollectors.NewRoleBindingCollector(),
			k8sCollectors.NewServiceCollector(),
			k8sCollectors.NewServiceAccountCollector(),
			k8sCollectors.NewStatefulSetCollector(),
			k8sCollectors.NewUnassignedPodCollector(),
		},
	}
}

// CollectorByName gets a collector given its name. It returns an error if the
// name is not known.
func (ci *CollectorInventory) CollectorByName(collectorName string) (collectors.Collector, error) {
	for _, c := range ci.collectors {
		if c.Metadata().Name == collectorName {
			return c, nil
		}
	}
	return nil, fmt.Errorf("no collector found for name %s", collectorName)
}

// StableCollectors get a list of all stable collectors in the inventory.
func (ci *CollectorInventory) StableCollectors() []collectors.Collector {
	var stableCollectors []collectors.Collector
	for _, c := range ci.collectors {
		if c.Metadata().IsStable {
			stableCollectors = append(stableCollectors, c)
		}
	}
	return stableCollectors
}
