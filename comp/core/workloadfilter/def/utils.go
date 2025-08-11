// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadfilter

import (
	"sync"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const (
	highPrecedence = 0
	lowPrecedence  = 1
)

// Scope defines the scope of the filters.
type Scope string

// Predefined scopes for the filters.
const (
	GlobalFilter  Scope = "GlobalFilter"
	MetricsFilter Scope = "MetricsFilter"
	LogsFilter    Scope = "LogsFilter"
)

var (
	// Cache for autodiscovery filters by scope
	autodiscoveryFiltersCache = make(map[Scope][][]ContainerFilter)
	autodiscoveryOnce         sync.Once

	// Cache for shared metric filters
	containerSharedMetricFiltersCache [][]ContainerFilter
	containerSharedMetricOnce         sync.Once

	// Cache for sbom filters
	containerSBOMFiltersCache [][]ContainerFilter
	containerSBOMOnce         sync.Once
)

// GetAutodiscoveryFilters identifies the filtering component's individual Container Filters for autodiscovery.
func GetAutodiscoveryFilters(filterScope Scope) [][]ContainerFilter {
	autodiscoveryOnce.Do(func() {
		// Generate all filter scopes at once
		for _, scope := range []Scope{GlobalFilter, MetricsFilter, LogsFilter} {
			flist := make([][]ContainerFilter, 2)

			high := []ContainerFilter{ContainerADAnnotations}
			low := []ContainerFilter{LegacyContainerGlobal}

			switch scope {
			case GlobalFilter:
				if len(pkgconfigsetup.Datadog().GetStringSlice("container_include")) == 0 {
					low = append(low, LegacyContainerACInclude)
				}
				if len(pkgconfigsetup.Datadog().GetStringSlice("container_exclude")) == 0 {
					low = append(low, LegacyContainerACExclude)
				}
			case MetricsFilter:
				low = append(low, LegacyContainerMetrics)
				high = append(high, ContainerADAnnotationsMetrics)
			case LogsFilter:
				low = append(low, LegacyContainerLogs)
				high = append(high, ContainerADAnnotationsLogs)
			default:
			}

			flist[highPrecedence] = high
			flist[lowPrecedence] = low

			autodiscoveryFiltersCache[scope] = flist
		}
	})

	return autodiscoveryFiltersCache[filterScope]
}

// GetContainerSharedMetricFilters identifies the filtering component's individual Container Filters for container metrics.
func GetContainerSharedMetricFilters() [][]ContainerFilter {
	containerSharedMetricOnce.Do(func() {
		flist := make([][]ContainerFilter, 2)

		high := []ContainerFilter{ContainerADAnnotations, ContainerADAnnotationsMetrics}
		low := []ContainerFilter{LegacyContainerGlobal, LegacyContainerMetrics}

		includeList := pkgconfigsetup.Datadog().GetStringSlice("container_include")
		excludeList := pkgconfigsetup.Datadog().GetStringSlice("container_exclude")
		includeList = append(includeList, pkgconfigsetup.Datadog().GetStringSlice("container_include_metrics")...)
		excludeList = append(excludeList, pkgconfigsetup.Datadog().GetStringSlice("container_exclude_metrics")...)

		if len(includeList) == 0 {
			low = append(low, LegacyContainerACInclude)
		}
		if len(excludeList) == 0 {
			low = append(low, LegacyContainerACExclude)
		}

		if pkgconfigsetup.Datadog().GetBool("exclude_pause_container") {
			low = append(low, ContainerPaused)
		}

		flist[highPrecedence] = high
		flist[lowPrecedence] = low
		containerSharedMetricFiltersCache = flist
	})

	return containerSharedMetricFiltersCache
}

// GetPodSharedMetricFilters identifies the filtering component's individual Pod Filters for pod metrics.
func GetPodSharedMetricFilters() [][]PodFilter {
	return [][]PodFilter{{PodADAnnotations, PodADAnnotationsMetrics}, {LegacyPod}}
}

// GetContainerSBOMFilters identifies the filter component's individual Container Filters for SBOM.
func GetContainerSBOMFilters() [][]ContainerFilter {
	containerSBOMOnce.Do(func() {
		selectedFilters := []ContainerFilter{LegacyContainerSBOM}
		if pkgconfigsetup.Datadog().GetBool("sbom.container_image.exclude_pause_container") {
			selectedFilters = append(selectedFilters, ContainerPaused)
		}
		containerSBOMFiltersCache = [][]ContainerFilter{selectedFilters}
	})
	return containerSBOMFiltersCache
}

// FlattenFilterSets flattens a slice of filter sets into a single slice.
func FlattenFilterSets[T ~int](
	filterSets [][]T, // Generic filter types
) []T {
	// Flatten the filter sets into a single slice
	flattened := make([]T, 0)
	for _, set := range filterSets {
		flattened = append(flattened, set...)
	}
	return flattened
}
