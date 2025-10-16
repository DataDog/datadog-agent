// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloadfilterimpl contains the implementation of the filter component.
package workloadfilterimpl

import (
	"os"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/catalog"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// filterProgramFactory holds a factory function and ensures it's called only once
type filterProgramFactory struct {
	once    sync.Once
	program program.FilterProgram
	factory func(filterConfig *catalog.FilterConfig, logger logcomp.Component) program.FilterProgram
}

// workloadfilterStore is the implementation of the workloadfilterStore component.
type workloadfilterStore struct {
	config              config.Component
	log                 logcomp.Component
	telemetry           coretelemetry.Component
	programFactoryStore map[workloadfilter.ResourceType]map[int]*filterProgramFactory
	selection           *filterSelection
	// Pre-built filter configuration with all parsed values
	filterConfig *catalog.FilterConfig
}

// Requires defines the dependencies of the filter component.
type Requires struct {
	compdef.In

	Config    config.Component
	Log       logcomp.Component
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

var _ workloadfilter.Component = (*workloadfilterStore)(nil)

func (f *workloadfilterStore) registerFactory(resourceType workloadfilter.ResourceType, programType int, factory func(filterConfig *catalog.FilterConfig, logger logcomp.Component) program.FilterProgram) {
	if f.programFactoryStore[resourceType] == nil {
		f.programFactoryStore[resourceType] = make(map[int]*filterProgramFactory)
	}
	f.programFactoryStore[resourceType][programType] = &filterProgramFactory{
		factory: factory,
	}
}

func (f *workloadfilterStore) getProgram(resourceType workloadfilter.ResourceType, programType int) program.FilterProgram {
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
		factory.program = factory.factory(f.filterConfig, f.log)
	})

	return factory.program
}

func newFilter(cfg config.Component, logger logcomp.Component, telemetry coretelemetry.Component) (workloadfilter.Component, error) {
	filterConfig, configErr := catalog.NewFilterConfig(cfg)
	if configErr != nil {
		logger.Criticalf("failed to parse 'cel_workload_exclude' filters. Provided value: \n%s\nError: %v", cfg.Get("cel_workload_exclude"), configErr)
		logger.Flush()
		os.Exit(1)
	}

	filter := &workloadfilterStore{
		config:              cfg,
		log:                 logger,
		telemetry:           telemetry,
		programFactoryStore: make(map[workloadfilter.ResourceType]map[int]*filterProgramFactory),
		selection:           newFilterSelection(cfg),
		filterConfig:        filterConfig,
	}

	genericADProgram := catalog.AutodiscoveryAnnotations()
	genericADMetricsProgram := catalog.AutodiscoveryMetricsAnnotations()
	genericADLogsProgram := catalog.AutodiscoveryLogsAnnotations()
	genericADProgramFactory := func(_ *catalog.FilterConfig, _ logcomp.Component) program.FilterProgram { return genericADProgram }
	genericADMetricsProgramFactory := func(_ *catalog.FilterConfig, _ logcomp.Component) program.FilterProgram {
		return genericADMetricsProgram
	}
	genericADLogsProgramFactory := func(_ *catalog.FilterConfig, _ logcomp.Component) program.FilterProgram { return genericADLogsProgram }

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

	filter.registerFactory(workloadfilter.ContainerType, int(workloadfilter.ContainerCELMetrics), catalog.ContainerCELMetricsProgram)
	filter.registerFactory(workloadfilter.ContainerType, int(workloadfilter.ContainerCELLogs), catalog.ContainerCELLogsProgram)
	filter.registerFactory(workloadfilter.ContainerType, int(workloadfilter.ContainerCELSBOM), catalog.ContainerCELSBOMProgram)
	filter.registerFactory(workloadfilter.ContainerType, int(workloadfilter.ContainerCELGlobal), catalog.ContainerCELGlobalProgram)

	// Service Filters
	filter.registerFactory(workloadfilter.ServiceType, int(workloadfilter.LegacyServiceGlobal), catalog.LegacyServiceGlobalProgram)
	filter.registerFactory(workloadfilter.ServiceType, int(workloadfilter.LegacyServiceMetrics), catalog.LegacyServiceMetricsProgram)
	filter.registerFactory(workloadfilter.ServiceType, int(workloadfilter.ServiceADAnnotations), genericADProgramFactory)
	filter.registerFactory(workloadfilter.ServiceType, int(workloadfilter.ServiceADAnnotationsMetrics), genericADMetricsProgramFactory)

	filter.registerFactory(workloadfilter.ServiceType, int(workloadfilter.ServiceCELMetrics), catalog.ServiceCELMetricsProgram)
	filter.registerFactory(workloadfilter.ServiceType, int(workloadfilter.ServiceCELGlobal), catalog.ServiceCELGlobalProgram)

	// Endpoints Filters
	filter.registerFactory(workloadfilter.EndpointType, int(workloadfilter.LegacyEndpointGlobal), catalog.LegacyEndpointsGlobalProgram)
	filter.registerFactory(workloadfilter.EndpointType, int(workloadfilter.LegacyEndpointMetrics), catalog.LegacyEndpointsMetricsProgram)
	filter.registerFactory(workloadfilter.EndpointType, int(workloadfilter.EndpointADAnnotations), genericADProgramFactory)
	filter.registerFactory(workloadfilter.EndpointType, int(workloadfilter.EndpointADAnnotationsMetrics), genericADMetricsProgramFactory)

	filter.registerFactory(workloadfilter.EndpointType, int(workloadfilter.EndpointCELMetrics), catalog.EndpointCELMetricsProgram)
	filter.registerFactory(workloadfilter.EndpointType, int(workloadfilter.EndpointCELGlobal), catalog.EndpointCELGlobalProgram)

	// Pod Filters
	filter.registerFactory(workloadfilter.PodType, int(workloadfilter.LegacyPod), catalog.LegacyPodProgram)
	filter.registerFactory(workloadfilter.PodType, int(workloadfilter.PodADAnnotations), genericADProgramFactory)
	filter.registerFactory(workloadfilter.PodType, int(workloadfilter.PodADAnnotationsMetrics), genericADMetricsProgramFactory)

	filter.registerFactory(workloadfilter.PodType, int(workloadfilter.PodCELMetrics), catalog.PodCELMetricsProgram)
	filter.registerFactory(workloadfilter.PodType, int(workloadfilter.PodCELGlobal), catalog.PodCELGlobalProgram)

	// Process Filters
	filter.registerFactory(workloadfilter.ProcessType, int(workloadfilter.LegacyProcessExcludeList), catalog.LegacyProcessExcludeProgram)

	return filter, nil
}

// GetContainerAutodiscoveryFilters returns the pre-computed container autodiscovery filters
func (f *workloadfilterStore) GetContainerAutodiscoveryFilters(filterScope workloadfilter.Scope) workloadfilter.FilterBundle {
	return f.GetContainerFilters(f.selection.GetContainerAutodiscoveryFilters(filterScope))
}

// GetServiceAutodiscoveryFilters returns the pre-computed service autodiscovery filters
func (f *workloadfilterStore) GetServiceAutodiscoveryFilters(filterScope workloadfilter.Scope) workloadfilter.FilterBundle {
	return f.GetServiceFilters(f.selection.GetServiceAutodiscoveryFilters(filterScope))
}

// GetEndpointAutodiscoveryFilters returns the pre-computed endpoint autodiscovery filters
func (f *workloadfilterStore) GetEndpointAutodiscoveryFilters(filterScope workloadfilter.Scope) workloadfilter.FilterBundle {
	return f.GetEndpointFilters(f.selection.GetEndpointAutodiscoveryFilters(filterScope))
}

// GetContainerSharedMetricFilters returns the pre-computed container shared metric filters
func (f *workloadfilterStore) GetContainerSharedMetricFilters() workloadfilter.FilterBundle {
	return f.GetContainerFilters(f.selection.GetContainerSharedMetricFilters())
}

// GetContainerPausedFilters returns the pre-computed container paused filters
func (f *workloadfilterStore) GetContainerPausedFilters() workloadfilter.FilterBundle {
	return f.GetContainerFilters(f.selection.GetContainerPausedFilters())
}

// GetPodSharedMetricFilters returns the pre-computed pod shared metric filters
func (f *workloadfilterStore) GetPodSharedMetricFilters() workloadfilter.FilterBundle {
	return f.GetPodFilters(f.selection.GetPodSharedMetricFilters())
}

// GetContainerSBOMFilters returns the pre-computed container SBOM filters
func (f *workloadfilterStore) GetContainerSBOMFilters() workloadfilter.FilterBundle {
	return f.GetContainerFilters(f.selection.GetContainerSBOMFilters())
}

// GetContainerFilters returns the filter bundle for the given container filters
func (f *workloadfilterStore) GetContainerFilters(containerFilters [][]workloadfilter.ContainerFilter) workloadfilter.FilterBundle {
	return getFilterBundle(f, workloadfilter.ContainerType, containerFilters)
}

// GetPodFilters returns the filter bundle for the given pod filters
func (f *workloadfilterStore) GetPodFilters(podFilters [][]workloadfilter.PodFilter) workloadfilter.FilterBundle {
	return getFilterBundle(f, workloadfilter.PodType, podFilters)
}

// GetServiceFilters returns the filter bundle for the given service filters
func (f *workloadfilterStore) GetServiceFilters(serviceFilters [][]workloadfilter.ServiceFilter) workloadfilter.FilterBundle {
	return getFilterBundle(f, workloadfilter.ServiceType, serviceFilters)
}

// GetEndpointFilters returns the filter bundle for the given endpoint filters
func (f *workloadfilterStore) GetEndpointFilters(endpointFilters [][]workloadfilter.EndpointFilter) workloadfilter.FilterBundle {
	return getFilterBundle(f, workloadfilter.EndpointType, endpointFilters)
}

func (f *workloadfilterStore) GetProcessFilters(processFilters [][]workloadfilter.ProcessFilter) workloadfilter.FilterBundle {
	return getFilterBundle(f, workloadfilter.ProcessType, processFilters)
}

// getFilterBundle constructs a filter bundle for a given resource type and filters.
func getFilterBundle[T ~int](f *workloadfilterStore, objType workloadfilter.ResourceType, filters [][]T) workloadfilter.FilterBundle {
	var filterSets [][]program.FilterProgram
	for _, filterSet := range filters {
		var set []program.FilterProgram
		for _, filter := range filterSet {
			prg := f.getProgram(objType, int(filter))
			if prg != nil {
				set = append(set, prg)
			}
		}
		filterSets = append(filterSets, set)
	}
	return &filterBundle{
		log:        f.log,
		filterSets: filterSets,
	}
}
