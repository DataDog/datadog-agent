// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !process

// Package systemprobe fetch information about the system probe
package systemprobe

import (
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
)

// GetStatus returns a notice that it is not supported on systems that do not at least build the process agent
func GetStatus(stats map[string]interface{}, _ string) {
	stats["systemProbeStats"] = map[string]interface{}{
		"Errors": "System Probe is not supported on this system",
	}
}

// GetProvider returns nil
func GetProvider(_ sysprobeconfig.Component) status.Provider {
	return nil
}
