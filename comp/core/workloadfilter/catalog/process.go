// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package catalog contains the implementation of the filtering catalogs.
package catalog

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
)

// LegacyProcessExcludeProgram creates a program for filtering processes based on legacy disallowlist patterns
func LegacyProcessExcludeProgram(config config.Component, logger log.Component) program.FilterProgram {
	programName := "LegacyProcessExcludeProgram"
	var initErrors []error

	processPatterns := config.GetStringSlice("process_config.blacklist_patterns")
	combinedPattern := strings.Join(processPatterns, "|")
	celRules := fmt.Sprintf("process.cmdline.matches(%s)", strconv.Quote(combinedPattern))
	excludeProgram, excludeErr := createCELProgram(celRules, workloadfilter.ProcessType)
	if excludeErr != nil {
		initErrors = append(initErrors, excludeErr)
		logger.Warnf("Error creating exclude program for %s: %v", programName, excludeErr)
	}

	return program.CELProgram{
		Name:                 programName,
		Exclude:              excludeProgram,
		InitializationErrors: initErrors,
	}
}
