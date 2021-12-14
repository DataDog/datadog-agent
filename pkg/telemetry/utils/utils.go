// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"github.com/DataDog/datadog-agent/pkg/config"
)

// IsCheckEnabled returns if we want telemetry for the given check.
// Returns true if a * is present in the telemetry.checks list.
func IsCheckEnabled(checkName string) bool {
	// false if telemetry is disabled
	if !IsEnabled() {
		return false
	}

	// by default, we don't enable telemetry for every checks stats
	if config.Datadog.IsSet("telemetry.checks") {
		for _, check := range config.Datadog.GetStringSlice("telemetry.checks") {
			if check == "*" {
				return true
			} else if check == checkName {
				return true
			}
		}
	}
	return false
}

// IsEnabled returns whether or not telemetry is enabled
func IsEnabled() bool {
	return config.Datadog.IsSet("telemetry.enabled") && config.Datadog.GetBool("telemetry.enabled")
}
