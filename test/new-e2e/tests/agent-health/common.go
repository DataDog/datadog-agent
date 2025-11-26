// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// HealthReport represents the structure of health reports sent to the intake
type HealthReport struct {
	SchemaVersion string                  `json:"schema_version"`
	EventType     string                  `json:"event_type"`
	EmittedAt     string                  `json:"emitted_at"`
	Host          HostInfo                `json:"host"`
	Issues        map[string]*IssueReport `json:"issues"`
}

// HostInfo contains host information
type HostInfo struct {
	Hostname     string `json:"hostname"`
	AgentVersion string `json:"agent_version"`
}

// IssueReport represents a single health issue
type IssueReport struct {
	ID          string         `json:"ID"`
	Category    string         `json:"Category"`
	Title       string         `json:"Title"`
	Description string         `json:"Description"`
	Tags        []string       `json:"Tags"`
	Extra       map[string]any `json:"Extra,omitempty"`
}

// getAgentHealthPayloads fetches agent health payloads from the fake intake
func getAgentHealthPayloads(client *fakeintake.Client) ([]api.Payload, error) {
	// Build the URL to query the fakeintake for agent health payloads
	u, err := url.Parse(client.URL())
	if err != nil {
		return nil, err
	}
	u.Path = "/fakeintake/payloads"
	q := u.Query()
	q.Set("endpoint", "/api/v2/agenthealth")
	u.RawQuery = q.Encode()

	// Make the HTTP request
	resp, err := http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read and parse the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response api.APIFakeIntakePayloadsRawGETResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}

	return response.Payloads, nil
}
