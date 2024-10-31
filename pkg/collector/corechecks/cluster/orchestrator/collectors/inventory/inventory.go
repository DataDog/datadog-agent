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
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	k8sCollectors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s"
)

// CollectorInventory is used to store and retrieve available collectors.
type CollectorInventory struct {
	collectors []collectors.CollectorVersions
}

// NewCollectorInventory returns a new inventory containing all known
// collectors.
func NewCollectorInventory(cfg config.Component, store workloadmeta.Component, tagger tagger.Component) *CollectorInventory {
	return &CollectorInventory{
		collectors: []collectors.CollectorVersions{
			k8sCollectors.NewClusterCollectorVersions(),
			k8sCollectors.NewClusterRoleCollectorVersions(),
			k8sCollectors.NewClusterRoleBindingCollectorVersions(),
			k8sCollectors.NewCRDCollectorVersions(),
			k8sCollectors.NewCronJobCollectorVersions(),
			k8sCollectors.NewDaemonSetCollectorVersions(),
			k8sCollectors.NewDeploymentCollectorVersions(),
			k8sCollectors.NewIngressCollectorVersions(),
			k8sCollectors.NewJobCollectorVersions(),
			k8sCollectors.NewLimitRangeCollectorVersions(),
			k8sCollectors.NewNamespaceCollectorVersions(),
			k8sCollectors.NewNodeCollectorVersions(),
			k8sCollectors.NewPersistentVolumeCollectorVersions(),
			k8sCollectors.NewPersistentVolumeClaimCollectorVersions(),
			k8sCollectors.NewReplicaSetCollectorVersions(),
			k8sCollectors.NewRoleCollectorVersions(),
			k8sCollectors.NewRoleBindingCollectorVersions(),
			k8sCollectors.NewServiceCollectorVersions(),
			k8sCollectors.NewServiceAccountCollectorVersions(),
			k8sCollectors.NewStatefulSetCollectorVersions(),
			k8sCollectors.NewStorageClassCollectorVersions(),
			k8sCollectors.NewUnassignedPodCollectorVersions(cfg, store, tagger),
			k8sCollectors.NewVerticalPodAutoscalerCollectorVersions(),
			k8sCollectors.NewHorizontalPodAutoscalerCollectorVersions(),
			k8sCollectors.NewNetworkPolicyCollectorVersions(),
		},
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
func (ci *CollectorInventory) CollectorForVersion(collectorName, collectorVersion string) (collectors.K8sCollector, error) {
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
