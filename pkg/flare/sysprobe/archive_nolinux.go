// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux

package sysprobe

import (
	"errors"
	"net/http"
)

// GetSystemProbeConntrackCached is a stub for unsupported OSes
func GetSystemProbeConntrackCached(_ *http.Client) ([]byte, error) {
	return nil, errors.New("GetSystemProbeConntrackCached is not supported")
}

// GetSystemProbeConntrackHost is a stub for unsupported OSes
func GetSystemProbeConntrackHost(_ *http.Client) ([]byte, error) {
	return nil, errors.New("GetSystemProbeConntrackHost is not supported")
}

// GetSystemProbeBTFLoaderInfo is a stub for unsupported OSes
func GetSystemProbeBTFLoaderInfo(_ *http.Client) ([]byte, error) {
	return nil, errors.New("GetSystemProbeBTFLoaderInfo is not supported")
}
