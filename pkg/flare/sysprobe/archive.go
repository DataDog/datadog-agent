// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build process

// Package sysprobe contains flare logic that only imports pkg/process/net when the process build tag is included
package sysprobe

import (
	"fmt"
	"io"
	"net/http"

	sysprobeclient "github.com/DataDog/datadog-agent/cmd/system-probe/api/client"
)

// GetSystemProbeTelemetry queries the telemetry endpoint from system-probe.
func GetSystemProbeTelemetry(client *http.Client) ([]byte, error) {
	url := sysprobeclient.URL("", "/telemetry")
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(`GetTelemetry got non-success status code: url: %s, status_code: %d, response: "%s"`, req.URL, resp.StatusCode, data)
	}

	return data, nil
}
