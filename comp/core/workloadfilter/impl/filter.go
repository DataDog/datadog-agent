// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloadfilterimpl contains the implementation of the filter component.
package workloadfilterimpl

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/catalog"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// filterFactory holds a factory function and ensures it's called only once
type filterFactory struct {
	once    sync.Once
	program program.FilterProgram
	factory func(cfg config.Component, logger log.Component) program.FilterProgram
}

// filter is the implementation of the filter component.
type filter struct {
	config              config.Component
	log                 log.Component
	telemetry           coretelemetry.Component
	programFactoryStore map[workloadfilter.ResourceType]map[int]*filterFactory
	selection           *filterSelection
}

// Requires defines the dependencies of the filter component.
type Requires struct {
	compdef.In

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

func (f *filter) registerFactory(resourceType workloadfilter.ResourceType, programType int, factory func(cfg config.Component, logger log.Component) program.FilterProgram) {
	if f.programFactoryStore[resourceType] == nil {
		f.programFactoryStore[resourceType] = make(map[int]*filterFactory)
	}
	f.programFactoryStore[resourceType][programType] = &filterFactory{
		factory: factory,
	}
}

func (f *filter) getProgram(resourceType workloadfilter.ResourceType, programType int) program.FilterProgram {
	if f.programFactoryStore == nil {
		return nil
	}

	programFactories, ok := f.programFactoryStore[resourceType]
	if !ok {
		return nil
	}

	factory, ok := programFactories[programType]
	if !ok {
		return nil
	}

	factory.once.Do(func() {
		factory.program = factory.factory(f.config, f.log)
	})

	return factory.program
}

func newFilter(cfg config.Component, logger log.Component, telemetry coretelemetry.Component) (workloadfilter.Component, error) {
	filter := &filter{
		config:              cfg,
		log:                 logger,
		telemetry:           telemetry,
		programFactoryStore: make(map[workloadfilter.ResourceType]map[int]*filterFactory),
		selection:           newFilterSelection(cfg),
	}

	genericADProgram := catalog.AutodiscoveryAnnotations()
	genericADMetricsProgram := catalog.AutodiscoveryMetricsAnnotations()
	genericADLogsProgram := catalog.AutodiscoveryLogsAnnotations()
	genericADProgramFactory := func(_ config.Component, _ log.Component) program.FilterProgram { return genericADProgram }
	genericADMetricsProgramFactory := func(_ config.Component, _ log.Component) program.FilterProgram { return genericADMetricsProgram }
	genericADLogsProgramFactory := func(_ config.Component, _ log.Component) program.FilterProgram { return genericADLogsProgram }

	// Container Filters
	filter.registerFactory(workloadfilter.ContainerType, int(workloadfilter.LegacyContainerMetrics), catalog.LegacyContainerMetricsProgram)
	filter.registerFactory(workloadfilter.ContainerType, int(workloadfilter.LegacyContainerLogs), catalog.LegacyContainerLogsProgram)
	filter.registerFactory(workloadfilter.ContainerType, int(workloadfilter.LegacyContainerACInclude), catalog.LegacyContainerACIncludeProgram)
	filter.registerFactory(workloadfilter.ContainerType, int(workloadfilter.LegacyContainerACExclude), catalog.LegacyContainerACExcludeProgram)
	filter.registerFactory(workloadfilter.ContainerType, int(workloadfilter.LegacyContainerGlobal), catalog.LegacyContainerGlobalProgram)
	filter.registerFactory(workloadfilter.ContainerType, int(workloadfilter.LegacyContainerSBOM), catalog.LegacyContainerSBOMProgram)

	filter.registerFactory(workloadfilter.ContainerType, int(workloadfilter.ContainerADAnnotations), genericADProgramFactory)
	filter.registerFactory(workloadfilter.ContainerType, int(workloadfilter.ContainerADAnnotationsMetrics), genericADMetricsProgramFactory)
	filter.registerFactory(workloadfilter.ContainerType, int(workloadfilter.ContainerADAnnotationsLogs), genericADLogsProgramFactory)
	filter.registerFactory(workloadfilter.ContainerType, int(workloadfilter.ContainerPaused), catalog.ContainerPausedProgram)

	// Service Filters
	filter.registerFactory(workloadfilter.ServiceType, int(workloadfilter.LegacyServiceGlobal), catalog.LegacyServiceGlobalProgram)
	filter.registerFactory(workloadfilter.ServiceType, int(workloadfilter.LegacyServiceMetrics), catalog.LegacyServiceMetricsProgram)
	filter.registerFactory(workloadfilter.ServiceType, int(workloadfilter.ServiceADAnnotations), genericADProgramFactory)
	filter.registerFactory(workloadfilter.ServiceType, int(workloadfilter.ServiceADAnnotationsMetrics), genericADMetricsProgramFactory)

	// Endpoints Filters
	filter.registerFactory(workloadfilter.EndpointType, int(workloadfilter.LegacyEndpointGlobal), catalog.LegacyEndpointsGlobalProgram)
	filter.registerFactory(workloadfilter.EndpointType, int(workloadfilter.LegacyEndpointMetrics), catalog.LegacyEndpointsMetricsProgram)
	filter.registerFactory(workloadfilter.EndpointType, int(workloadfilter.EndpointADAnnotations), genericADProgramFactory)
	filter.registerFactory(workloadfilter.EndpointType, int(workloadfilter.EndpointADAnnotationsMetrics), genericADMetricsProgramFactory)

	// Pod Filters
	filter.registerFactory(workloadfilter.PodType, int(workloadfilter.LegacyPod), catalog.LegacyPodProgram)
	filter.registerFactory(workloadfilter.PodType, int(workloadfilter.PodADAnnotations), genericADProgramFactory)
	filter.registerFactory(workloadfilter.PodType, int(workloadfilter.PodADAnnotationsMetrics), genericADMetricsProgramFactory)

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

// GetContainerAutodiscoveryFilters returns the pre-computed container autodiscovery filters
func (f *filter) GetContainerAutodiscoveryFilters(filterScope workloadfilter.Scope) [][]workloadfilter.ContainerFilter {
	return f.selection.GetContainerAutodiscoveryFilters(filterScope)
}

// GetPodAutodiscoveryFilters returns the pre-computed pod autodiscovery filters
func (f *filter) GetPodAutodiscoveryFilters(filterScope workloadfilter.Scope) [][]workloadfilter.PodFilter {
	return f.selection.GetPodAutodiscoveryFilters(filterScope)
}

// GetServiceAutodiscoveryFilters returns the pre-computed service autodiscovery filters
func (f *filter) GetServiceAutodiscoveryFilters(filterScope workloadfilter.Scope) [][]workloadfilter.ServiceFilter {
	return f.selection.GetServiceAutodiscoveryFilters(filterScope)
}

// GetEndpointAutodiscoveryFilters returns the pre-computed endpoint autodiscovery filters
func (f *filter) GetEndpointAutodiscoveryFilters(filterScope workloadfilter.Scope) [][]workloadfilter.EndpointFilter {
	return f.selection.GetEndpointAutodiscoveryFilters(filterScope)
}

// GetContainerSharedMetricFilters returns the pre-computed container shared metric filters
func (f *filter) GetContainerSharedMetricFilters() [][]workloadfilter.ContainerFilter {
	return f.selection.GetContainerSharedMetricFilters()
}

// GetPodSharedMetricFilters returns the pre-computed pod shared metric filters
func (f *filter) GetPodSharedMetricFilters() [][]workloadfilter.PodFilter {
	return f.selection.GetPodSharedMetricFilters()
}

// GetContainerPausedFilters returns the pre-computed container paused filters
func (f *filter) GetContainerPausedFilters() [][]workloadfilter.ContainerFilter {
	return f.selection.GetContainerPausedFilters()
}

// GetContainerSBOMFilters returns the pre-computed container SBOM filters
func (f *filter) GetContainerSBOMFilters() [][]workloadfilter.ContainerFilter {
	return f.selection.GetContainerSBOMFilters()
}
