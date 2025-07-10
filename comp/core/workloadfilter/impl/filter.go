// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloadfilterimpl contains the implementation of the filter component.
package workloadfilterimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/catalog"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// filter is the implementation of the filter component.
type filter struct {
	config    config.Component
	log       log.Component
	telemetry coretelemetry.Component
	prgs      map[workloadfilter.ResourceType]map[int]program.FilterProgram
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

	Comp workloadfilter.Component
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

var _ workloadfilter.Component = (*filter)(nil)

func (f *filter) registerProgram(resourceType workloadfilter.ResourceType, programType int, prg program.FilterProgram) {
	if f.prgs[resourceType] == nil {
		f.prgs[resourceType] = make(map[int]program.FilterProgram)
	}
	f.prgs[resourceType][programType] = prg
}

func (f *filter) getProgram(resourceType workloadfilter.ResourceType, programType int) program.FilterProgram {
	if f.prgs == nil {
		return nil
	}
	if programs, ok := f.prgs[resourceType]; ok {
		return programs[programType]
	}
	return nil
}

func newFilter(config config.Component, logger log.Component, telemetry coretelemetry.Component) (workloadfilter.Component, error) {
	filter := &filter{
		config:    config,
		log:       logger,
		telemetry: telemetry,
		prgs:      make(map[workloadfilter.ResourceType]map[int]program.FilterProgram),
	}

	// Container Filters
	filter.registerProgram(workloadfilter.ContainerType, int(workloadfilter.LegacyContainerMetrics), catalog.LegacyContainerMetricsProgram(config, logger))
	filter.registerProgram(workloadfilter.ContainerType, int(workloadfilter.LegacyContainerLogs), catalog.LegacyContainerLogsProgram(config, logger))
	filter.registerProgram(workloadfilter.ContainerType, int(workloadfilter.LegacyContainerACInclude), catalog.LegacyContainerACIncludeProgram(config, logger))
	filter.registerProgram(workloadfilter.ContainerType, int(workloadfilter.LegacyContainerACExclude), catalog.LegacyContainerACExcludeProgram(config, logger))
	filter.registerProgram(workloadfilter.ContainerType, int(workloadfilter.LegacyContainerGlobal), catalog.LegacyContainerGlobalProgram(config, logger))
	filter.registerProgram(workloadfilter.ContainerType, int(workloadfilter.LegacyContainerSBOM), catalog.LegacyContainerSBOMProgram(config, logger))

	filter.registerProgram(workloadfilter.ContainerType, int(workloadfilter.ContainerADAnnotations), catalog.ContainerADAnnotationsProgram(config, logger))
	filter.registerProgram(workloadfilter.ContainerType, int(workloadfilter.ContainerADAnnotationsMetrics), catalog.ContainerADAnnotationsMetricsProgram(config, logger))
	filter.registerProgram(workloadfilter.ContainerType, int(workloadfilter.ContainerADAnnotationsLogs), catalog.ContainerADAnnotationsLogsProgram(config, logger))
	filter.registerProgram(workloadfilter.ContainerType, int(workloadfilter.ContainerPaused), catalog.ContainerPausedProgram(config, logger))

	// Service Filters
	filter.registerProgram(workloadfilter.ServiceType, int(workloadfilter.LegacyServiceGlobal), catalog.LegacyServiceGlobalProgram(config, logger))
	filter.registerProgram(workloadfilter.ServiceType, int(workloadfilter.LegacyServiceMetrics), catalog.LegacyServiceMetricsProgram(config, logger))

	// Endpoints Filters
	filter.registerProgram(workloadfilter.EndpointType, int(workloadfilter.LegacyEndpointGlobal), catalog.LegacyEndpointsGlobalProgram(config, logger))
	filter.registerProgram(workloadfilter.EndpointType, int(workloadfilter.LegacyEndpointMetrics), catalog.LegacyEndpointsMetricsProgram(config, logger))

	// WIP: Pod Filters

	return filter, nil
}

// IsContainerExcluded checks if a container is excluded based on the provided filters.
func (f *filter) IsContainerExcluded(container *workloadfilter.Container, containerFilters [][]workloadfilter.ContainerFilter) bool {
	return evaluateResource(f, container, containerFilters) == workloadfilter.Excluded
}

// IsPodExcluded checks if a pod is excluded based on the provided filters.
func (f *filter) IsPodExcluded(pod *workloadfilter.Pod, podFilters [][]workloadfilter.PodFilter) bool {
	return evaluateResource(f, pod, podFilters) == workloadfilter.Excluded
}

func (f *filter) IsServiceExcluded(service *workloadfilter.Service, serviceFilters [][]workloadfilter.ServiceFilter) bool {
	return evaluateResource(f, service, serviceFilters) == workloadfilter.Excluded
}

func (f *filter) IsEndpointExcluded(endpoint *workloadfilter.Endpoint, endpointFilters [][]workloadfilter.EndpointFilter) bool {
	return evaluateResource(f, endpoint, endpointFilters) == workloadfilter.Excluded
}

// evaluateResource checks if a resource is excluded based on the provided filters.
func evaluateResource[T ~int](
	f *filter,
	resource workloadfilter.Filterable, // Filterable resource (e.g., Container, Pod)
	filterSets [][]T, // Generic filter types
) workloadfilter.Result {
	for _, filterSet := range filterSets {
		var setResult = workloadfilter.Unknown
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
			if res == workloadfilter.Included {
				f.log.Debugf("Resource %s is included by filter %d", resource.Type(), filter)
				return res
			}
			if res == workloadfilter.Excluded {
				setResult = workloadfilter.Excluded
			}
		}
		// If the set of filters produces a Include/Exclude result,
		// then return the set's results and don't execute subsequent sets.
		if setResult != workloadfilter.Unknown {
			return setResult
		}
	}
	return workloadfilter.Unknown
}

// GetContainerFilterInitializationErrors returns initialization errors for a specific container filter
func (f *filter) GetContainerFilterInitializationErrors(filters []workloadfilter.ContainerFilter) []error {
	return getFilterErrors(f, workloadfilter.ContainerType, filters)
}

// getFilterErrors returns initialization errors for a specific filter
func getFilterErrors[T ~int](
	f *filter,
	resourceType workloadfilter.ResourceType, // Filterable resource (e.g., Container, Pod)
	filters []T, // Generic filter types
) []error {
	errs := []error{}
	for _, filter := range filters {
		prg := f.getProgram(resourceType, int(filter))
		if prg == nil {
			continue
		}
		errs = append(errs, prg.GetInitializationErrors()...)
	}
	return errs
}
