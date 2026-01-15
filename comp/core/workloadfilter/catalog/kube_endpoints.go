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

// EndpointCELMetricsProgram creates a program for filtering endpoints metrics via CEL rules
func EndpointCELMetricsProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	rule := filterConfig.GetCELRulesForProduct(workloadfilter.ProductMetrics, workloadfilter.KubeEndpointType)
	return createCELExcludeProgram(string(workloadfilter.KubeEndpointCELMetrics), rule, workloadfilter.KubeEndpointType, logger)
}

// EndpointCELGlobalProgram creates a program for filtering endpoints globally via CEL rules
func EndpointCELGlobalProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	rule := filterConfig.GetCELRulesForProduct(workloadfilter.ProductGlobal, workloadfilter.KubeEndpointType)
	return createCELExcludeProgram(string(workloadfilter.KubeEndpointCELGlobal), rule, workloadfilter.KubeEndpointType, logger)
}
