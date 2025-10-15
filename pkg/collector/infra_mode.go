// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"slices"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

var infraBasicAllowedChecks = map[string]struct{}{
	"cpu":               {},
	"agent_telemetry":   {},
	"agentcrashdetect":  {},
	"disk":              {},
	"file_handle":       {},
	"filehandles":       {},
	"io":                {},
	"load":              {},
	"memory":            {},
	"network":           {},
	"ntp":               {},
	"process":           {},
	"service_discovery": {},
	"system":            {},
	"system_core":       {},
	"system_swap":       {},
	"telemetry":         {},
	"telemetryCheck":    {},
	"uptime":            {},
	"win32_event_log":   {},
	"wincrashdetect":    {},
	"winkmem":           {},
	"winproc":           {},
}

// GetAllowedChecks returns the map of allowed checks for infra basic mode,
// including any additional checks specified in the configuration via 'infra_basic_additional_checks'
// when running in full mode, all checks are allowed (returns an empty map)
func GetAllowedChecks(cfg pkgconfigmodel.Reader) map[string]struct{} {
	if cfg.GetString("infrastructure_mode") != "basic" {
		return make(map[string]struct{})
	}

	// Copy the default allowed checks
	allowedMap := make(map[string]struct{}, len(infraBasicAllowedChecks))
	for check := range infraBasicAllowedChecks {
		allowedMap[check] = struct{}{}
	}

	// Add any additional checks from config
	additionalChecks := cfg.GetStringSlice("infra_basic_additional_checks")
	for _, check := range additionalChecks {
		allowedMap[check] = struct{}{}
	}

	return allowedMap
}

// IsCheckAllowed returns true if the check is allowed.
// When not in basic mode, all checks are allowed (returns true).
// When in basic mode, only checks in the allowed list are permitted.
func IsCheckAllowed(checkName string, cfg pkgconfigmodel.Reader) bool {
	// When not in basic mode, all checks are allowed
	if cfg.GetString("infrastructure_mode") != "basic" {
		return true
	}

	// Check if it's in the default allowed checks
	if _, exists := infraBasicAllowedChecks[checkName]; exists {
		return true
	}

	// Check if it's in the additional checks from config
	additionalChecks := cfg.GetStringSlice("infra_basic_additional_checks")
	return slices.Contains(additionalChecks, checkName)
}
