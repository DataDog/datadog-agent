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
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// LegacyContainerMetricsProgram creates a program for filtering container metrics
func LegacyContainerMetricsProgram(config config.Component, logger log.Component) program.CELProgram {
	programName := "LegacyContainerMetricsProgram"
	var initErrors []error

	includeProgram, includeErr := createProgramFromOldFilters(config.GetStringSlice("container_include_metrics"), workloadfilter.ContainerType)
	if includeErr != nil {
		initErrors = append(initErrors, includeErr)
		logger.Warnf("Error creating include program for %s: %v", programName, includeErr)
	}

	excludeProgram, excludeErr := createProgramFromOldFilters(config.GetStringSlice("container_exclude_metrics"), workloadfilter.ContainerType)
	if excludeErr != nil {
		initErrors = append(initErrors, excludeErr)
		logger.Warnf("Error creating exclude program for %s: %v", programName, excludeErr)
	}

	return program.CELProgram{
		Name:                 programName,
		Include:              includeProgram,
		Exclude:              excludeProgram,
		InitializationErrors: initErrors,
	}
}

// LegacyContainerLogsProgram creates a program for filtering container logs
func LegacyContainerLogsProgram(config config.Component, logger log.Component) program.CELProgram {
	programName := "LegacyContainerLogsProgram"
	var initErrors []error

	includeProgram, includeErr := createProgramFromOldFilters(config.GetStringSlice("container_include_logs"), workloadfilter.ContainerType)
	if includeErr != nil {
		initErrors = append(initErrors, includeErr)
		logger.Warnf("Error creating include program for %s: %v", programName, includeErr)
	}

	excludeProgram, excludeErr := createProgramFromOldFilters(config.GetStringSlice("container_exclude_logs"), workloadfilter.ContainerType)
	if excludeErr != nil {
		initErrors = append(initErrors, excludeErr)
		logger.Warnf("Error creating exclude program for %s: %v", programName, excludeErr)
	}

	return program.CELProgram{
		Name:                 programName,
		Include:              includeProgram,
		Exclude:              excludeProgram,
		InitializationErrors: initErrors,
	}
}

// LegacyContainerACExcludeProgram creates a program for excluding containers via legacy `AC` filters
func LegacyContainerACExcludeProgram(config config.Component, logger log.Component) program.CELProgram {
	programName := "LegacyContainerACExcludeProgram"
	var initErrors []error

	excludeProgram, excludeErr := createProgramFromOldFilters(config.GetStringSlice("ac_exclude"), workloadfilter.ContainerType)
	if excludeErr != nil {
		initErrors = append(initErrors, excludeErr)
		logger.Warnf("Error creating exclude program for %s: %v", programName, excludeErr)
	}

	return program.CELProgram{
		Name:                 programName,
		Exclude:              excludeProgram,
		InitializationErrors: initErrors,
	}
}

// LegacyContainerACIncludeProgram creates a program for including containers via legacy `AC` filters
func LegacyContainerACIncludeProgram(config config.Component, logger log.Component) program.CELProgram {
	programName := "LegacyContainerACIncludeProgram"
	var initErrors []error

	includeProgram, includeErr := createProgramFromOldFilters(config.GetStringSlice("ac_include"), workloadfilter.ContainerType)
	if includeErr != nil {
		initErrors = append(initErrors, includeErr)
		logger.Warnf("Error creating include program for %s: %v", programName, includeErr)
	}

	return program.CELProgram{
		Name:                 programName,
		Include:              includeProgram,
		InitializationErrors: initErrors,
	}
}

// LegacyContainerGlobalProgram creates a program for filtering container globally
func LegacyContainerGlobalProgram(config config.Component, logger log.Component) program.CELProgram {
	programName := "LegacyContainerGlobalProgram"
	var initErrors []error

	includeProgram, includeErr := createProgramFromOldFilters(config.GetStringSlice("container_include"), workloadfilter.ContainerType)
	if includeErr != nil {
		initErrors = append(initErrors, includeErr)
		logger.Warnf("Error creating include program for %s: %v", programName, includeErr)
	}

	excludeProgram, excludeErr := createProgramFromOldFilters(config.GetStringSlice("container_exclude"), workloadfilter.ContainerType)
	if excludeErr != nil {
		initErrors = append(initErrors, excludeErr)
		logger.Warnf("Error creating exclude program for %s: %v", programName, excludeErr)
	}

	return program.CELProgram{
		Name:                 programName,
		Include:              includeProgram,
		Exclude:              excludeProgram,
		InitializationErrors: initErrors,
	}
}

// LegacyContainerSBOMProgram creates a program for filtering container SBOMs
func LegacyContainerSBOMProgram(config config.Component, logger log.Component) program.CELProgram {
	programName := "LegacyContainerSBOMProgram"
	var initErrors []error

	if !config.GetBool("sbom.enabled") && !config.GetBool("sbom.container_image.enabled") && !config.GetBool("sbom.container.enabled") {
		return program.CELProgram{
			Name: programName,
		}
	}

	excludeList := config.GetStringSlice("sbom.container_image.container_exclude")
	if config.GetBool("sbom.container_image.exclude_pause_container") {
		excludeList = append(excludeList, containers.GetPauseContainerExcludeList()...)
	}

	includeProgram, includeErr := createProgramFromOldFilters(config.GetStringSlice("sbom.container_image.container_include"), workloadfilter.ContainerType)
	if includeErr != nil {
		initErrors = append(initErrors, includeErr)
		logger.Warnf("Error creating include program for %s: %v", programName, includeErr)
	}

	excludeProgram, excludeErr := createProgramFromOldFilters(excludeList, workloadfilter.ContainerType)
	if excludeErr != nil {
		initErrors = append(initErrors, excludeErr)
		logger.Warnf("Error creating exclude program for %s: %v", programName, excludeErr)
	}

	return program.CELProgram{
		Name:                 programName,
		Include:              includeProgram,
		Exclude:              excludeProgram,
		InitializationErrors: initErrors,
	}
}

// ContainerPausedProgram creates a program for filtering paused containers
func ContainerPausedProgram(config config.Component, logger log.Component) program.CELProgram {
	programName := "ContainerPausedProgram"
	var initErrors []error
	var excludeList []string
	if config.GetBool("exclude_pause_container") {
		excludeList = containers.GetPauseContainerExcludeList()
	}

	excludeProgram, excludeErr := createProgramFromOldFilters(excludeList, workloadfilter.ContainerType)
	if excludeErr != nil {
		initErrors = append(initErrors, excludeErr)
		logger.Warnf("Error creating exclude program for %s: %v", programName, excludeErr)
	}

	return program.CELProgram{
		Name:                 programName,
		Exclude:              excludeProgram,
		InitializationErrors: initErrors,
	}
}
