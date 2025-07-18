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

// LegacyPodProgram creates a program for filtering legacy pods.
func LegacyPodProgram(config config.Component, loggger log.Component) program.FilterProgram {
	programName := "LegacyPodProgram"
	var initErrors []error

	includeList := config.GetStringSlice("container_include")
	excludeList := config.GetStringSlice("container_exclude")
	includeList = append(includeList, config.GetStringSlice("container_include_metrics")...)
	excludeList = append(excludeList, config.GetStringSlice("container_exclude_metrics")...)

	if len(includeList) == 0 {
		// support legacy "ac_include" config
		includeList = config.GetStringSlice("ac_include")
	}
	if len(excludeList) == 0 {
		// support legacy "ac_exclude" config
		excludeList = config.GetStringSlice("ac_exclude")
	}

	includeProgram, includeErr := createProgramFromOldFilters(includeList, workloadfilter.PodType)
	if includeErr != nil {
		initErrors = append(initErrors, includeErr)
		loggger.Warnf("Error creating include program for %s: %v", programName, includeErr)
	}
	excludeProgram, excludeErr := createProgramFromOldFilters(excludeList, workloadfilter.PodType)
	if excludeErr != nil {
		initErrors = append(initErrors, excludeErr)
		loggger.Warnf("Error creating exclude program for %s: %v", programName, excludeErr)
	}

	return program.CELProgram{
		Name:                 programName,
		Include:              includeProgram,
		Exclude:              excludeProgram,
		InitializationErrors: initErrors,
	}
}

// PodADAnnotationsProgram creates a program for filtering pods based on AD annotations
func PodADAnnotationsProgram(_ config.Component, logger log.Component) program.CELProgram {
	programName := "PodADAnnotationsProgram"

	var initErrors []error
	// Use 'in' operator to safely check if annotation exists before accessing it
	excludeFilter := `(("ad.datadoghq.com/exclude") in pod.annotations && 
		 pod.annotations["ad.datadoghq.com/exclude"] in ["1", "t", "T", "true", "TRUE", "True"])`

	excludeProgram, err := createCELProgram(excludeFilter, workloadfilter.PodType)
	if err != nil {
		initErrors = append(initErrors, err)
		logger.Warnf("Error creating CEL filtering program for %s: %v", programName, err)
	}

	return program.CELProgram{
		Name:                 programName,
		Exclude:              excludeProgram,
		InitializationErrors: initErrors,
	}
}

// PodADAnnotationsMetricsProgram creates a program for filtering pods metrics based on AD annotations
func PodADAnnotationsMetricsProgram(_ config.Component, logger log.Component) program.CELProgram {
	programName := "PodADAnnotationsMetricsProgram"

	var initErrors []error
	// Use 'in' operator to safely check if annotation exists before accessing it
	excludeFilter := `(("ad.datadoghq.com/metrics_exclude") in pod.annotations && 
		 pod.annotations["ad.datadoghq.com/metrics_exclude"] in ["1", "t", "T", "true", "TRUE", "True"])`

	excludeProgram, err := createCELProgram(excludeFilter, workloadfilter.PodType)
	if err != nil {
		initErrors = append(initErrors, err)
		logger.Warnf("Error creating CEL filtering program for %s: %v", programName, err)
	}

	return program.CELProgram{
		Name:                 programName,
		Exclude:              excludeProgram,
		InitializationErrors: initErrors,
	}
}
