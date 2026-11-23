// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

//nolint:revive // TODO(CAPP) Fix revive linter
package inventory

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	k8sCollectors "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/registry"
)

// CollectorInventory is used to store and retrieve available collectors.
type CollectorInventory struct {
	collectors []collectors.CollectorVersions
}

// NewCollectorInventory returns an inventory containing the collectors of the combined and structured metadata
// pipelines, one for each version of the built-in resources of the registry.
func NewCollectorInventory(cfg config.Component, store workloadmeta.Component, tagger tagger.Component) *CollectorInventory {
	return &CollectorInventory{
		collectors: k8sCollectors.NewResourceCollectorVersions(registry.Dependencies{Config: cfg, Store: store, Tagger: tagger}, false),
	}
}

// NewManifestCollectorInventory returns an inventory containing the collectors of the manifest pipeline, one for each
// version of the built-in resources of the registry.
func NewManifestCollectorInventory(cfg config.Component, store workloadmeta.Component, tagger tagger.Component) *CollectorInventory {
	return &CollectorInventory{
		collectors: k8sCollectors.NewResourceCollectorVersions(registry.Dependencies{Config: cfg, Store: store, Tagger: tagger}, true),
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

// Groups returns the API groups of the collectors in the inventory.
func (ci *CollectorInventory) Groups() map[string]struct{} {
	groups := make(map[string]struct{})
	for _, cv := range ci.collectors {
		for _, c := range cv.Collectors {
			groups[c.Metadata().Group] = struct{}{}
		}
	}
	return groups
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
