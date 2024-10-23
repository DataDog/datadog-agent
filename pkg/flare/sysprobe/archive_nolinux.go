// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !(process && linux)

package sysprobe

import "errors"

// GetSystemProbeConntrackCached is not supported without the process agent on linux
func GetSystemProbeConntrackCached(_ string) ([]byte, error) {
	return nil, errors.New("GetSystemProbeConntrackCached is not supported without the process agent on linux")
}

// GetSystemProbeConntrackHost is not supported without the process agent on linux
func GetSystemProbeConntrackHost(_ string) ([]byte, error) {
	return nil, errors.New("GetSystemProbeConntrackHost is not supported without the process agent on linux")
}

// GetSystemProbeBTFLoaderInfo is not supported without the process agent on linux
func GetSystemProbeBTFLoaderInfo(socketPath string) ([]byte, error) {
	return nil, errors.New("GetSystemProbeBTFLoaderInfo is not supported without the process agent on linux")
}
