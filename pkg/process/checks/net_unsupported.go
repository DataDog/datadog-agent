// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux && !windows

package checks

import (
	"errors"
	"net/http"
)

// fetchRemoteServiceData is not applicable on this platform; returns nil.
func fetchRemoteServiceData(_ *http.Client) (map[string][]string, map[uint32][]string, map[int32]int32) {
	return nil, nil, nil
}

// getRemoteProcessTags is not implemented on this platform; returns nil.
func getRemoteProcessTags(_ int32, _ map[uint32][]string, _ func(int32) ([]string, error)) []string {
	return nil
}

// getNetworkID fetches network_id
func getNetworkID(_ *http.Client) (string, error) {
	return "", errors.New("unsupported on this platform")
}
