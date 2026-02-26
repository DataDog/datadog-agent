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

// getListeningPortToPIDMap is not implemented on this platform; returns nil.
func getListeningPortToPIDMap() map[int32]int32 {
	return nil
}

// fetchIISTagsCache is not applicable on this platform; returns nil.
func fetchIISTagsCache(_ *http.Client) map[string][]string {
	return nil
}

// fetchProcessCacheTags is not applicable on this platform; returns nil.
func fetchProcessCacheTags(_ *http.Client) map[uint32][]string {
	return nil
}

// getNetworkID fetches network_id
func getNetworkID(_ *http.Client) (string, error) {
	return "", errors.New("unsupported on this platform")
}
