// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
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

// IsCheckAllowedInInfraBasic returns true if the check is allowed.
// When not in basic mode, all checks are allowed (returns true).
// When in basic mode, only checks in the allowed list are permitted.
func IsCheckAllowedInInfraBasic(checkName string, cfg pkgconfigmodel.Reader) bool {
	allowedChecks := GetAllowedChecks(cfg)
	if len(allowedChecks) == 0 {
		return true
	}
	_, exists := allowedChecks[checkName]
	return exists
}
