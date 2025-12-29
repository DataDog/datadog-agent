// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"slices"
	"strings"
	"sync"
)

// infraModeConfig holds the pre-computed infrastructure mode configuration
type infraModeConfigType struct {
	mode           string
	allowedChecks  []string
	excludedChecks []string
}

var (
	infraModeConfig     infraModeConfigType
	infraModeConfigOnce sync.Once
)

// getInfraModeConfig returns the infrastructure mode configuration, initializing it lazily.
func getInfraModeConfig() *infraModeConfigType {
	infraModeConfigOnce.Do(func() {
		cfg := Datadog()
		infraModeConfig = infraModeConfigType{
			mode:           cfg.GetString("infrastructure_mode"),
			allowedChecks:  append(cfg.GetStringSlice("allowed_checks"), cfg.GetStringSlice("allowed_additional_checks")...),
			excludedChecks: cfg.GetStringSlice("excluded_default_checks"),
		}
	})
	return &infraModeConfig
}

// IsCheckAllowedByInfraMode returns true if the check is allowed based on infrastructure mode settings.
// When infrastructure_mode is "full", all checks are allowed.
// Otherwise, only checks in the allowlist (allowed_checks + allowed_additional_checks - excluded_default_checks) are permitted.
// Custom checks (starting with "custom_") are always allowed.
func IsCheckAllowedByInfraMode(checkName string) bool {
	cfg := getInfraModeConfig()

	// When in "full" mode, all checks are allowed
	if cfg.mode == "full" {
		return true
	}

	// Allow all custom checks
	if strings.HasPrefix(checkName, "custom_") {
		return true
	}

	// Check if it's excluded first
	if slices.Contains(cfg.excludedChecks, checkName) {
		return false
	}

	// Check if it's in the allowed list
	return slices.Contains(cfg.allowedChecks, checkName)
}

// IsCheckExcludedByInfraMode returns true if the check is in the excluded_default_checks list.
func IsCheckExcludedByInfraMode(checkName string) bool {
	return slices.Contains(getInfraModeConfig().excludedChecks, checkName)
}
