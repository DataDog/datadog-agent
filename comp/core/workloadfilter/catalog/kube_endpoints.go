// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	filter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// LegacyEndpointsMetricsProgram creates a program for filtering endpoints metrics
func LegacyEndpointsMetricsProgram(config config.Component, logger log.Component) program.CELProgram {
	return program.CELProgram{
		Name:    "LegacyEndpointsMetricsProgram",
		Include: createProgramFromOldFilters(config.GetStringSlice("container_include_metrics"), filter.EndpointType, logger),
		Exclude: createProgramFromOldFilters(config.GetStringSlice("container_exclude_metrics"), filter.EndpointType, logger),
	}
}

// LegacyEndpointsGlobalProgram creates a program for filtering endpoints globally
func LegacyEndpointsGlobalProgram(config config.Component, logger log.Component) program.CELProgram {
	includeList := config.GetStringSlice("container_include")
	excludeList := config.GetStringSlice("container_exclude")
	if len(includeList) == 0 {
		// fallback and support legacy "ac_include" config
		includeList = config.GetStringSlice("ac_include")
	}
	if len(excludeList) == 0 {
		// fallback and support legacy "ac_exclude" config
		excludeList = config.GetStringSlice("ac_exclude")
	}

	return program.CELProgram{
		Name:    "LegacyEndpointsGlobalProgram",
		Include: createProgramFromOldFilters(includeList, filter.EndpointType, logger),
		Exclude: createProgramFromOldFilters(excludeList, filter.EndpointType, logger),
	}
}
