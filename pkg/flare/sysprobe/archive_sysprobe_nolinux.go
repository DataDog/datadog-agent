// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !(process && linux)

package sysprobe

// GetSystemProbeConntrackHostJSON is not supported without the process agent on linux
func GetSystemProbeConntrackHostJSON(socketPath string) ([]byte, error) {
	return nil, errors.New("GetSystemProbeConntrackHostJSON is not supported without the process agent on linux")
}

// GetSystemProbeConntrackCachedJSON is not supported without the process agent on linux
func GetSystemProbeConntrackCachedJSON(socketPath string) ([]byte, error) {
	return nil, errors.New("GetSystemProbeConntrackCachedJSON is not supported without the process agent on linux")
}
