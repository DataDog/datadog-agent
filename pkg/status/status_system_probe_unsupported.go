// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !process

package status

// GetSystemProbeStats returns a notice that it is not supported on systems that do not at least build the process agent
//
//nolint:revive // TODO(ASC) Fix revive linter
func GetSystemProbeStats(socketPath string) map[string]interface{} {
	return map[string]interface{}{
		"Errors": "System Probe is not supported on this system",
	}
}
