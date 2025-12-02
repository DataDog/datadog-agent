// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package baseimpl contains the base implementation of the filter component.
package baseimpl

import (
	"os"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/catalog"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/telemetry"
)

// FilterProgramFactory holds a factory function and ensures it's called only once
type FilterProgramFactory struct {
	once    sync.Once
	program program.FilterProgram
	factory func(filterConfig *catalog.FilterConfig, logger logcomp.Component) program.FilterProgram
}

// ProgramFactory is an interface for creating filter programs
type ProgramFactory interface {
	// Get returns a filter program, either creating it or returning a cached instance
	Get(filterConfig *catalog.FilterConfig, logger logcomp.Component) program.FilterProgram
}

// BaseFilterStore is the base implementation of the filter store
type BaseFilterStore struct {
	Config              config.Component
	Log                 logcomp.Component
	ProgramFactoryStore map[workloadfilter.ResourceType]map[string]*FilterProgramFactory
	TelemetryStore      *telemetry.Store
	// Pre-built filter configuration with all parsed values
	selection    *filterSelection
	FilterConfig *catalog.FilterConfig
}

// NewBaseFilterStore creates the common base with all shared initialization
func NewBaseFilterStore(cfg config.Component, logger logcomp.Component, telemetryComp coretelemetry.Component) *BaseFilterStore {
	filterConfig, configErr := catalog.NewFilterConfig(cfg)
	if configErr != nil {
		logger.Criticalf("failed to parse 'cel_workload_exclude' filters. Provided value: \n%s\nError: %v", cfg.Get("cel_workload_exclude"), configErr)
		logger.Flush()
		os.Exit(1)
	}

	baseFilter := &BaseFilterStore{
		Config:              cfg,
		Log:                 logger,
		ProgramFactoryStore: make(map[workloadfilter.ResourceType]map[string]*FilterProgramFactory),
		selection:           newFilterSelection(cfg),
		FilterConfig:        filterConfig,
		TelemetryStore:      telemetry.NewStore(telemetryComp),
	}

	genericADProgram := catalog.AutodiscoveryAnnotations()
	genericADMetricsProgram := catalog.AutodiscoveryMetricsAnnotations()
	genericADLogsProgram := catalog.AutodiscoveryLogsAnnotations()
	genericADProgramFactory := func(_ *catalog.FilterConfig, _ logcomp.Component) program.FilterProgram { return genericADProgram }
	genericADMetricsProgramFactory := func(_ *catalog.FilterConfig, _ logcomp.Component) program.FilterProgram {
		return genericADMetricsProgram
	}
	genericADLogsProgramFactory := func(_ *catalog.FilterConfig, _ logcomp.Component) program.FilterProgram { return genericADLogsProgram }

	// Pre-compute legacy programs via `DD_CONTAINER_EXCLUDE*` that can be shared across entity types
	legacyGlobalPrg := catalog.LegacyContainerGlobalProgram(filterConfig, logger)
	legacyMetricsPrg := catalog.LegacyContainerMetricsProgram(filterConfig, logger)
	legacyLogsPrg := catalog.LegacyContainerLogsProgram(filterConfig, logger)
	legacyACIncludePrg := catalog.LegacyContainerACIncludeProgram(filterConfig, logger)
	legacyACExcludePrg := catalog.LegacyContainerACExcludeProgram(filterConfig, logger)
	legacyGlobalPrgFactory := func(_ *catalog.FilterConfig, _ logcomp.Component) program.FilterProgram { return legacyGlobalPrg }
	legacyMetricsPrgFactory := func(_ *catalog.FilterConfig, _ logcomp.Component) program.FilterProgram { return legacyMetricsPrg }
	legacyLogsPrgFactory := func(_ *catalog.FilterConfig, _ logcomp.Component) program.FilterProgram { return legacyLogsPrg }
	legacyACIncludePrgFactory := func(_ *catalog.FilterConfig, _ logcomp.Component) program.FilterProgram { return legacyACIncludePrg }
	legacyACExcludePrgFactory := func(_ *catalog.FilterConfig, _ logcomp.Component) program.FilterProgram { return legacyACExcludePrg }

	// Container Filters
	baseFilter.RegisterFactory(workloadfilter.ContainerLegacyMetrics, legacyMetricsPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerLegacyLogs, legacyLogsPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerLegacyACInclude, legacyACIncludePrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerLegacyACExclude, legacyACExcludePrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerLegacyGlobal, legacyGlobalPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerLegacySBOM, catalog.LegacyContainerSBOMProgram)

	baseFilter.RegisterFactory(workloadfilter.ContainerADAnnotations, genericADProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerADAnnotationsMetrics, genericADMetricsProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerADAnnotationsLogs, genericADLogsProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerPaused, catalog.ContainerPausedProgram)

	// Service Filters
	baseFilter.RegisterFactory(workloadfilter.ServiceLegacyGlobal, legacyGlobalPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ServiceLegacyMetrics, legacyMetricsPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ServiceADAnnotations, genericADProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.ServiceADAnnotationsMetrics, genericADMetricsProgramFactory)

	// Endpoints Filters
	baseFilter.RegisterFactory(workloadfilter.EndpointLegacyGlobal, legacyGlobalPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.EndpointLegacyMetrics, legacyMetricsPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.EndpointADAnnotations, genericADProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.EndpointADAnnotationsMetrics, genericADMetricsProgramFactory)

	// Pod Filters
	baseFilter.RegisterFactory(workloadfilter.PodLegacyMetrics, legacyMetricsPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.PodLegacyGlobal, legacyGlobalPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.PodADAnnotations, genericADProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.PodADAnnotationsMetrics, genericADMetricsProgramFactory)

	// Process Filters
	baseFilter.RegisterFactory(workloadfilter.ProcessType, string(workloadfilter.ProcessLegacyExclude), catalog.LegacyProcessExcludeProgram)

	return baseFilter
}

// RegisterFactory registers a factory function for a given resource type and program ID
func (f *BaseFilterStore) RegisterFactory(id workloadfilter.FilterIdentifier, factory func(filterConfig *catalog.FilterConfig, logger logcomp.Component) program.FilterProgram) {
	resourceType := id.TargetResource()
	programID := id.GetFilterName()
	if f.ProgramFactoryStore[resourceType] == nil {
		f.ProgramFactoryStore[resourceType] = make(map[string]*FilterProgramFactory)
	}
	f.ProgramFactoryStore[resourceType][programID] = &FilterProgramFactory{
		factory: factory,
	}
}

// GetProgram returns the program for the given resource type and program ID
func (f *BaseFilterStore) GetProgram(resourceType workloadfilter.ResourceType, programID string) program.FilterProgram {
	if f.ProgramFactoryStore == nil {
		return nil
	}

	programFactories, ok := f.ProgramFactoryStore[resourceType]
	if !ok {
		return nil
	}

	factory, ok := programFactories[programID]
	if !ok {
		return nil
	}

	factory.once.Do(func() {
		factory.program = factory.factory(f.FilterConfig, f.Log)
	})

	return factory.program
}

// GetContainerAutodiscoveryFilters returns the pre-computed container autodiscovery filters
func (f *BaseFilterStore) GetContainerAutodiscoveryFilters(filterScope workloadfilter.Scope) workloadfilter.FilterBundle {
	return f.GetContainerFilters(f.selection.GetContainerAutodiscoveryFilters(filterScope))
}

// GetServiceAutodiscoveryFilters returns the pre-computed service autodiscovery filters
func (f *BaseFilterStore) GetServiceAutodiscoveryFilters(filterScope workloadfilter.Scope) workloadfilter.FilterBundle {
	return f.GetServiceFilters(f.selection.GetServiceAutodiscoveryFilters(filterScope))
}

// GetEndpointAutodiscoveryFilters returns the pre-computed endpoint autodiscovery filters
func (f *BaseFilterStore) GetEndpointAutodiscoveryFilters(filterScope workloadfilter.Scope) workloadfilter.FilterBundle {
	return f.GetEndpointFilters(f.selection.GetEndpointAutodiscoveryFilters(filterScope))
}

// GetContainerSharedMetricFilters returns the pre-computed container shared metric filters
func (f *BaseFilterStore) GetContainerSharedMetricFilters() workloadfilter.FilterBundle {
	return f.GetContainerFilters(f.selection.GetContainerSharedMetricFilters())
}

// GetContainerPausedFilters returns the pre-computed container paused filters
func (f *BaseFilterStore) GetContainerPausedFilters() workloadfilter.FilterBundle {
	return f.GetContainerFilters(f.selection.GetContainerPausedFilters())
}

// GetPodSharedMetricFilters returns the pre-computed pod shared metric filters
func (f *BaseFilterStore) GetPodSharedMetricFilters() workloadfilter.FilterBundle {
	return f.GetPodFilters(f.selection.GetPodSharedMetricFilters())
}

// GetContainerSBOMFilters returns the pre-computed container SBOM filters
func (f *BaseFilterStore) GetContainerSBOMFilters() workloadfilter.FilterBundle {
	return f.GetContainerFilters(f.selection.GetContainerSBOMFilters())
}

// GetContainerFilters returns the filter bundle for the given container filters
func (f *BaseFilterStore) GetContainerFilters(containerFilters [][]workloadfilter.ContainerFilter) workloadfilter.FilterBundle {
	return getFilterBundle(f, workloadfilter.ContainerType, containerFilters)
}

// GetPodFilters returns the filter bundle for the given pod filters
func (f *BaseFilterStore) GetPodFilters(podFilters [][]workloadfilter.PodFilter) workloadfilter.FilterBundle {
	return getFilterBundle(f, workloadfilter.PodType, podFilters)
}

// GetServiceFilters returns the filter bundle for the given service filters
func (f *BaseFilterStore) GetServiceFilters(serviceFilters [][]workloadfilter.ServiceFilter) workloadfilter.FilterBundle {
	return getFilterBundle(f, workloadfilter.ServiceType, serviceFilters)
}

// GetEndpointFilters returns the filter bundle for the given endpoint filters
func (f *BaseFilterStore) GetEndpointFilters(endpointFilters [][]workloadfilter.EndpointFilter) workloadfilter.FilterBundle {
	return getFilterBundle(f, workloadfilter.EndpointType, endpointFilters)
}

// GetProcessFilters returns the filter bundle for the given process filters
func (f *BaseFilterStore) GetProcessFilters(processFilters [][]workloadfilter.ProcessFilter) workloadfilter.FilterBundle {
	return getFilterBundle(f, workloadfilter.ProcessType, processFilters)
}

// GetFilterConfigString returns a string representation of the raw filter configuration
func (f *BaseFilterStore) GetFilterConfigString() (string, error) {
	return f.FilterConfig.String()
}

// getFilterBundle constructs a filter bundle for a given resource type and filters.
func getFilterBundle[T ~string](f *BaseFilterStore, objType workloadfilter.ResourceType, filters [][]T) workloadfilter.FilterBundle {
	var filterSets [][]program.FilterProgram
	for _, filterSet := range filters {
		var set []program.FilterProgram
		for _, filter := range filterSet {
			prg := f.GetProgram(objType, string(filter))
			if prg != nil {
				set = append(set, prg)
			}
		}
		filterSets = append(filterSets, set)
	}
	return &filterBundle{
		log:        f.Log,
		filterSets: filterSets,
	}
}
