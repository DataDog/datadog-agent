// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// getDefaultInfraBasicAllowedChecks returns the default list of allowed checks for infra basic mode
func getDefaultInfraBasicAllowedChecks() map[string]struct{} {
	return map[string]struct{}{
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
}

// GetInfraBasicAllowedChecks returns the map of allowed checks for infra basic mode,
// including any additional checks specified in the configuration via 'infra_basic_additional_checks'
func GetInfraBasicAllowedChecks(cfg pkgconfigmodel.Reader) map[string]struct{} {
	allowedMap := make(map[string]struct{})

	if cfg.GetString("infrastructure_mode") != "basic" {
		return allowedMap
	}

	allowedMap = getDefaultInfraBasicAllowedChecks()
	additionalChecks := cfg.GetStringSlice("infra_basic_additional_checks")

	for _, check := range additionalChecks {
		allowedMap[check] = struct{}{}
	}

	return allowedMap
}

// IsCheckAllowedInInfraBasic returns true if the check is allowed.
// When not in basic mode, all checks are allowed (returns true).
// When in basic mode, only checks in the allowed list are permitted.
func IsCheckAllowedInInfraBasic(checkName string, cfg pkgconfigmodel.Reader) bool {
	infraBasicAllowedChecks := GetInfraBasicAllowedChecks(cfg)
	if len(infraBasicAllowedChecks) == 0 {
		return true
	}

	_, exists := infraBasicAllowedChecks[checkName]
	return exists
}
