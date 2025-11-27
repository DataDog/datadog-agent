// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	"regexp"
	"strings"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
)

// LegacyProcessExcludeProgram creates a regex-based program for filtering processes based on legacy disallowlist patterns
func LegacyProcessExcludeProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "LegacyProcessExcludeProgram"
	var initErrors []error

	extractFieldFunc := func(entity workloadfilter.Filterable) string {
		process, ok := entity.(*workloadfilter.Process)
		if !ok {
			return ""
		}
		return process.GetCmdline()
	}

	processPatterns := filterConfig.ProcessBlacklistPatterns
	if len(processPatterns) == 0 {
		return &program.RegexProgram{
			Name:                 programName,
			ExtractField:         extractFieldFunc,
			InitializationErrors: initErrors,
		}
	}

	combinedPattern := strings.Join(processPatterns, "|")
	// Compile the regex pattern
	excludeRegex, err := regexp.Compile(combinedPattern)
	if err != nil {
		initErrors = append(initErrors, err)
		logger.Warnf("Error compiling regex pattern for %s: %v", programName, err)
		return &program.RegexProgram{
			Name:                 programName,
			ExtractField:         extractFieldFunc,
			InitializationErrors: initErrors,
		}
	}

	return &program.RegexProgram{
		Name:                 programName,
		ExcludeRegex:         []*regexp.Regexp{excludeRegex},
		ExtractField:         extractFieldFunc,
		InitializationErrors: initErrors,
	}
}

// ProcessCELLogsProgram creates a program for filtering process logs via CEL rules
func ProcessCELLogsProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "ProcessCELLogsProgram"
	rule := filterConfig.GetCELRulesForProduct(workloadfilter.ProductLogs, workloadfilter.ProcessType)
	return createCELExcludeProgram(programName, rule, workloadfilter.ProcessType, logger)
}

// ProcessCELGlobalProgram creates a program for filtering processes globally via CEL rules
func ProcessCELGlobalProgram(filterConfig *FilterConfig, logger log.Component) program.FilterProgram {
	programName := "ProcessCELGlobalProgram"
	rule := filterConfig.GetCELRulesForProduct(workloadfilter.ProductGlobal, workloadfilter.ProcessType)
	return createCELExcludeProgram(programName, rule, workloadfilter.ProcessType, logger)
}
