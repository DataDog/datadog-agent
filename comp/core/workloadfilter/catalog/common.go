// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

// This file contains filter programs that can be shared across different entity types.

import (
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
)

// AutodiscoveryAnnotations creates an annotations program for autodiscovery
func AutodiscoveryAnnotations(b *ProgramBuilder) program.FilterProgram {
	return b.CreateAnnotationsProgram(workloadfilter.ContainerADAnnotations, "")
}

// AutodiscoveryMetricsAnnotations creates an annotations program for metrics autodiscovery
func AutodiscoveryMetricsAnnotations(b *ProgramBuilder) program.FilterProgram {
	return b.CreateAnnotationsProgram(workloadfilter.ContainerADAnnotationsMetrics, "metrics_")
}

// AutodiscoveryLogsAnnotations creates an annotations program for logs autodiscovery
func AutodiscoveryLogsAnnotations(b *ProgramBuilder) program.FilterProgram {
	return b.CreateAnnotationsProgram(workloadfilter.ContainerADAnnotationsLogs, "logs_")
}

// LegacyContainerGlobalProgram creates a legacy filter program for global containerized filtering
func LegacyContainerGlobalProgram(b *ProgramBuilder) program.FilterProgram {
	return b.CreateLegacyProgram(
		workloadfilter.ContainerLegacyGlobal,
		b.config.ContainerInclude,
		b.config.ContainerExclude,
	)
}

// LegacyContainerMetricsProgram creates a legacy filter program for containerized metrics filtering
func LegacyContainerMetricsProgram(b *ProgramBuilder) program.FilterProgram {
	return b.CreateLegacyProgram(
		workloadfilter.ContainerLegacyMetrics,
		b.config.ContainerIncludeMetrics,
		b.config.ContainerExcludeMetrics,
	)
}

// LegacyContainerLogsProgram creates a legacy filter program for containerized logs filtering
func LegacyContainerLogsProgram(b *ProgramBuilder) program.FilterProgram {
	return b.CreateLegacyProgram(
		workloadfilter.ContainerLegacyLogs,
		b.config.ContainerIncludeLogs,
		b.config.ContainerExcludeLogs,
	)
}

// LegacyContainerACExcludeProgram creates a legacy filter program for containerized AC exclusion filtering
func LegacyContainerACExcludeProgram(b *ProgramBuilder) program.FilterProgram {
	return b.CreateLegacyProgram(
		workloadfilter.ContainerLegacyACExclude,
		nil,
		b.config.ACExclude,
	)
}

// LegacyContainerACIncludeProgram creates a legacy filter program for containerized AC inclusion filtering
func LegacyContainerACIncludeProgram(b *ProgramBuilder) program.FilterProgram {
	return b.CreateLegacyProgram(
		workloadfilter.ContainerLegacyACInclude,
		b.config.ACInclude,
		nil,
	)
}

// LegacyContainerSBOMProgram creates a legacy filter program for containerized SBOM filtering
func LegacyContainerSBOMProgram(b *ProgramBuilder) program.FilterProgram {
	return b.CreateLegacyProgram(
		workloadfilter.ContainerLegacySBOM,
		b.config.SBOMContainerInclude,
		b.config.SBOMContainerExclude,
	)
}
