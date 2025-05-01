// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filterimpl contains the implementation of the filter component.
package filterimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	catalog "github.com/DataDog/datadog-agent/comp/core/filter/catalog"
	common "github.com/DataDog/datadog-agent/comp/core/filter/common"
	filterdef "github.com/DataDog/datadog-agent/comp/core/filter/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// filter is the implementation of the filter component.
type filter struct {
	config    config.Component
	log       log.Component
	telemetry coretelemetry.Component
	prgs      map[filterdef.ResourceType]map[int]common.FilterProgram
}

// Requires defines the dependencies of the filter component.
type Requires struct {
	compdef.In

	Lc        compdef.Lifecycle
	Config    config.Component
	Log       log.Component
	Telemetry coretelemetry.Component
}

// Provides contains the fields provided by the tagger constructor.
type Provides struct {
	compdef.Out

	Comp filterdef.Component
}

// NewComponent returns a new tagger client
func NewComponent(req Requires) (Provides, error) {
	filterInstance, err := newFilter(req.Config, req.Log, req.Telemetry)
	if err != nil {
		return Provides{}, err
	}

	return Provides{
		Comp: filterInstance,
	}, nil
}

var _ filterdef.Component = (*filter)(nil)

func newFilter(config config.Component, logger log.Component, telemetry coretelemetry.Component) (filterdef.Component, error) {
	// Initialize the filter component
	prgs := map[filterdef.ResourceType]map[int]common.FilterProgram{
		filterdef.ContainerKey: {
			int(filterdef.ContainerMetrics):       catalog.ContainerMetricsProgram(config, logger),
			int(filterdef.ContainerLogs):          catalog.ContainerLogsProgram(config, logger),
			int(filterdef.ContainerACLegacy):      catalog.ContainerACLegacyProgram(config, logger),
			int(filterdef.ContainerADAnnotations): catalog.ContainerADAnnotationsProgram(config, logger),
			int(filterdef.ContainerGlobal):        catalog.ContainerGlobalProgram(config, logger),
			int(filterdef.ContainerPaused):        catalog.ContainerPausedProgram(config, logger),
			int(filterdef.ContainerSBOM):          catalog.ContainerSBOMProgram(config, logger),
		},
	}

	filter := &filter{
		config:    config,
		log:       logger,
		telemetry: telemetry,
		prgs:      prgs,
	}

	return filter, nil
}

// IsContainerExcluded checks if a container is excluded based on the provided filters.
func (f *filter) IsContainerExcluded(container filterdef.Container, containerFilters []filterdef.ContainerFilter, defaultValue bool) (bool, error) {
	return f.isResourceExcluded(container, convertFiltersToInts(containerFilters), defaultValue), nil
}

// IsPodExcluded checks if a pod is excluded based on the provided filters.
func (f *filter) IsPodExcluded(pod filterdef.Pod, podFilters []filterdef.PodFilter, defaultValue bool) (bool, error) {
	return f.isResourceExcluded(pod, convertFiltersToInts(podFilters), defaultValue), nil
}

// isResourceExcluded checks if a resource is excluded based on the provided filters.
func (f *filter) isResourceExcluded(
	resource filterdef.Filterable, // Generic resource (e.g., Container, Pod)
	filters []int, // Generic filter types
	defaultValue bool,
) bool {
	isExcluded := defaultValue
	for _, filter := range filters {
		prg := f.prgs[resource.Key()][filter]
		if prg == nil {
			continue
		}
		res, err := prg.IsExcluded(string(resource.Key()), resource.ToMap())
		if err != nil {
			f.log.Warnf("Error evaluating filter %d for resource %s: %v", filter, resource.Key(), err)
			continue
		}
		if res == common.Included {
			f.log.Debugf("Resource %s is included by filter %d", resource.Key(), filter)
			return false
		}
		if res == common.Excluded {
			isExcluded = true
		}
	}
	return isExcluded
}

// convertFiltersToInts converts a slice of filters to a slice of ints.
func convertFiltersToInts[T ~int](filters []T) []int {
	intFilters := make([]int, len(filters))
	for i, filter := range filters {
		intFilters[i] = int(filter)
	}
	return intFilters
}
