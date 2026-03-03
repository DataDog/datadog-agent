// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package listeners

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
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
