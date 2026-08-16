// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package config resolves Network Path configuration shared across processes.
package config

// Reader is the subset of Agent configuration needed to resolve baseline
// Dynamic Tests.
type Reader interface {
	GetBool(string) bool
}

const (
	standardEnabledKey = "network_path.connections_monitoring.enabled"
	baselineEnabledKey = "network_path.connections_monitoring.baseline_tests_enabled"
	networkConfigKey   = "network_config.enabled"
)

// BaselineEnabled reports whether baseline Dynamic Tests are enabled for an
// Agent running Cloud Network Monitoring.
func BaselineEnabled(core, systemProbe Reader) bool {
	return core != nil && systemProbe != nil && !core.GetBool(standardEnabledKey) && core.GetBool(baselineEnabledKey) && systemProbe.GetBool(networkConfigKey)
}
