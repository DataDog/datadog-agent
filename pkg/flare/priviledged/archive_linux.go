// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package priviledged

import (
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
)

// GetLinuxDmesg returns the dmesg output from the system probe
func GetLinuxDmesg() ([]byte, error) {
	sysProbeClient := sysprobeclient.Get(GetSystemProbeSocketPath())
	url := sysprobeclient.DebugURL("/dmesg")
	return GetHTTPData(sysProbeClient, url)
}
