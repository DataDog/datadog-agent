// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package testutil

import (
	"fmt"
	"io"
	"net/http"
)

// HTTPGet fetches a URL and reads the whole body
func HTTPGet(client *http.Client, url string) (int, string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return 0, "", fmt.Errorf("failed to get '%s': %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", fmt.Errorf("failed to ReadAll from '%s': %w", url, err)
	}

	return resp.StatusCode, string(body), nil
}
