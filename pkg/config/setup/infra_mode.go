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

// Infrastructure mode constants
const (
	InfraModeFull          = "full"
	InfraModeBasic         = "basic"
	InfraModeEndUserDevice = "end_user_device"
)

// infraModeConfig holds the pre-computed infrastructure mode configuration
type infraModeConfigType struct {
	mode            string
	allowedChecks   []string
	excludedChecks  []string
	exclusiveChecks map[string][]string
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
			mode:            cfg.GetString("infrastructure_mode"),
			allowedChecks:   append(cfg.GetStringSlice("allowed_checks"), cfg.GetStringSlice("allowed_additional_checks")...),
			excludedChecks:  cfg.GetStringSlice("excluded_default_checks"),
			exclusiveChecks: cfg.GetStringMapStringSlice("exclusive_checks"),
		}
	})
	return &infraModeConfig
}

// IsCheckAllowedByInfraMode returns true if the check is allowed based on infrastructure mode settings.
// When infrastructure_mode is "full", all checks are allowed except mode-exclusive checks for other modes.
// When in a specific mode (e.g., "end_user_device"), checks exclusive to that mode are included.
// Otherwise, only checks in the allowlist (allowed_checks + allowed_additional_checks - excluded_default_checks) are permitted.
// Custom checks (starting with "custom_") are always allowed.
func IsCheckAllowedByInfraMode(checkName string) bool {
	cfg := getInfraModeConfig()

	// Check if it's excluded first
	if slices.Contains(cfg.excludedChecks, checkName) {
		return false
	}

	if cfg.mode == InfraModeFull || len(cfg.allowedChecks) == 0 || strings.HasPrefix(checkName, "custom_") || slices.Contains(cfg.exclusiveChecks[cfg.mode], checkName) {
		return true
	}

	// Check if it's in the allowed list
	return slices.Contains(cfg.allowedChecks, checkName)
}

// IsCheckExcludedByInfraMode returns true if the check is in the excluded_default_checks list.
func IsCheckExcludedByInfraMode(checkName string) bool {
	return slices.Contains(getInfraModeConfig().excludedChecks, checkName)
}

// GetExclusiveChecksForCurrentMode returns the list of exclusive checks for the current infrastructure mode.
// Returns nil if the mode is "full" or if there are no exclusive checks for the current mode.
func GetExclusiveChecksForCurrentMode() []string {
	cfg := getInfraModeConfig()
	if cfg.mode == InfraModeFull {
		return nil
	}
	return cfg.exclusiveChecks[cfg.mode]
}
