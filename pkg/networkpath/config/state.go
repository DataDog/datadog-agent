// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package config resolves the effective Network Path Dynamic Tests state.
package config

// Reader is the subset of Agent configuration needed to resolve Dynamic Tests.
type Reader interface {
	GetBool(string) bool
}

// DynamicTestsState is the mutually exclusive effective CNM Dynamic Tests state.
type DynamicTestsState uint8

const (
	// DynamicTestsOff disables CNM Dynamic Tests.
	DynamicTestsOff DynamicTestsState = iota
	// DynamicTestsBaseline enables the included, bounded baseline profile.
	DynamicTestsBaseline
	// DynamicTestsStandard enables explicitly configured recurring Dynamic Tests.
	DynamicTestsStandard
)

func (s DynamicTestsState) String() string {
	switch s {
	case DynamicTestsBaseline:
		return "baseline"
	case DynamicTestsStandard:
		return "standard"
	default:
		return "off"
	}
}

const (
	standardEnabledKey = "network_path.connections_monitoring.enabled"
	baselineEnabledKey = "network_path.connections_monitoring.baseline_tests_enabled"
	systemProbeKey     = "system_probe_config.enabled"
	networkConfigKey   = "network_config.enabled"
)

// ResolveDynamicTestsState combines core and system-probe settings. Dynamic
// Tests never activate outside effective CNM, and standard takes precedence.
func ResolveDynamicTestsState(core, systemProbe Reader) DynamicTestsState {
	if core == nil || systemProbe == nil || !systemProbe.GetBool(systemProbeKey) || !systemProbe.GetBool(networkConfigKey) {
		return DynamicTestsOff
	}
	if core.GetBool(standardEnabledKey) {
		return DynamicTestsStandard
	}
	if core.GetBool(baselineEnabledKey) {
		return DynamicTestsBaseline
	}
	return DynamicTestsOff
}
