// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

//nolint:revive // TODO(CAPP) Fix revive linter
package inventory

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	k8sCollectors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
)

// defaultGenericResource is a list of generic resources that are collected by default.
var defaultGenericResource = []k8sCollectors.GenericResource{
	{
		Name:     "endpointslices",
		Group:    "discovery.k8s.io",
		Version:  "v1",
		NodeType: orchestrator.K8sEndpointSlice,
		Stable:   true,
	},
}

// getGenericCollectorVersions returns a list of collector versions for the default generic resources.
func getGenericCollectorVersions() []collectors.CollectorVersions {
	cvs := make([]collectors.CollectorVersions, 0, len(defaultGenericResource))
	for _, resource := range defaultGenericResource {
		cvs = append(cvs, resource.NewCollectorVersions())
	}
	return cvs
}

// CollectorInventory is used to store and retrieve available collectors.
type CollectorInventory struct {
	collectors []collectors.CollectorVersions
}

// NewCollectorInventory returns a new inventory containing all known
// collectors.
func NewCollectorInventory(cfg config.Component, store workloadmeta.Component, tagger tagger.Component) *CollectorInventory {
	return &CollectorInventory{
		collectors: append([]collectors.CollectorVersions{
			k8sCollectors.NewCRDCollectorVersions(),
			k8sCollectors.NewClusterCollectorVersions(),
			k8sCollectors.NewClusterRoleBindingCollectorVersions(tagger),
			k8sCollectors.NewClusterRoleCollectorVersions(tagger),
			k8sCollectors.NewCronJobCollectorVersions(tagger),
			k8sCollectors.NewDaemonSetCollectorVersions(tagger),
			k8sCollectors.NewDeploymentCollectorVersions(tagger),
			k8sCollectors.NewHorizontalPodAutoscalerCollectorVersions(tagger),
			k8sCollectors.NewIngressCollectorVersions(tagger),
			k8sCollectors.NewJobCollectorVersions(tagger),
			k8sCollectors.NewLimitRangeCollectorVersions(tagger),
			k8sCollectors.NewNamespaceCollectorVersions(tagger),
			k8sCollectors.NewNetworkPolicyCollectorVersions(tagger),
			k8sCollectors.NewNodeCollectorVersions(tagger),
			k8sCollectors.NewPersistentVolumeClaimCollectorVersions(tagger),
			k8sCollectors.NewPersistentVolumeCollectorVersions(tagger),
			k8sCollectors.NewPodDisruptionBudgetCollectorVersions(tagger),
			k8sCollectors.NewReplicaSetCollectorVersions(tagger),
			k8sCollectors.NewRoleBindingCollectorVersions(tagger),
			k8sCollectors.NewRoleCollectorVersions(tagger),
			k8sCollectors.NewServiceAccountCollectorVersions(tagger),
			k8sCollectors.NewServiceCollectorVersions(tagger),
			k8sCollectors.NewStatefulSetCollectorVersions(tagger),
			k8sCollectors.NewStorageClassCollectorVersions(tagger),
			k8sCollectors.NewUnassignedPodCollectorVersions(cfg, store, tagger),
			k8sCollectors.NewTerminatedPodCollectorVersions(cfg, store, tagger),
			k8sCollectors.NewVerticalPodAutoscalerCollectorVersions(tagger),
		}, getGenericCollectorVersions()...),
	}
}

// CollectorForDefaultVersion retrieves a collector given its name. It returns an error if the
// name is not known.
func (ci *CollectorInventory) CollectorForDefaultVersion(collectorName string) (collectors.K8sCollector, error) {
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
func (ci *CollectorInventory) CollectorForVersion(collectorName, collectorGroupVersion string) (collectors.K8sCollector, error) {
	for _, cv := range ci.collectors {
		for _, c := range cv.Collectors {
			if c.Metadata().Name == collectorName && c.Metadata().GroupVersion() == collectorGroupVersion {
				return c, nil
			}
		}
	}
	return nil, fmt.Errorf("no collector found for name %s and version %s", collectorName, collectorGroupVersion)
}

// StableCollectors get a list of all stable collectors in the inventory.
func (ci *CollectorInventory) StableCollectors() []collectors.K8sCollector {
	var stableCollectors []collectors.K8sCollector
	for _, cv := range ci.collectors {
		for _, c := range cv.Collectors {
			if c.Metadata().IsStable && c.Metadata().IsDefaultVersion {
				stableCollectors = append(stableCollectors, c)
			}
		}
	}
	return stableCollectors
}
