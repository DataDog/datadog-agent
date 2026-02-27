// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package baseimpl contains the base implementation of the filter component.
package baseimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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
	containerCompliance           [][]workloadfilter.ContainerFilter
	containerRuntimeSecurity      [][]workloadfilter.ContainerFilter

	// Pod filters
	podSharedMetric [][]workloadfilter.PodFilter

	// Service filters
	serviceAutodiscoveryGlobal  [][]workloadfilter.KubeServiceFilter
	serviceAutodiscoveryMetrics [][]workloadfilter.KubeServiceFilter

	// Endpoint filters
	endpointAutodiscoveryGlobal  [][]workloadfilter.KubeEndpointFilter
	endpointAutodiscoveryMetrics [][]workloadfilter.KubeEndpointFilter
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

	pf.containerCompliance = pf.computeContainerComplianceFilters(cfg)
	pf.containerRuntimeSecurity = pf.computeContainerRuntimeSecurityFilters(pkgconfigsetup.SystemProbe())

	// Initialize container paused and SBOM filters
	pf.containerPaused = pf.computeContainerPausedFilters(cfg)
	pf.containerSBOM = pf.computeContainerSBOMFilters(cfg)

	// Initialize pod filters
	pf.podSharedMetric = pf.computePodSharedMetricFilters(cfg)

	// Initialize service filters
	pf.serviceAutodiscoveryGlobal = pf.computeKubeServiceAutodiscoveryFilters(cfg, workloadfilter.GlobalFilter)
	pf.serviceAutodiscoveryMetrics = pf.computeKubeServiceAutodiscoveryFilters(cfg, workloadfilter.MetricsFilter)

	// Initialize endpoint filters
	pf.endpointAutodiscoveryGlobal = pf.computeKubeEndpointAutodiscoveryFilters(cfg, workloadfilter.GlobalFilter)
	pf.endpointAutodiscoveryMetrics = pf.computeKubeEndpointAutodiscoveryFilters(cfg, workloadfilter.MetricsFilter)
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

// GetKubeServiceAutodiscoveryFilters returns pre-computed service autodiscovery filters
func (pf *filterSelection) GetServiceAutodiscoveryFilters(filterScope workloadfilter.Scope) [][]workloadfilter.KubeServiceFilter {
	switch filterScope {
	case workloadfilter.GlobalFilter:
		return pf.serviceAutodiscoveryGlobal
	case workloadfilter.MetricsFilter:
		return pf.serviceAutodiscoveryMetrics
	default:
		return nil
	}
}

// GetKubeEndpointAutodiscoveryFilters returns pre-computed endpoint autodiscovery filters
func (pf *filterSelection) GetEndpointAutodiscoveryFilters(filterScope workloadfilter.Scope) [][]workloadfilter.KubeEndpointFilter {
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
	low := []workloadfilter.ContainerFilter{workloadfilter.ContainerCELGlobal}

	switch filterScope {
	case workloadfilter.GlobalFilter:
		low = append(low, workloadfilter.ContainerLegacyGlobal)
		if len(cfg.GetStringSlice("container_include")) == 0 {
			low = append(low, workloadfilter.ContainerLegacyACInclude)
		}
		if len(cfg.GetStringSlice("container_exclude")) == 0 {
			low = append(low, workloadfilter.ContainerLegacyACExclude)
		}
	case workloadfilter.MetricsFilter:
		low = append(low, workloadfilter.ContainerLegacyMetrics, workloadfilter.ContainerCELMetrics)
		high = append(high, workloadfilter.ContainerADAnnotationsMetrics)
	case workloadfilter.LogsFilter:
		low = append(low, workloadfilter.ContainerLegacyLogs, workloadfilter.ContainerCELLogs)
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
	low := []workloadfilter.ContainerFilter{workloadfilter.ContainerLegacyGlobal, workloadfilter.ContainerLegacyMetrics, workloadfilter.ContainerCELGlobal, workloadfilter.ContainerCELMetrics}

	includeList := cfg.GetStringSlice("container_include")
	excludeList := cfg.GetStringSlice("container_exclude")
	includeList = append(includeList, cfg.GetStringSlice("container_include_metrics")...)
	excludeList = append(excludeList, cfg.GetStringSlice("container_exclude_metrics")...)

	if len(includeList) == 0 {
		low = append(low, workloadfilter.ContainerLegacyACInclude)
	}
	if len(excludeList) == 0 {
		low = append(low, workloadfilter.ContainerLegacyACExclude)
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
func (pf *filterSelection) computeContainerSBOMFilters(cfg config.Component) [][]workloadfilter.ContainerFilter {
	if cfg.GetBool("sbom.container_image.exclude_pause_container") {
		return [][]workloadfilter.ContainerFilter{{workloadfilter.ContainerLegacySBOM, workloadfilter.ContainerPaused}}
	}
	return [][]workloadfilter.ContainerFilter{{workloadfilter.ContainerLegacySBOM}}
}

// computePodSharedMetricFilters computes pod shared metric filters (migrated from def/utils.go)
func (pf *filterSelection) computePodSharedMetricFilters(_ config.Component) [][]workloadfilter.PodFilter {
	return [][]workloadfilter.PodFilter{{workloadfilter.PodADAnnotations, workloadfilter.PodADAnnotationsMetrics}, {workloadfilter.PodLegacyMetrics, workloadfilter.PodLegacyGlobal, workloadfilter.PodCELGlobal, workloadfilter.PodCELMetrics}}
}

// computeKubeServiceAutodiscoveryFilters computes service autodiscovery filters
func (pf *filterSelection) computeKubeServiceAutodiscoveryFilters(_ config.Component, filterScope workloadfilter.Scope) [][]workloadfilter.KubeServiceFilter {
	flist := make([][]workloadfilter.KubeServiceFilter, 2)

	high := []workloadfilter.KubeServiceFilter{workloadfilter.KubeServiceADAnnotations}
	low := []workloadfilter.KubeServiceFilter{workloadfilter.KubeServiceCELGlobal}

	switch filterScope {
	case workloadfilter.MetricsFilter:
		high = append(high, workloadfilter.KubeServiceADAnnotationsMetrics)
		low = append(low, workloadfilter.KubeServiceLegacyMetrics, workloadfilter.KubeServiceCELMetrics)
	case workloadfilter.GlobalFilter:
		low = append(low, workloadfilter.KubeServiceLegacyGlobal)
	}

	flist[0] = high // highPrecedence
	flist[1] = low  // lowPrecedence

	return flist
}

// computeKubeEndpointAutodiscoveryFilters computes endpoint autodiscovery filters
func (pf *filterSelection) computeKubeEndpointAutodiscoveryFilters(_ config.Component, filterScope workloadfilter.Scope) [][]workloadfilter.KubeEndpointFilter {
	flist := make([][]workloadfilter.KubeEndpointFilter, 2)

	high := []workloadfilter.KubeEndpointFilter{workloadfilter.KubeEndpointADAnnotations}
	low := []workloadfilter.KubeEndpointFilter{workloadfilter.KubeEndpointCELGlobal}

	switch filterScope {
	case workloadfilter.MetricsFilter:
		high = append(high, workloadfilter.KubeEndpointADAnnotationsMetrics)
		low = append(low, workloadfilter.KubeEndpointLegacyMetrics, workloadfilter.KubeEndpointCELMetrics)
	case workloadfilter.GlobalFilter:
		low = append(low, workloadfilter.KubeEndpointLegacyGlobal)
	default:
	}

	flist[0] = high // highPrecedence
	flist[1] = low  // lowPrecedence

	return flist
}

// computeContainerComplianceFilters computes container compliance filters
func (pf *filterSelection) computeContainerComplianceFilters(cfg config.Component) [][]workloadfilter.ContainerFilter {
	flist := []workloadfilter.ContainerFilter{workloadfilter.ContainerLegacyCompliance}
	if cfg.GetBool("compliance_config.exclude_pause_container") {
		flist = append(flist, workloadfilter.ContainerPaused)
	}
	return [][]workloadfilter.ContainerFilter{flist}
}

// computeContainerRuntimeSecurityFilters computes container runtime security filters
func (pf *filterSelection) computeContainerRuntimeSecurityFilters(cfg config.Component) [][]workloadfilter.ContainerFilter {
	flist := []workloadfilter.ContainerFilter{workloadfilter.ContainerLegacyRuntimeSecurity}
	if cfg.GetBool("runtime_security_config.exclude_pause_container") {
		flist = append(flist, workloadfilter.ContainerPaused)
	}
	return [][]workloadfilter.ContainerFilter{flist}
}
