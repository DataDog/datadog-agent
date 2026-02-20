// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// ContainerPausedProgram creates a program for filtering paused containers
func ContainerPausedProgram(b *ProgramBuilder) program.FilterProgram {
	return b.CreateLegacyProgram(
		workloadfilter.ContainerPaused,
		nil,
		containers.GetPauseContainerExcludeList(),
	)
}

// ContainerCELMetricsProgram creates a program for filtering container metrics via CEL rules
func ContainerCELMetricsProgram(b *ProgramBuilder) program.FilterProgram {
	return b.CreateCELProgram(workloadfilter.ContainerCELMetrics, workloadfilter.ProductMetrics)
}

// ContainerCELLogsProgram creates a program for filtering container logs via CEL rules
func ContainerCELLogsProgram(b *ProgramBuilder) program.FilterProgram {
	return b.CreateCELProgram(workloadfilter.ContainerCELLogs, workloadfilter.ProductLogs)
}

// ContainerCELSBOMProgram creates a program for filtering container SBOMs via CEL rules
func ContainerCELSBOMProgram(b *ProgramBuilder) program.FilterProgram {
	return b.CreateCELProgram(workloadfilter.ContainerCELSBOM, workloadfilter.ProductSBOM)
}

// ContainerCELGlobalProgram creates a program for filtering containers globally via CEL rules
func ContainerCELGlobalProgram(b *ProgramBuilder) program.FilterProgram {
	return b.CreateCELProgram(workloadfilter.ContainerCELGlobal, workloadfilter.ProductGlobal)
}

// ContainerLegacyRuntimeSecurityProgram creates a program for filtering containers for runtime security
func ContainerLegacyRuntimeSecurityProgram(b *ProgramBuilder) program.FilterProgram {
	return b.CreateLegacyProgram(
		workloadfilter.ContainerLegacyRuntimeSecurity,
		b.config.ContainerRuntimeSecurityInclude,
		b.config.ContainerRuntimeSecurityExclude,
	)
}

// ContainerLegacyComplianceProgram creates a program for filtering containers for compliance
func ContainerLegacyComplianceProgram(b *ProgramBuilder) program.FilterProgram {
	return b.CreateLegacyProgram(
		workloadfilter.ContainerLegacyCompliance,
		b.config.ContainerComplianceInclude,
		b.config.ContainerComplianceExclude,
	)
}
