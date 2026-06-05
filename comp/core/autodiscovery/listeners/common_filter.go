// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package listeners

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FilterableService is an interface for a subset of services that can use advanced filtering
type FilterableService interface {
	// GetFilterableEntity returns the workloadmeta entity used for filtering, or nil if not available
	GetFilterableEntity() workloadfilter.Filterable
}

// filterTemplatesMatched removes any config that does not match the service's filterable entity
func filterTemplatesMatched(svc FilterableService, configs map[string]integration.Config) {
	filterableEntity := svc.GetFilterableEntity()
	if filterableEntity != nil {
		for digest, config := range configs {
			if !config.IsMatched(filterableEntity) {
				delete(configs, digest)
			}
		}
	}
}

// filterTemplatesDiscovery drops configuration-discovery templates that are
// redundant with another config source for the same integration. Dropped when:
//  1. another check template (Instances > 0) for the same integration Name has
//     matched this same service (present in configs), or
//  2. a scheduled non-template (static) config exists for the same Name
//     (tracked in staticIdx).
//
// Logs-only sibling templates (no Instances) are ignored — discovery covers
// metric-check configuration and shouldn't be suppressed by an integration's
// log forwarding setup.
func filterTemplatesDiscovery(staticIdx *StaticConfigIndex, configs map[string]integration.Config) {
	if len(configs) == 0 {
		return
	}
	nonDiscoveryNames := map[string]struct{}{}
	for _, cfg := range configs {
		if !cfg.IsDiscovery() && len(cfg.Instances) > 0 {
			nonDiscoveryNames[cfg.Name] = struct{}{}
		}
	}
	for digest, cfg := range configs {
		if !cfg.IsDiscovery() {
			continue
		}
		_, hasSibling := nonDiscoveryNames[cfg.Name]
		if hasSibling || staticIdx.Has(cfg.Name) {
			log.Debugf("Ignoring discovery template %s from %s: another config source already covers this integration",
				cfg.Name, cfg.Source)
			delete(configs, digest)
		}
	}
}
