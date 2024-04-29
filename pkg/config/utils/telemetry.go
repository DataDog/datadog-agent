// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// IsCheckTelemetryEnabled returns if we want telemetry for the given check.
// Returns true if a * is present in the telemetry.checks list.
func IsCheckTelemetryEnabled(checkName string, cfg pkgconfigmodel.Reader) bool {
	// when agent_telemetry is enabled, we enable telemetry for every check
	if isAgentTelemetryEnabled(cfg) {
		return true
	}

	// false if telemetry is disabled
	if !IsTelemetryEnabled(cfg) {
		return false
	}

	// by default, we don't enable telemetry for every checks stats
	if cfg.IsSet("telemetry.checks") {
		for _, check := range cfg.GetStringSlice("telemetry.checks") {
			if check == "*" {
				return true
			} else if check == checkName {
				return true
			}
		}
	}
	return false
}

// IsTelemetryEnabled returns whether or not telemetry is enabled
func IsTelemetryEnabled(cfg pkgconfigmodel.Reader) bool {
	return cfg.IsSet("telemetry.enabled") && cfg.GetBool("telemetry.enabled") ||
		(isAgentTelemetryEnabled(cfg))
}

// isAgentTelemetryEnabled returns whether or not agent telemetry is enabled
func isAgentTelemetryEnabled(cfg pkgconfigmodel.Reader) bool {
	return cfg.IsSet("agent_telemetry.enabled") && cfg.GetBool("agent_telemetry.enabled")
}
