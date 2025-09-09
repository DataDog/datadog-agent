// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloadfilterimpl contains the implementation of the filter component.
package workloadfilterimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

// filterSelection stores pre-computed filter lists to avoid recalculating them on every call
type filterSelection struct {

	// Container filters
	containerAutodiscoveryGlobal  [][]workloadfilter.ContainerFilter
	containerAutodiscoveryMetrics [][]workloadfilter.ContainerFilter
	containerAutodiscoveryLogs    [][]workloadfilter.ContainerFilter
	containerSharedMetric         [][]workloadfilter.ContainerFilter
	containerPaused               [][]workloadfilter.ContainerFilter
	containerSBOM                 [][]workloadfilter.ContainerFilter

	// Pod filters
	podAutodiscoveryGlobal  [][]workloadfilter.PodFilter
	podAutodiscoveryMetrics [][]workloadfilter.PodFilter
	podAutodiscoveryLogs    [][]workloadfilter.PodFilter
	podSharedMetric         [][]workloadfilter.PodFilter

	// Service filters
	serviceAutodiscoveryGlobal  [][]workloadfilter.ServiceFilter
	serviceAutodiscoveryMetrics [][]workloadfilter.ServiceFilter

	// Endpoint filters
	endpointAutodiscoveryGlobal  [][]workloadfilter.EndpointFilter
	endpointAutodiscoveryMetrics [][]workloadfilter.EndpointFilter
}

// newFilterSelection creates a new filterSelection instance
func newFilterSelection(cfg config.Component) *filterSelection {
	selection := &filterSelection{}
	selection.initializeSelections(cfg)
	return selection
}

// initializeSelections computes all filter lists once based on current configuration
func (pf *filterSelection) initializeSelections(cfg config.Component) {

	// Initialize container filters
	pf.containerAutodiscoveryGlobal = pf.computeContainerAutodiscoveryFilters(cfg, workloadfilter.GlobalFilter)
	pf.containerAutodiscoveryMetrics = pf.computeContainerAutodiscoveryFilters(cfg, workloadfilter.MetricsFilter)
	pf.containerAutodiscoveryLogs = pf.computeContainerAutodiscoveryFilters(cfg, workloadfilter.LogsFilter)
	pf.containerSharedMetric = pf.computeContainerSharedMetricFilters(cfg)

	// Initialize container paused and SBOM filters
	pf.containerPaused = pf.computeContainerPausedFilters(cfg)
	pf.containerSBOM = pf.computeContainerSBOMFilters(cfg)

	// Initialize pod filters
	pf.podAutodiscoveryGlobal = pf.computePodAutodiscoveryFilters(cfg, workloadfilter.GlobalFilter)
	pf.podAutodiscoveryMetrics = pf.computePodAutodiscoveryFilters(cfg, workloadfilter.MetricsFilter)
	pf.podAutodiscoveryLogs = pf.computePodAutodiscoveryFilters(cfg, workloadfilter.LogsFilter)
	pf.podSharedMetric = pf.computePodSharedMetricFilters(cfg)

	// Initialize service filters
	pf.serviceAutodiscoveryGlobal = pf.computeServiceAutodiscoveryFilters(cfg, workloadfilter.GlobalFilter)
	pf.serviceAutodiscoveryMetrics = pf.computeServiceAutodiscoveryFilters(cfg, workloadfilter.MetricsFilter)

	// Initialize endpoint filters
	pf.endpointAutodiscoveryGlobal = pf.computeEndpointAutodiscoveryFilters(cfg, workloadfilter.GlobalFilter)
	pf.endpointAutodiscoveryMetrics = pf.computeEndpointAutodiscoveryFilters(cfg, workloadfilter.MetricsFilter)
}

// GetContainerAutodiscoveryFilters returns pre-computed container autodiscovery filters
func (pf *filterSelection) GetContainerAutodiscoveryFilters(filterScope workloadfilter.Scope) [][]workloadfilter.ContainerFilter {
	switch filterScope {
	case workloadfilter.GlobalFilter:
		return pf.containerAutodiscoveryGlobal
	case workloadfilter.MetricsFilter:
		return pf.containerAutodiscoveryMetrics
	case workloadfilter.LogsFilter:
		return pf.containerAutodiscoveryLogs
	default:
		return nil
	}
}

// GetContainerSharedMetricFilters returns pre-computed container shared metric filters
func (pf *filterSelection) GetContainerSharedMetricFilters() [][]workloadfilter.ContainerFilter {
	return pf.containerSharedMetric
}

// GetContainerPausedFilters returns pre-computed container paused filters
func (pf *filterSelection) GetContainerPausedFilters() [][]workloadfilter.ContainerFilter {
	return pf.containerPaused
}

// GetContainerSBOMFilters returns pre-computed container SBOM filters
func (pf *filterSelection) GetContainerSBOMFilters() [][]workloadfilter.ContainerFilter {
	return pf.containerSBOM
}

// GetPodAutodiscoveryFilters returns pre-computed pod autodiscovery filters
func (pf *filterSelection) GetPodAutodiscoveryFilters(filterScope workloadfilter.Scope) [][]workloadfilter.PodFilter {
	switch filterScope {
	case workloadfilter.GlobalFilter:
		return pf.podAutodiscoveryGlobal
	case workloadfilter.MetricsFilter:
		return pf.podAutodiscoveryMetrics
	case workloadfilter.LogsFilter:
		return pf.podAutodiscoveryLogs
	default:
		return nil
	}
}

// GetPodSharedMetricFilters returns pre-computed pod shared metric filters
func (pf *filterSelection) GetPodSharedMetricFilters() [][]workloadfilter.PodFilter {
	return pf.podSharedMetric
}

// GetServiceAutodiscoveryFilters returns pre-computed service autodiscovery filters
func (pf *filterSelection) GetServiceAutodiscoveryFilters(filterScope workloadfilter.Scope) [][]workloadfilter.ServiceFilter {
	switch filterScope {
	case workloadfilter.GlobalFilter:
		return pf.serviceAutodiscoveryGlobal
	case workloadfilter.MetricsFilter:
		return pf.serviceAutodiscoveryMetrics
	default:
		return nil
	}
}

// GetEndpointAutodiscoveryFilters returns pre-computed endpoint autodiscovery filters
func (pf *filterSelection) GetEndpointAutodiscoveryFilters(filterScope workloadfilter.Scope) [][]workloadfilter.EndpointFilter {
	switch filterScope {
	case workloadfilter.GlobalFilter:
		return pf.endpointAutodiscoveryGlobal
	case workloadfilter.MetricsFilter:
		return pf.endpointAutodiscoveryMetrics
	default:
		return nil
	}
}

// computeContainerAutodiscoveryFilters computes container autodiscovery filters (migrated from def/utils.go)
func (pf *filterSelection) computeContainerAutodiscoveryFilters(cfg config.Component, filterScope workloadfilter.Scope) [][]workloadfilter.ContainerFilter {
	flist := make([][]workloadfilter.ContainerFilter, 2)

	high := []workloadfilter.ContainerFilter{workloadfilter.ContainerADAnnotations}
	low := []workloadfilter.ContainerFilter{workloadfilter.LegacyContainerGlobal}

	switch filterScope {
	case workloadfilter.GlobalFilter:
		if len(cfg.GetStringSlice("container_include")) == 0 {
			low = append(low, workloadfilter.LegacyContainerACInclude)
		}
		if len(cfg.GetStringSlice("container_exclude")) == 0 {
			low = append(low, workloadfilter.LegacyContainerACExclude)
		}
	case workloadfilter.MetricsFilter:
		low = append(low, workloadfilter.LegacyContainerMetrics)
		high = append(high, workloadfilter.ContainerADAnnotationsMetrics)
	case workloadfilter.LogsFilter:
		low = append(low, workloadfilter.LegacyContainerLogs)
		high = append(high, workloadfilter.ContainerADAnnotationsLogs)
	default:
	}

	flist[0] = high // highPrecedence
	flist[1] = low  // lowPrecedence

	return flist
}

// computeContainerSharedMetricFilters computes container shared metric filters (migrated from def/utils.go)
func (pf *filterSelection) computeContainerSharedMetricFilters(cfg config.Component) [][]workloadfilter.ContainerFilter {
	flist := make([][]workloadfilter.ContainerFilter, 2)

	high := []workloadfilter.ContainerFilter{workloadfilter.ContainerADAnnotations, workloadfilter.ContainerADAnnotationsMetrics}
	low := []workloadfilter.ContainerFilter{workloadfilter.LegacyContainerGlobal, workloadfilter.LegacyContainerMetrics}

	includeList := cfg.GetStringSlice("container_include")
	excludeList := cfg.GetStringSlice("container_exclude")
	includeList = append(includeList, cfg.GetStringSlice("container_include_metrics")...)
	excludeList = append(excludeList, cfg.GetStringSlice("container_exclude_metrics")...)

	if len(includeList) == 0 {
		low = append(low, workloadfilter.LegacyContainerACInclude)
	}
	if len(excludeList) == 0 {
		low = append(low, workloadfilter.LegacyContainerACExclude)
	}

	if cfg.GetBool("exclude_pause_container") {
		low = append(low, workloadfilter.ContainerPaused)
	}

	flist[0] = high // highPrecedence
	flist[1] = low  // lowPrecedence
	return flist
}

// computeContainerPausedFilters computes container paused filters (migrated from def/utils.go)
func (pf *filterSelection) computeContainerPausedFilters(cfg config.Component) [][]workloadfilter.ContainerFilter {
	if cfg.GetBool("exclude_pause_container") {
		return [][]workloadfilter.ContainerFilter{{workloadfilter.ContainerPaused}}
	}
	return nil
}

// computeContainerSBOMFilters computes container SBOM filters (migrated from def/utils.go)
func (pf *filterSelection) computeContainerSBOMFilters(_ config.Component) [][]workloadfilter.ContainerFilter {
	return [][]workloadfilter.ContainerFilter{{workloadfilter.LegacyContainerSBOM}}
}

// computePodAutodiscoveryFilters computes pod autodiscovery filters
func (pf *filterSelection) computePodAutodiscoveryFilters(_ config.Component, filterScope workloadfilter.Scope) [][]workloadfilter.PodFilter {
	flist := make([][]workloadfilter.PodFilter, 2)

	high := []workloadfilter.PodFilter{workloadfilter.PodADAnnotations}
	low := []workloadfilter.PodFilter{workloadfilter.LegacyPod}

	switch filterScope {
	case workloadfilter.MetricsFilter:
		high = append(high, workloadfilter.PodADAnnotationsMetrics)
	}

	flist[0] = high // highPrecedence
	flist[1] = low  // lowPrecedence

	return flist
}

// computePodSharedMetricFilters computes pod shared metric filters (migrated from def/utils.go)
func (pf *filterSelection) computePodSharedMetricFilters(_ config.Component) [][]workloadfilter.PodFilter {
	return [][]workloadfilter.PodFilter{{workloadfilter.PodADAnnotations, workloadfilter.PodADAnnotationsMetrics}, {workloadfilter.LegacyPod}}
}

// computeServiceAutodiscoveryFilters computes service autodiscovery filters
func (pf *filterSelection) computeServiceAutodiscoveryFilters(_ config.Component, filterScope workloadfilter.Scope) [][]workloadfilter.ServiceFilter {
	flist := make([][]workloadfilter.ServiceFilter, 2)

	high := []workloadfilter.ServiceFilter{workloadfilter.ServiceADAnnotations}
	low := []workloadfilter.ServiceFilter{}

	switch filterScope {
	case workloadfilter.MetricsFilter:
		high = append(high, workloadfilter.ServiceADAnnotationsMetrics)
		low = append(low, workloadfilter.LegacyServiceMetrics)
	case workloadfilter.GlobalFilter:
		low = append(low, workloadfilter.LegacyServiceGlobal)
	}

	flist[0] = high // highPrecedence
	flist[1] = low  // lowPrecedence

	return flist
}

// computeEndpointAutodiscoveryFilters computes endpoint autodiscovery filters
func (pf *filterSelection) computeEndpointAutodiscoveryFilters(_ config.Component, filterScope workloadfilter.Scope) [][]workloadfilter.EndpointFilter {
	flist := make([][]workloadfilter.EndpointFilter, 2)

	high := []workloadfilter.EndpointFilter{workloadfilter.EndpointADAnnotations}
	low := []workloadfilter.EndpointFilter{}

	switch filterScope {
	case workloadfilter.MetricsFilter:
		high = append(high, workloadfilter.EndpointADAnnotationsMetrics)
		low = append(low, workloadfilter.LegacyEndpointMetrics)
	case workloadfilter.GlobalFilter:
		low = append(low, workloadfilter.LegacyEndpointGlobal)
	default:
	}

	flist[0] = high // highPrecedence
	flist[1] = low  // lowPrecedence

	return flist
}
