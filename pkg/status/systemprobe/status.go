// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package systemprobe fetch information about the system probe
package systemprobe

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/core/status"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
)

// GetStatus returns the expvar stats of the system probe
func GetStatus(stats map[string]interface{}, socketPath string) {
	client := sysprobeclient.Get(socketPath)
	systemProbeDetails, err := getStats(client)
	if err != nil {
		stats["systemProbeStats"] = map[string]interface{}{
			"Errors": fmt.Sprintf("issue querying stats from system probe: %v", err),
		}
		return
	}
	stats["systemProbeStats"] = systemProbeDetails
}

func getStats(client *http.Client) (map[string]interface{}, error) {
	url := sysprobeclient.DebugURL("/stats")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("conn request failed: url: %s, status code: %d", req.URL, resp.StatusCode)
	}

	body, err := sysprobeclient.ReadAllResponseBody(resp)
	if err != nil {
		return nil, err
	}

	stats := make(map[string]interface{})
	err = json.Unmarshal(body, &stats)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

//go:embed status_templates
var templatesFS embed.FS

// RenderText renders system probe module stats as text.
func RenderText(stats map[string]any, buffer io.Writer) error {
	return status.RenderText(templatesFS, "systemprobe.tmpl", buffer, map[string]any{
		"systemProbeStats": stats,
	})
}
