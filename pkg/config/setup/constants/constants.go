// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package constants holds constants
package constants

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

const (
	// DefaultEBPFLessProbeAddr defines the default ebpfless probe address
	DefaultEBPFLessProbeAddr = "localhost:5678"
	// ClusterIDCacheKey is the key name for the orchestrator cluster id in the agent in-mem cache
	ClusterIDCacheKey = "orchestratorClusterID"
	// NodeKubeDistributionKey is the key name for the node kube distribution in the agent in-mem cache
	NodeKubeDistributionKey = "nodeKubeDistribution"
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

	// Start with default allowed checks
	allowedMap = getDefaultInfraBasicAllowedChecks()

	// Get additional checks from config
	// Config key: infra_basic_additional_checks
	// Example in datadog.yaml:
	//   infra_basic_additional_checks:
	//     - custom_check_1
	//     - custom_check_2
	additionalChecks := cfg.GetStringSlice("infra_basic_additional_checks")

	// Merge additional checks with defaults
	for _, check := range additionalChecks {
		allowedMap[check] = struct{}{}
	}

	return allowedMap
}

// IsCheckAllowedInInfraBasic returns true if the check is allowed in infrastructure basic mode
func IsCheckAllowedInInfraBasic(checkName string, cfg pkgconfigmodel.Reader) bool {
	// Start with default allowed checks
	allowedMap := GetInfraBasicAllowedChecks(cfg)

	// Check if the check name exists in the map
	_, exists := allowedMap[checkName]
	return exists
}
