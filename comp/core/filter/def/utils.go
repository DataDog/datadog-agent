// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filter

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// GetSharedMetricsFilters identifies the filtering component's individual Container Filters for container metrics.
func GetSharedMetricsFilters() [][]ContainerFilter {
	const (
		highPrecedence = 0
		lowPrecedence  = 1
	)
	flist := make([][]ContainerFilter, 2)

	flist[highPrecedence] = []ContainerFilter{ContainerADAnnotations}

	low := []ContainerFilter{ContainerGlobal, ContainerMetrics}

	includeList := pkgconfigsetup.Datadog().GetStringSlice("container_include")
	excludeList := pkgconfigsetup.Datadog().GetStringSlice("container_exclude")
	includeList = append(includeList, pkgconfigsetup.Datadog().GetStringSlice("container_include_metrics")...)
	excludeList = append(excludeList, pkgconfigsetup.Datadog().GetStringSlice("container_exclude_metrics")...)

	if len(includeList) == 0 {
		low = append(low, ContainerACLegacyInclude)
	}
	if len(excludeList) == 0 {
		low = append(low, ContainerACLegacyExclude)

	}

	if pkgconfigsetup.Datadog().GetBool("exclude_pause_container") {
		low = append(low, ContainerPaused)
	}

	flist[lowPrecedence] = low
	return flist
}
