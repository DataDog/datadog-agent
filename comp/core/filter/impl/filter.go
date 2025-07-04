// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filterimpl contains the implementation of the filter component.
package filterimpl

import (
	config "github.com/DataDog/datadog-agent/comp/core/config/def"
	"github.com/DataDog/datadog-agent/comp/core/filter/catalog"
	filterdef "github.com/DataDog/datadog-agent/comp/core/filter/def"
	"github.com/DataDog/datadog-agent/comp/core/filter/program"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// filter is the implementation of the filter component.
type filter struct {
	config    config.Component
	log       log.Component
	telemetry coretelemetry.Component
	prgs      map[filterdef.ResourceType]map[int]program.FilterProgram
}

// Requires defines the dependencies of the filter component.
type Requires struct {
	compdef.In

	Lc        compdef.Lifecycle
	Config    config.Component
	Log       log.Component
	Telemetry coretelemetry.Component
}

// Provides contains the fields provided by the filter constructor.
type Provides struct {
	compdef.Out

	Comp filterdef.Component
}

// NewComponent returns a new filter client
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

func (f *filter) registerProgram(resourceType filterdef.ResourceType, programType int, prg program.FilterProgram) {
	if f.prgs[resourceType] == nil {
		f.prgs[resourceType] = make(map[int]program.FilterProgram)
	}
	f.prgs[resourceType][programType] = prg
}

func (f *filter) getProgram(resourceType filterdef.ResourceType, programType int) program.FilterProgram {
	if f.prgs == nil {
		return nil
	}
	if programs, ok := f.prgs[resourceType]; ok {
		return programs[programType]
	}
	return nil
}

func newFilter(config config.Component, logger log.Component, telemetry coretelemetry.Component) (filterdef.Component, error) {
	filter := &filter{
		config:    config,
		log:       logger,
		telemetry: telemetry,
		prgs:      make(map[filterdef.ResourceType]map[int]program.FilterProgram),
	}

	// Container Filters
	filter.registerProgram(filterdef.ContainerType, int(filterdef.LegacyContainerMetrics), catalog.LegacyContainerMetricsProgram(config, logger))
	filter.registerProgram(filterdef.ContainerType, int(filterdef.LegacyContainerLogs), catalog.LegacyContainerLogsProgram(config, logger))
	filter.registerProgram(filterdef.ContainerType, int(filterdef.LegacyContainerACInclude), catalog.LegacyContainerACIncludeProgram(config, logger))
	filter.registerProgram(filterdef.ContainerType, int(filterdef.LegacyContainerACExclude), catalog.LegacyContainerACExcludeProgram(config, logger))
	filter.registerProgram(filterdef.ContainerType, int(filterdef.LegacyContainerGlobal), catalog.LegacyContainerGlobalProgram(config, logger))
	filter.registerProgram(filterdef.ContainerType, int(filterdef.LegacyContainerSBOM), catalog.LegacyContainerSBOMProgram(config, logger))

	filter.registerProgram(filterdef.ContainerType, int(filterdef.ContainerADAnnotations), catalog.ContainerADAnnotationsProgram(config, logger))
	filter.registerProgram(filterdef.ContainerType, int(filterdef.ContainerPaused), catalog.ContainerPausedProgram(config, logger))

	// WIP: Pod Filters

	return filter, nil
}

// IsContainerExcluded checks if a container is excluded based on the provided filters.
func (f *filter) IsContainerExcluded(container *filterdef.Container, containerFilters [][]filterdef.ContainerFilter) bool {
	return evaluateResource(f, container, containerFilters) == filterdef.Excluded
}

// IsPodExcluded checks if a pod is excluded based on the provided filters.
func (f *filter) IsPodExcluded(pod *filterdef.Pod, podFilters [][]filterdef.PodFilter) bool {
	return evaluateResource(f, pod, podFilters) == filterdef.Excluded
}

// evaluateResource checks if a resource is excluded based on the provided filters.
func evaluateResource[T ~int](
	f *filter,
	resource filterdef.Filterable, // Filterable resource (e.g., Container, Pod)
	filterSets [][]T, // Generic filter types
) filterdef.Result {
	for _, filterSet := range filterSets {
		var setResult = filterdef.Unknown
		for _, filter := range filterSet {

			// 1. Retrieve the filtering program
			prg := f.getProgram(resource.Type(), int(filter))
			if prg == nil {
				f.log.Warnf("No program found for filter %d on resource %s", filter, resource.Type())
				continue
			}

			// 2. Evaluate the filtering program
			res, prgErrs := prg.Evaluate(resource)
			if prgErrs != nil {
				f.log.Debug(prgErrs)
			}

			// 3. Process the results
			if res == filterdef.Included {
				f.log.Debugf("Resource %s is included by filter %d", resource.Type(), filter)
				return res
			}
			if res == filterdef.Excluded {
				setResult = filterdef.Excluded
			}
		}
		// If the set of filters produces a Include/Exclude result,
		// then return the set's results and don't execute subsequent sets.
		if setResult != filterdef.Unknown {
			return setResult
		}
	}
	return filterdef.Unknown
}
