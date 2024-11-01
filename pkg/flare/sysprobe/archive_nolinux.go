// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !(process && linux)

package sysprobe

import "errors"

// GetSystemProbeConntrackCached is a stub designed to prevent builds without the process agent from importing pkg/process/net
func GetSystemProbeConntrackCached(_ string) ([]byte, error) {
	return nil, errors.New("GetSystemProbeConntrackCached is not supported")
}

// GetSystemProbeConntrackHost is a stub designed to prevent builds without the process agent from importing pkg/process/net
func GetSystemProbeConntrackHost(_ string) ([]byte, error) {
	return nil, errors.New("GetSystemProbeConntrackHost is not supported")
}

// GetSystemProbeBTFLoaderInfo is a stub designed to prevent builds without the process agent from importing pkg/process/net
func GetSystemProbeBTFLoaderInfo(_ string) ([]byte, error) {
	return nil, errors.New("GetSystemProbeBTFLoaderInfo is not supported")
}
