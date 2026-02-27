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
	factory func(builder *catalog.ProgramBuilder) program.FilterProgram
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
	Builder             *catalog.ProgramBuilder
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

	telemetryStore := telemetry.NewStore(telemetryComp)
	builder := catalog.NewProgramBuilder(filterConfig, logger, telemetryStore)

	baseFilter := &BaseFilterStore{
		Config:              cfg,
		Log:                 logger,
		ProgramFactoryStore: make(map[workloadfilter.ResourceType]map[string]*FilterProgramFactory),
		selection:           newFilterSelection(cfg),
		FilterConfig:        filterConfig,
		TelemetryStore:      telemetryStore,
		Builder:             builder,
	}

	// Pre-compute shared annotation programs
	genericADProgram := catalog.AutodiscoveryAnnotations(builder)
	genericADMetricsProgram := catalog.AutodiscoveryMetricsAnnotations(builder)
	genericADLogsProgram := catalog.AutodiscoveryLogsAnnotations(builder)
	genericADProgramFactory := func(_ *catalog.ProgramBuilder) program.FilterProgram { return genericADProgram }
	genericADMetricsProgramFactory := func(_ *catalog.ProgramBuilder) program.FilterProgram {
		return genericADMetricsProgram
	}
	genericADLogsProgramFactory := func(_ *catalog.ProgramBuilder) program.FilterProgram { return genericADLogsProgram }

	// Pre-compute legacy programs via `DD_CONTAINER_EXCLUDE*` that can be shared across entity types
	legacyGlobalPrg := catalog.LegacyContainerGlobalProgram(builder)
	legacyMetricsPrg := catalog.LegacyContainerMetricsProgram(builder)
	legacyLogsPrg := catalog.LegacyContainerLogsProgram(builder)
	legacyACIncludePrg := catalog.LegacyContainerACIncludeProgram(builder)
	legacyACExcludePrg := catalog.LegacyContainerACExcludeProgram(builder)
	legacyGlobalPrgFactory := func(_ *catalog.ProgramBuilder) program.FilterProgram { return legacyGlobalPrg }
	legacyMetricsPrgFactory := func(_ *catalog.ProgramBuilder) program.FilterProgram { return legacyMetricsPrg }
	legacyLogsPrgFactory := func(_ *catalog.ProgramBuilder) program.FilterProgram { return legacyLogsPrg }
	legacyACIncludePrgFactory := func(_ *catalog.ProgramBuilder) program.FilterProgram { return legacyACIncludePrg }
	legacyACExcludePrgFactory := func(_ *catalog.ProgramBuilder) program.FilterProgram { return legacyACExcludePrg }

	// Container Filters
	baseFilter.RegisterFactory(workloadfilter.ContainerLegacyMetrics, legacyMetricsPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerLegacyLogs, legacyLogsPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerLegacyACInclude, legacyACIncludePrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerLegacyACExclude, legacyACExcludePrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerLegacyGlobal, legacyGlobalPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerLegacySBOM, catalog.LegacyContainerSBOMProgram)
	baseFilter.RegisterFactory(workloadfilter.ContainerLegacyRuntimeSecurity, catalog.ContainerLegacyRuntimeSecurityProgram)
	baseFilter.RegisterFactory(workloadfilter.ContainerLegacyCompliance, catalog.ContainerLegacyComplianceProgram)

	baseFilter.RegisterFactory(workloadfilter.ContainerADAnnotations, genericADProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerADAnnotationsMetrics, genericADMetricsProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerADAnnotationsLogs, genericADLogsProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.ContainerPaused, catalog.ContainerPausedProgram)

	// Service Filters
	baseFilter.RegisterFactory(workloadfilter.KubeServiceLegacyGlobal, legacyGlobalPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.KubeServiceLegacyMetrics, legacyMetricsPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.KubeServiceADAnnotations, genericADProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.KubeServiceADAnnotationsMetrics, genericADMetricsProgramFactory)

	// Endpoints Filters
	baseFilter.RegisterFactory(workloadfilter.KubeEndpointLegacyGlobal, legacyGlobalPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.KubeEndpointLegacyMetrics, legacyMetricsPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.KubeEndpointADAnnotations, genericADProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.KubeEndpointADAnnotationsMetrics, genericADMetricsProgramFactory)

	// Pod Filters
	baseFilter.RegisterFactory(workloadfilter.PodLegacyMetrics, legacyMetricsPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.PodLegacyGlobal, legacyGlobalPrgFactory)
	baseFilter.RegisterFactory(workloadfilter.PodADAnnotations, genericADProgramFactory)
	baseFilter.RegisterFactory(workloadfilter.PodADAnnotationsMetrics, genericADMetricsProgramFactory)

	// Process Filters
	baseFilter.RegisterFactory(workloadfilter.ProcessLegacyExclude, catalog.LegacyProcessExcludeProgram)

	return baseFilter
}

// RegisterFactory registers a factory function for a given resource type and program ID
func (f *BaseFilterStore) RegisterFactory(id workloadfilter.FilterIdentifier, factory func(builder *catalog.ProgramBuilder) program.FilterProgram) {
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
		factory.program = factory.factory(f.Builder)
	})

	return factory.program
}

// GetContainerAutodiscoveryFilters returns the pre-computed container autodiscovery filters
func (f *BaseFilterStore) GetContainerAutodiscoveryFilters(filterScope workloadfilter.Scope) workloadfilter.FilterBundle {
	return f.GetContainerFilters(f.selection.GetContainerAutodiscoveryFilters(filterScope))
}

// GetKubeServiceAutodiscoveryFilters returns the pre-computed service autodiscovery filters
func (f *BaseFilterStore) GetKubeServiceAutodiscoveryFilters(filterScope workloadfilter.Scope) workloadfilter.FilterBundle {
	return f.GetKubeServiceFilters(f.selection.GetServiceAutodiscoveryFilters(filterScope))
}

// GetKubeEndpointAutodiscoveryFilters returns the pre-computed endpoint autodiscovery filters
func (f *BaseFilterStore) GetKubeEndpointAutodiscoveryFilters(filterScope workloadfilter.Scope) workloadfilter.FilterBundle {
	return f.GetKubeEndpointFilters(f.selection.GetEndpointAutodiscoveryFilters(filterScope))
}

// GetContainerSharedMetricFilters returns the pre-computed container shared metric filters
func (f *BaseFilterStore) GetContainerSharedMetricFilters() workloadfilter.FilterBundle {
	return f.GetContainerFilters(f.selection.containerSharedMetric)
}

// GetContainerPausedFilters returns the pre-computed container paused filters
func (f *BaseFilterStore) GetContainerPausedFilters() workloadfilter.FilterBundle {
	return f.GetContainerFilters(f.selection.containerPaused)
}

// GetPodSharedMetricFilters returns the pre-computed pod shared metric filters
func (f *BaseFilterStore) GetPodSharedMetricFilters() workloadfilter.FilterBundle {
	return f.GetPodFilters(f.selection.podSharedMetric)
}

// GetContainerSBOMFilters returns the pre-computed container SBOM filters
func (f *BaseFilterStore) GetContainerSBOMFilters() workloadfilter.FilterBundle {
	return f.GetContainerFilters(f.selection.containerSBOM)
}

// GetContainerRuntimeSecurityFilters returns the pre-computed container runtime security filters
func (f *BaseFilterStore) GetContainerRuntimeSecurityFilters() workloadfilter.FilterBundle {
	return f.GetContainerFilters(f.selection.containerRuntimeSecurity)
}

// GetContainerComplianceFilters returns the pre-computed container compliance filters
func (f *BaseFilterStore) GetContainerComplianceFilters() workloadfilter.FilterBundle {
	return f.GetContainerFilters(f.selection.containerCompliance)
}

// GetContainerFilters returns the filter bundle for the given container filters
func (f *BaseFilterStore) GetContainerFilters(containerFilters [][]workloadfilter.ContainerFilter) workloadfilter.FilterBundle {
	return getFilterBundle(f, workloadfilter.ContainerType, containerFilters)
}

// GetPodFilters returns the filter bundle for the given pod filters
func (f *BaseFilterStore) GetPodFilters(podFilters [][]workloadfilter.PodFilter) workloadfilter.FilterBundle {
	return getFilterBundle(f, workloadfilter.PodType, podFilters)
}

// GetKubeServiceFilters returns the filter bundle for the given service filters
func (f *BaseFilterStore) GetKubeServiceFilters(serviceFilters [][]workloadfilter.KubeServiceFilter) workloadfilter.FilterBundle {
	return getFilterBundle(f, workloadfilter.KubeServiceType, serviceFilters)
}

// GetKubeEndpointFilters returns the filter bundle for the given endpoint filters
func (f *BaseFilterStore) GetKubeEndpointFilters(endpointFilters [][]workloadfilter.KubeEndpointFilter) workloadfilter.FilterBundle {
	return getFilterBundle(f, workloadfilter.KubeEndpointType, endpointFilters)
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
	printFilter(&buffer, fmt.Sprintf("  %-16s", "Global:"), f.GetKubeServiceAutodiscoveryFilters(workloadfilter.GlobalFilter), useColor)
	printFilter(&buffer, fmt.Sprintf("  %-16s", "Metrics:"), f.GetKubeServiceAutodiscoveryFilters(workloadfilter.MetricsFilter), useColor)

	// Endpoint Autodiscovery Filters
	fmt.Fprintln(&buffer)
	printSectionHeader(&buffer, "-------- Kube Endpoint Autodiscovery Filters --------", useColor)
	printFilter(&buffer, fmt.Sprintf("  %-16s", "Global:"), f.GetKubeEndpointAutodiscoveryFilters(workloadfilter.GlobalFilter), useColor)
	printFilter(&buffer, fmt.Sprintf("  %-16s", "Metrics:"), f.GetKubeEndpointAutodiscoveryFilters(workloadfilter.MetricsFilter), useColor)

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
