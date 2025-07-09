// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadfilter

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const (
	highPrecedence = 0
	lowPrecedence  = 1
)

// GetSharedMetricsFilters identifies the filtering component's individual Container Filters for container metrics.
func GetSharedMetricsFilters() [][]ContainerFilter {

	flist := make([][]ContainerFilter, 2)

	// TODO: Add config option for users to configure AD annotations to take lower priority
	flist[highPrecedence] = []ContainerFilter{ContainerADAnnotations}

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

	flist[lowPrecedence] = low
	return flist
}

// Scope defines the scope of the filters.
type Scope string

// Predefined scopes for the filters.
const (
	GlobalFilter  Scope = "GlobalFilter"
	MetricsFilter Scope = "MetricsFilter"
	LogsFilter    Scope = "LogsFilter"
)

// GetAutodiscoveryFilters identifies the filtering component's individual Container Filters for autodiscovery.
func GetAutodiscoveryFilters(filterScope Scope) [][]ContainerFilter {

	flist := make([][]ContainerFilter, 2)

	// TODO: Add config option for users to configure AD annotations to take lower priority
	flist[highPrecedence] = []ContainerFilter{ContainerADAnnotations}

	low := []ContainerFilter{LegacyContainerGlobal}

	switch filterScope {
	case GlobalFilter:
		if len(pkgconfigsetup.Datadog().GetStringSlice("container_include")) == 0 {
			low = append(low, LegacyContainerACInclude)
		}
		if len(pkgconfigsetup.Datadog().GetStringSlice("container_exclude")) == 0 {
			low = append(low, LegacyContainerACExclude)
		}
	case MetricsFilter:
		low = append(low, LegacyContainerMetrics, ContainerADAnnotationsMetrics)
	case LogsFilter:
		low = append(low, LegacyContainerLogs, ContainerADAnnotationsLogs)
	}

	flist[lowPrecedence] = low

	return flist
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
