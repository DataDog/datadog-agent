// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build process

// Package sysprobe contains flare logic that only imports pkg/process/net when the process build tag is included
package sysprobe

import "github.com/DataDog/datadog-agent/pkg/process/net"

// GetSystemProbeTelemetry queries the telemetry endpoint from system-probe.
func GetSystemProbeTelemetry(socketPath string) ([]byte, error) {
	probeUtil, err := net.GetRemoteSystemProbeUtil(socketPath)
	if err != nil {
		return nil, err
	}
	return probeUtil.GetTelemetry()
}
