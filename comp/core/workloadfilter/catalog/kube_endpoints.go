// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
)

// EndpointCELMetricsProgram creates a program for filtering endpoints metrics via CEL rules
func EndpointCELMetricsProgram(b *ProgramBuilder) program.FilterProgram {
	return b.CreateCELProgram(workloadfilter.KubeEndpointCELMetrics, workloadfilter.ProductMetrics)
}

// EndpointCELGlobalProgram creates a program for filtering endpoints globally via CEL rules
func EndpointCELGlobalProgram(b *ProgramBuilder) program.FilterProgram {
	return b.CreateCELProgram(workloadfilter.KubeEndpointCELGlobal, workloadfilter.ProductGlobal)
}
