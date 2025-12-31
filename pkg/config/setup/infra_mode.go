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
	mode             InfrastructureMode
	allowedChecks    []string
	additionalChecks []string
	excludedChecks   []string
}

var (
	infraModeConfig     infraModeConfigType
	infraModeConfigOnce sync.Once
)

// InfrastructureMode represents the agent's infrastructure mode
type InfrastructureMode string

// Infrastructure mode constants
const (
	InfraModeFull          InfrastructureMode = "full"
	InfraModeBasic         InfrastructureMode = "basic"
	InfraModeEndUserDevice InfrastructureMode = "end_user_device"
)

// GetCheckExclusiveModeFunc is a callback to get the exclusive mode for a check.
// This is set by the corechecks package to avoid circular imports.
// Returns the mode as a string which is then cast to InfrastructureMode.
var GetCheckExclusiveModeFunc func(checkName string) string

// getInfraModeConfig returns the infrastructure mode configuration, initializing it lazily.
func getInfraModeConfig() *infraModeConfigType {
	infraModeConfigOnce.Do(func() {
		cfg := Datadog()
		infraModeConfig = infraModeConfigType{
			mode:             InfrastructureMode(cfg.GetString("infrastructure_mode")),
			allowedChecks:    cfg.GetStringSlice("allowed_checks"),
			additionalChecks: cfg.GetStringSlice("allowed_additional_checks"),
			excludedChecks:   cfg.GetStringSlice("excluded_default_checks"),
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

	// Check if this check is exclusive to a specific mode
	if GetCheckExclusiveModeFunc != nil {
		exclusiveMode := InfrastructureMode(GetCheckExclusiveModeFunc(checkName))
		if exclusiveMode != "" {
			// This check is exclusive to a specific mode - only allow if current mode matches
			return cfg.mode == exclusiveMode
		}
	}

	// For non-exclusive checks, apply normal logic
	if cfg.mode == InfraModeFull || len(cfg.allowedChecks) == 0 || strings.HasPrefix(checkName, "custom_") {
		return true
	}

	// Check if it's in the allowed list
	return slices.Contains(cfg.allowedChecks, checkName) || slices.Contains(cfg.additionalChecks, checkName)
}

// IsCheckExcludedByInfraMode returns true if the check is in the excluded_default_checks list.
func IsCheckExcludedByInfraMode(checkName string) bool {
	return slices.Contains(getInfraModeConfig().excludedChecks, checkName)
}

// ResetInfraModeConfig resets the infrastructure mode configuration cache.
// This is only for testing purposes.
func ResetInfraModeConfig() {
	infraModeConfigOnce = sync.Once{}
}
