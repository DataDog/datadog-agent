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

// LegacyImageProgram creates a program for filtering legacy images.
func LegacyImageProgram(config config.Component, loggger log.Component) program.FilterProgram {
	programName := "LegacyImageProgram"
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

	includeProgram, includeErr := createProgramFromOldFilters(includeList, workloadfilter.ImageType)
	if includeErr != nil {
		initErrors = append(initErrors, includeErr)
		loggger.Warnf("Error creating include program for %s: %v", programName, includeErr)
	}
	excludeProgram, excludeErr := createProgramFromOldFilters(excludeList, workloadfilter.ImageType)
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

// ImageSBOMProgram creates a program for filtering legacy image SBOMs.
func ImageSBOMProgram(config config.Component, logger log.Component) program.CELProgram {
	programName := "LegacyImageSBOMProgram"
	var initErrors []error

	includeProgram, includeErr := createProgramFromOldFilters(config.GetStringSlice("sbom.container_image.container_include"), workloadfilter.ImageType)
	if includeErr != nil {
		initErrors = append(initErrors, includeErr)
		logger.Warnf("Error creating include program for %s: %v", programName, includeErr)
	}

	excludeProgram, excludeErr := createProgramFromOldFilters(config.GetStringSlice("sbom.container_image.container_exclude"), workloadfilter.ImageType)
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

// ImagePausedProgram creates a program for filtering paused images.
func ImagePausedProgram(config config.Component, logger log.Component) program.FilterProgram {
	programName := "ImagePausedProgram"
	var initErrors []error
	var excludeList []string
	if config.GetBool("exclude_pause_container") {
		excludeList = containers.GetPauseContainerExcludeList()
	}

	excludeProgram, excludeErr := createProgramFromOldFilters(excludeList, workloadfilter.ImageType)
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
