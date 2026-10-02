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
var defaultGenericResource = append([]k8sCollectors.GenericResource{
	{
		Name:             "endpointslices",
		Group:            "discovery.k8s.io",
		Version:          "v1",
		NodeType:         orchestrator.K8sEndpointSlice,
		Stable:           true,
		IsDefaultVersion: true,
	},
}, draGenericResources()...)

// draAPIVersions are the resource.k8s.io versions this collector understands,
// newest first. The group reached v1 only in Kubernetes 1.34, so a cluster in
// the field commonly serves a beta version instead — and discovery matches on
// the exact GroupVersion and skips a miss without logging, so registering v1
// alone means collecting nothing at all, silently, on every earlier cluster.
var draAPIVersions = []string{"v1", "v1beta2", "v1beta1"}

// draGenericResources registers DRA's ResourceClaim and ResourceSlice once per
// understood API version. Discovery dedupes by resource name and walks
// server-preferred group versions first, so exactly one version of each is
// enabled: whichever the API server prefers. Only the newest is the default
// version — see GenericResource.IsDefaultVersion for why that matters.
//
// Registered as non-stable, so they are collected only when the check names
// them explicitly. Stable would activate them on every cluster serving
// resource.k8s.io, including the ones whose Cluster Agent has no RBAC for the
// group yet: the LIST 403s, HasSynced never fires, and Initialize blocks for
// kube_cache_sync_timeout_seconds plus the extra sync timeout (70s by default)
// before skipCollector gives up on them. The permissions ship from
// helm-charts and datadog-operator, on their own release trains, so that
// window is real rather than hypothetical. Flip to stable once those carry the
// grant; until then this also matches kubernetes_state_core, where DRA sits
// behind collect_dra_resources.
func draGenericResources() []k8sCollectors.GenericResource {
	resources := make([]k8sCollectors.GenericResource, 0, 2*len(draAPIVersions))
	for _, name := range []string{"resourceclaims", "resourceslices"} {
		for i, version := range draAPIVersions {
			resources = append(resources, k8sCollectors.GenericResource{
				Name:             name,
				Group:            "resource.k8s.io",
				Version:          version,
				NodeType:         orchestrator.K8sCR,
				Stable:           false,
				IsDefaultVersion: i == 0,
			})
		}
	}
	return resources
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
			k8sCollectors.NewConfigMapCollectorVersions(tagger),
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
