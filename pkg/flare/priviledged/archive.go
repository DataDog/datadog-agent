// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package priviledged contains helper functions for system-probe and security-agent
package priviledged

import (
	"fmt"
	"io"
	"net/http"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// GetSystemProbeSocketPath returns the path to the system probe socket
func GetSystemProbeSocketPath() string {
	return pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")
}

// GetHTTPData fetches data from the given url
func GetHTTPData(client *http.Client, url string) ([]byte, error) {
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
		return nil, fmt.Errorf("non-ok status code: url: %s, status_code: %d, response: `%s`", req.URL, resp.StatusCode, string(data))
	}
	return data, nil
}
