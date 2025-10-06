// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
)

// LegacyEndpointsMetricsProgram creates a program for filtering endpoints metrics
func LegacyEndpointsMetricsProgram(config config.Component, logger log.Component) program.FilterProgram {
	programName := "LegacyEndpointsMetricsProgram"
	include := config.GetStringSlice("container_include_metrics")
	exclude := config.GetStringSlice("container_exclude_metrics")
	return createFromOldFilters(programName, include, exclude, workloadfilter.EndpointType, logger)
}

// LegacyEndpointsGlobalProgram creates a program for filtering endpoints globally
func LegacyEndpointsGlobalProgram(config config.Component, logger log.Component) program.FilterProgram {
	programName := "LegacyEndpointsGlobalProgram"

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
	return createFromOldFilters(programName, includeList, excludeList, workloadfilter.EndpointType, logger)
}
