// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package constants holds constants
package constants

const (
	// DefaultEBPFLessProbeAddr defines the default ebpfless probe address
	DefaultEBPFLessProbeAddr = "localhost:5678"
	// ClusterIDCacheKey is the key name for the orchestrator cluster id in the agent in-mem cache
	ClusterIDCacheKey = "orchestratorClusterID"
)

// constexpr
func GetInfraBasicAllowedChecks() []string {
	var allowed = [...]string{
		"cpu",
		"agent_telemetry",
		"agentcrashdetect",
		"disk",
		"file_handle",
		"filehandles",
		"io",
		"load",
		"memory",
		"network",
		"ntp",
		"process",
		"service_discovery",
		"system",
		"system_core",
		"system_swap",
		"telemetry",
		"telemetryCheck",
		"uptime",
		"win32_event_log",
		"wincrashdetect",
		"winkmem",
		"winproc",
	}
	return allowed[:]
}
