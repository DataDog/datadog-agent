// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package util

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// LocalProcessCollectorIsEnabled returns whether the local process collector is enabled
// based on agent flavor and config values. This prevents any conflict between the process collectors
// and unnecessary data collection. Always returns false outside of linux.
func LocalProcessCollectorIsEnabled() bool {
	if flavor.GetFlavor() != flavor.DefaultAgent {
		return false
	}

	processChecksInCoreAgent := config.Datadog().GetBool("process_config.run_in_core_agent.enabled")
	langDetectionEnabled := config.Datadog().GetBool("language_detection.enabled")

	return langDetectionEnabled && processChecksInCoreAgent
}
