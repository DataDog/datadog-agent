// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package baseimpl contains the base implementation of the filter component.
package baseimpl

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/fatih/color"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
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
	baseFilter.RegisterFactory(workloadfilter.ContainerType, string(workloadfilter.ContainerLegacyMetrics), legacyMetricsPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerType, string(workloadfilter.ContainerLegacyLogs), legacyLogsPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerType, string(workloadfilter.ContainerLegacyACInclude), legacyACIncludePrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerType, string(workloadfilter.ContainerLegacyACExclude), legacyACExcludePrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerType, string(workloadfilter.ContainerLegacyGlobal), legacyGlobalPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerType, string(workloadfilter.ContainerLegacySBOM), catalog.LegacyContainerSBOMProgram)

	baseFilter.RegisterFactory(workloadfilter.ContainerType, string(workloadfilter.ContainerADAnnotations), genericADProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerType, string(workloadfilter.ContainerADAnnotationsMetrics), genericADMetricsProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerType, string(workloadfilter.ContainerADAnnotationsLogs), genericADLogsProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerType, string(workloadfilter.ContainerPaused), catalog.ContainerPausedProgram)

	// Service Filters
	baseFilter.RegisterFactory(workloadfilter.ServiceType, string(workloadfilter.ServiceLegacyGlobal), legacyGlobalPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ServiceType, string(workloadfilter.ServiceLegacyMetrics), legacyMetricsPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ServiceType, string(workloadfilter.ServiceADAnnotations), genericADProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.ServiceType, string(workloadfilter.ServiceADAnnotationsMetrics), genericADMetricsProgramFactory)

	// Endpoints Filters
	baseFilter.RegisterFactory(workloadfilter.EndpointType, string(workloadfilter.EndpointLegacyGlobal), legacyGlobalPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.EndpointType, string(workloadfilter.EndpointLegacyMetrics), legacyMetricsPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.EndpointType, string(workloadfilter.EndpointADAnnotations), genericADProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.EndpointType, string(workloadfilter.EndpointADAnnotationsMetrics), genericADMetricsProgramFactory)

	// Pod Filters
	baseFilter.RegisterFactory(workloadfilter.PodType, string(workloadfilter.PodLegacyMetrics), legacyMetricsPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.PodType, string(workloadfilter.PodLegacyGlobal), legacyGlobalPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.PodType, string(workloadfilter.PodADAnnotations), genericADProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.PodType, string(workloadfilter.PodADAnnotationsMetrics), genericADMetricsProgramFactory)

	// Process Filters
	baseFilter.RegisterFactory(workloadfilter.ProcessType, string(workloadfilter.ProcessLegacyExcludeList), catalog.LegacyProcessExcludeProgram)

	return baseFilter
}

// RegisterFactory registers a factory function for a given resource type and program ID
func (f *BaseFilterStore) RegisterFactory(resourceType workloadfilter.ResourceType, programID string, factory func(filterConfig *catalog.FilterConfig, logger logcomp.Component) program.FilterProgram) {
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

func (f *BaseFilterStore) FlareCallback(fb flaretypes.FlareBuilder) error {
	fb.AddFile("workload-filter.log", []byte(f.String(false)))
	return nil
}

// String returns a string representation of the workloadfilter configuration
func (f *BaseFilterStore) String(useColor bool) string {
	var buffer bytes.Buffer

	printMainHeader(&buffer, "=== Workload Filter Status ===", useColor)
	fmt.Fprintln(&buffer)

	// Container Autodiscovery Filters
	printSectionHeader(&buffer, "-------- Container Autodiscovery Filters --------", useColor)
	printFilter(&buffer, fmt.Sprintf("  %-16s", "Global:"), f.GetContainerAutodiscoveryFilters(workloadfilter.GlobalFilter), useColor)
	printFilter(&buffer, fmt.Sprintf("  %-16s", "Metrics:"), f.GetContainerAutodiscoveryFilters(workloadfilter.MetricsFilter), useColor)
	printFilter(&buffer, fmt.Sprintf("  %-16s", "Logs:"), f.GetContainerAutodiscoveryFilters(workloadfilter.LogsFilter), useColor)

	// Service Autodiscovery Filters
	fmt.Fprintln(&buffer)
	printSectionHeader(&buffer, "-------- Kube Service Autodiscovery Filters --------", useColor)
	printFilter(&buffer, fmt.Sprintf("  %-16s", "Global:"), f.GetServiceAutodiscoveryFilters(workloadfilter.GlobalFilter), useColor)
	printFilter(&buffer, fmt.Sprintf("  %-16s", "Metrics:"), f.GetServiceAutodiscoveryFilters(workloadfilter.MetricsFilter), useColor)

	// Endpoint Autodiscovery Filters
	fmt.Fprintln(&buffer)
	printSectionHeader(&buffer, "-------- Kube Endpoint Autodiscovery Filters --------", useColor)
	printFilter(&buffer, fmt.Sprintf("  %-16s", "Global:"), f.GetEndpointAutodiscoveryFilters(workloadfilter.GlobalFilter), useColor)
	printFilter(&buffer, fmt.Sprintf("  %-16s", "Metrics:"), f.GetEndpointAutodiscoveryFilters(workloadfilter.MetricsFilter), useColor)

	// Pod Shared Metric Filters
	fmt.Fprintln(&buffer)
	printSectionHeader(&buffer, "-------- Pod Shared Metrics Filters --------", useColor)
	printFilter(&buffer, fmt.Sprintf("  %-16s", "SharedMetrics:"), f.GetPodSharedMetricFilters(), useColor)

	// Container Shared Metric Filters
	fmt.Fprintln(&buffer)
	printSectionHeader(&buffer, "-------- Container Shared Metrics Filters --------", useColor)
	printFilter(&buffer, fmt.Sprintf("  %-16s", "SharedMetrics:"), f.GetContainerSharedMetricFilters(), useColor)

	// Container Paused Filters
	fmt.Fprintln(&buffer)
	printSectionHeader(&buffer, "-------- Container Paused Filters --------", useColor)
	printFilter(&buffer, fmt.Sprintf("  %-16s", "PausedContainers:"), f.GetContainerPausedFilters(), useColor)

	// Container SBOM Filters
	fmt.Fprintln(&buffer)
	printSectionHeader(&buffer, "-------- Container SBOM Filters --------", useColor)
	printFilter(&buffer, fmt.Sprintf("  %-16s", "SBOM:"), f.GetContainerSBOMFilters(), useColor)

	// Print raw filter configuration
	fmt.Fprintln(&buffer)
	printSectionHeader(&buffer, "-------- Raw Filter Configuration --------", useColor)

	if f.FilterConfig == nil {
		if useColor {
			fmt.Fprintf(&buffer, "      %s\n", color.HiRedString("-> Filter config not initialized"))
		} else {
			fmt.Fprintf(&buffer, "      -> Filter config not initialized\n")
		}
	} else {
		fmt.Fprint(&buffer, f.FilterConfig.String(useColor))
	}

	return buffer.String()
}

func printMainHeader(w io.Writer, text string, useColor bool) {
	if useColor {
		fmt.Fprintf(w, "    %s\n", color.HiCyanString(text))
	} else {
		fmt.Fprintf(w, "%s\n", text)
	}
}

func printSectionHeader(w io.Writer, text string, useColor bool) {
	if useColor {
		fmt.Fprintf(w, "    %s\n", color.HiCyanString(text))
	} else {
		fmt.Fprintf(w, "%s\n", text)
	}
}

func printFilter(w io.Writer, name string, bundle workloadfilter.FilterBundle, useColor bool) {
	if bundle == nil {
		fmt.Fprintf(w, "%s: No filters configured\n", name)
		return
	}

	errors := bundle.GetErrors()
	if len(errors) > 0 {
		if useColor {
			fmt.Fprintf(w, "%s %s %s\n", color.HiRedString("✗"), name, color.HiRedString("failed to load"))
		} else {
			fmt.Fprintf(w, "x %s failed to load\n", name)
		}
		for _, err := range errors {
			fmt.Fprintf(w, "        Error: %s\n", err)
		}
		return
	}

	if useColor {
		fmt.Fprintf(w, "%s %s Loaded successfully\n", color.HiGreenString("✓"), name)
	} else {
		fmt.Fprintf(w, "v %s Loaded successfully\n", name)
	}
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
