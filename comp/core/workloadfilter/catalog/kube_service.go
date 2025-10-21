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

// ServiceCELMetricsProgram creates a program for filtering services metrics via CEL rules
func ServiceCELMetricsProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "ServiceCELMetricsProgram"
	rule := filterConfig.GetCELRulesForProduct(workloadfilter.ProductMetrics, workloadfilter.ServiceType)
	return createCELExcludeProgram(programName, rule, workloadfilter.ServiceType, logger)
}

// ServiceCELGlobalProgram creates a program for filtering services globally via CEL rules
func ServiceCELGlobalProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "ServiceCELGlobalProgram"
	rule := filterConfig.GetCELRulesForProduct(workloadfilter.ProductGlobal, workloadfilter.ServiceType)
	return createCELExcludeProgram(programName, rule, workloadfilter.ServiceType, logger)
}
