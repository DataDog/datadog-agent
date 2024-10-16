// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !(process && linux)

package sysprobe

// GetSystemProbeConntrackCached is not supported without the process agent on linux
func GetSystemProbeConntrackCached(socketPath string) ([]byte, error) {
	return nil, errors.New("GetSystemProbeConntrackCached is not supported without the process agent on linux")
}

// GetSystemProbeConntrackHost is not supported without the process agent on linux
func GetSystemProbeConntrackHost(socketPath string) ([]byte, error) {
	return nil, errors.New("GetSystemProbeConntrackHost is not supported without the process agent on linux")
}
