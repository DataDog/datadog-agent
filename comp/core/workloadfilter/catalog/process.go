// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
)

// LegacyProcessExcludeProgram creates a regex-based program for filtering processes based on legacy disallowlist patterns
func LegacyProcessExcludeProgram(b *ProgramBuilder) program.FilterProgram {
	extractFieldFunc := func(entity workloadfilter.Filterable) string {
		process, ok := entity.(*workloadfilter.Process)
		if !ok {
			return ""
		}
		return process.GetCmdline()
	}

	return b.CreateRegexProgram(
		workloadfilter.ProcessLegacyExclude,
		b.config.ProcessBlacklistPatterns,
		extractFieldFunc,
	)
}

// ProcessCELLogsProgram creates a program for filtering process logs via CEL rules
func ProcessCELLogsProgram(b *ProgramBuilder) program.FilterProgram {
	return b.CreateCELProgram(workloadfilter.ProcessCELLogs, workloadfilter.ProductLogs)
}

// ProcessCELGlobalProgram creates a program for filtering processes globally via CEL rules
func ProcessCELGlobalProgram(b *ProgramBuilder) program.FilterProgram {
	return b.CreateCELProgram(workloadfilter.ProcessCELGlobal, workloadfilter.ProductGlobal)
}
