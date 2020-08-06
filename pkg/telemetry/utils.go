package telemetry

import (
	"github.com/StackVista/stackstate-agent/pkg/config"
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
