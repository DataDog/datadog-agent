// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// ContainerPausedProgram creates a program for filtering paused containers
func ContainerPausedProgram(_ *FilterConfig, logger log.Component) program.FilterProgram {
	return createLegacyContainerProgram("ContainerPausedProgram", nil, containers.GetPauseContainerExcludeList(), logger)
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
