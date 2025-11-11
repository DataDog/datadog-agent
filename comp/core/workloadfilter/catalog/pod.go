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
)

// PodCELMetricsProgram creates a program for filtering pods metrics via CEL rules
func PodCELMetricsProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "PodCELMetricsProgram"
	rule := filterConfig.GetCELRulesForProduct(workloadfilter.ProductMetrics, workloadfilter.PodType)
	return createCELExcludeProgram(programName, rule, workloadfilter.PodType, logger)
}

// PodCELGlobalProgram creates a program for filtering pods globally via CEL rules
func PodCELGlobalProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "PodCELGlobalProgram"
	rule := filterConfig.GetCELRulesForProduct(workloadfilter.ProductGlobal, workloadfilter.PodType)
	return createCELExcludeProgram(programName, rule, workloadfilter.PodType, logger)
}
