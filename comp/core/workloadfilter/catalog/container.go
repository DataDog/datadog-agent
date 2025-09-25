// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// LegacyContainerMetricsProgram creates a program for filtering container metrics
func LegacyContainerMetricsProgram(config config.Component, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerMetricsProgram"
	include := config.GetStringSlice("container_include_metrics")
	exclude := config.GetStringSlice("container_exclude_metrics")
	return createFromOldFilters(programName, include, exclude, workloadfilter.ContainerType, logger)
}

// LegacyContainerLogsProgram creates a program for filtering container logs
func LegacyContainerLogsProgram(config config.Component, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerLogsProgram"
	include := config.GetStringSlice("container_include_logs")
	exclude := config.GetStringSlice("container_exclude_logs")
	return createFromOldFilters(programName, include, exclude, workloadfilter.ContainerType, logger)
}

// LegacyContainerACExcludeProgram creates a program for excluding containers via legacy `AC` filters
func LegacyContainerACExcludeProgram(config config.Component, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerACExcludeProgram"
	exclude := config.GetStringSlice("ac_exclude")
	return createFromOldFilters(programName, nil, exclude, workloadfilter.ContainerType, logger)
}

// LegacyContainerACIncludeProgram creates a program for including containers via legacy `AC` filters
func LegacyContainerACIncludeProgram(config config.Component, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerACIncludeProgram"
	include := config.GetStringSlice("ac_include")
	return createFromOldFilters(programName, include, nil, workloadfilter.ContainerType, logger)
}

// LegacyContainerGlobalProgram creates a program for filtering container globally
func LegacyContainerGlobalProgram(config config.Component, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerGlobalProgram"
	include := config.GetStringSlice("container_include")
	exclude := config.GetStringSlice("container_exclude")
	return createFromOldFilters(programName, include, exclude, workloadfilter.ContainerType, logger)
}

// LegacyContainerSBOMProgram creates a program for filtering container SBOMs
func LegacyContainerSBOMProgram(config config.Component, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerSBOMProgram"
	include := config.GetStringSlice("sbom.container_image.container_include")
	exclude := config.GetStringSlice("sbom.container_image.container_exclude")
	return createFromOldFilters(programName, include, exclude, workloadfilter.ContainerType, logger)
}

// ContainerPausedProgram creates a program for filtering paused containers
func ContainerPausedProgram(_ config.Component, logger log.Component) program.FilterProgram {
	programName := "ContainerPausedProgram"
	var initErrors []error

	exclude := containers.GetPauseContainerExcludeList()

	excludeRegex := make([]*regexp.Regexp, 0, len(exclude))
	for _, pattern := range exclude {
		pattern = strings.TrimPrefix(pattern, "image:")
		regex, err := regexp.Compile(pattern)
		if err != nil {
			initErrors = append(initErrors, err)
			logger.Warnf("Error compiling regex pattern for %s: %v", programName, err)
			continue
		}
		excludeRegex = append(excludeRegex, regex)
	}

	return &program.RegexProgram{
		Name:                 programName,
		ExcludeRegex:         excludeRegex,
		InitializationErrors: initErrors,
	}
}
