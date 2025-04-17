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
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

// CollectorInventory is used to store and retrieve available collectors.
type CollectorInventory struct {
	collectors []collectors.CollectorVersions
}

// NewCollectorInventory returns a new inventory containing all known
// collectors.
func NewCollectorInventory(cfg config.Component, store workloadmeta.Component, tagger tagger.Component) *CollectorInventory {
	metadataAsTags := utils.GetMetadataAsTags(cfg)
	return &CollectorInventory{
		collectors: []collectors.CollectorVersions{
			k8sCollectors.NewCRDCollectorVersions(),
			k8sCollectors.NewClusterCollectorVersions(),
			k8sCollectors.NewClusterRoleBindingCollectorVersions(metadataAsTags),
			k8sCollectors.NewClusterRoleCollectorVersions(metadataAsTags),
			k8sCollectors.NewCronJobCollectorVersions(metadataAsTags),
			k8sCollectors.NewDaemonSetCollectorVersions(metadataAsTags),
			k8sCollectors.NewDeploymentCollectorVersions(metadataAsTags),
			k8sCollectors.NewHorizontalPodAutoscalerCollectorVersions(metadataAsTags),
			k8sCollectors.NewIngressCollectorVersions(metadataAsTags),
			k8sCollectors.NewJobCollectorVersions(metadataAsTags),
			k8sCollectors.NewLimitRangeCollectorVersions(metadataAsTags),
			k8sCollectors.NewNamespaceCollectorVersions(metadataAsTags),
			k8sCollectors.NewNetworkPolicyCollectorVersions(metadataAsTags),
			k8sCollectors.NewNodeCollectorVersions(metadataAsTags),
			k8sCollectors.NewPersistentVolumeClaimCollectorVersions(metadataAsTags),
			k8sCollectors.NewPersistentVolumeCollectorVersions(metadataAsTags),
			k8sCollectors.NewPodDisruptionBudgetCollectorVersions(metadataAsTags),
			k8sCollectors.NewReplicaSetCollectorVersions(metadataAsTags),
			k8sCollectors.NewRoleBindingCollectorVersions(metadataAsTags),
			k8sCollectors.NewRoleCollectorVersions(metadataAsTags),
			k8sCollectors.NewServiceAccountCollectorVersions(metadataAsTags),
			k8sCollectors.NewServiceCollectorVersions(metadataAsTags),
			k8sCollectors.NewStatefulSetCollectorVersions(metadataAsTags),
			k8sCollectors.NewStorageClassCollectorVersions(metadataAsTags),
			k8sCollectors.NewUnassignedPodCollectorVersions(cfg, store, tagger, metadataAsTags),
			k8sCollectors.NewTerminatedPodCollectorVersions(cfg, store, tagger, metadataAsTags),
			k8sCollectors.NewVerticalPodAutoscalerCollectorVersions(metadataAsTags),
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
