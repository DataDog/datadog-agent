// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	"regexp"
	"strings"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// LegacyContainerMetricsProgram creates a program for filtering container metrics
func LegacyContainerMetricsProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerMetricsProgram"
	include := filterConfig.ContainerIncludeMetrics
	exclude := filterConfig.ContainerExcludeMetrics
	return createFromOldFilters(programName, include, exclude, workloadfilter.ContainerType, logger)
}

// LegacyContainerLogsProgram creates a program for filtering container logs
func LegacyContainerLogsProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerLogsProgram"
	include := filterConfig.ContainerIncludeLogs
	exclude := filterConfig.ContainerExcludeLogs
	return createFromOldFilters(programName, include, exclude, workloadfilter.ContainerType, logger)
}

// LegacyContainerACExcludeProgram creates a program for excluding containers via legacy `AC` filters
func LegacyContainerACExcludeProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerACExcludeProgram"
	exclude := filterConfig.ACExclude
	return createFromOldFilters(programName, nil, exclude, workloadfilter.ContainerType, logger)
}

// LegacyContainerACIncludeProgram creates a program for including containers via legacy `AC` filters
func LegacyContainerACIncludeProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerACIncludeProgram"
	include := filterConfig.ACInclude
	return createFromOldFilters(programName, include, nil, workloadfilter.ContainerType, logger)
}

// LegacyContainerGlobalProgram creates a program for filtering container globally
func LegacyContainerGlobalProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerGlobalProgram"
	include := filterConfig.ContainerInclude
	exclude := filterConfig.ContainerExclude
	return createFromOldFilters(programName, include, exclude, workloadfilter.ContainerType, logger)
}

// LegacyContainerSBOMProgram creates a program for filtering container SBOMs
func LegacyContainerSBOMProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "LegacyContainerSBOMProgram"
	include := filterConfig.SBOMContainerInclude
	exclude := filterConfig.SBOMContainerExclude
	return createFromOldFilters(programName, include, exclude, workloadfilter.ContainerType, logger)
}

// ContainerPausedProgram creates a program for filtering paused containers
func ContainerPausedProgram(_ *FilterConfig, logger log.Component) program.FilterProgram {
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
		Name:         programName,
		ExcludeRegex: excludeRegex,
		ExtractField: func(entity workloadfilter.Filterable) string {
			container, ok := entity.(*workloadfilter.Container)
			if !ok {
				return ""
			}
			return container.GetImage()
		},
		InitializationErrors: initErrors,
	}
}

// ContainerCELMetricsProgram creates a program for filtering container metrics via CEL rules
func ContainerCELMetricsProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "ContainerCELMetricsProgram"
	rule := filterConfig.GetCELRulesForProduct(workloadfilter.ProductMetrics, workloadfilter.ContainerType)
	return createCELExcludeProgram(programName, rule, workloadfilter.ContainerType, logger)
}

// ContainerCELLogsProgram creates a program for filtering container logs via CEL rules
func ContainerCELLogsProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "ContainerCELLogsProgram"
	rule := filterConfig.GetCELRulesForProduct(workloadfilter.ProductLogs, workloadfilter.ContainerType)
	return createCELExcludeProgram(programName, rule, workloadfilter.ContainerType, logger)
}

// ContainerCELSBOMProgram creates a program for filtering container SBOMs via CEL rules
func ContainerCELSBOMProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "ContainerCELSBOMProgram"
	rule := filterConfig.GetCELRulesForProduct(workloadfilter.ProductSBOM, workloadfilter.ContainerType)
	return createCELExcludeProgram(programName, rule, workloadfilter.ContainerType, logger)
}

// ContainerCELGlobalProgram creates a program for filtering containers globally via CEL rules
func ContainerCELGlobalProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "ContainerCELGlobalProgram"
	rule := filterConfig.GetCELRulesForProduct(workloadfilter.ProductGlobal, workloadfilter.ContainerType)
	return createCELExcludeProgram(programName, rule, workloadfilter.ContainerType, logger)
}
