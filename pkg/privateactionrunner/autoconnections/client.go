// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// httpDoer is an interface for making HTTP requests
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client is an HTTP client for the Connections API
type Client struct {
	httpClient httpDoer
	endpoint   string
	apiKey     string
	appKey     string
}

// NewConnectionAPIClient creates a new HTTP client for the Connections API
func NewConnectionAPIClient(cfg model.Reader) (*Client, error) {
	// 1. Validate credentials
	apiKey := utils.SanitizeAPIKey(cfg.GetString("api_key"))
	appKey := utils.SanitizeAPIKey(cfg.GetString("app_key"))
	if apiKey == "" || appKey == "" {
		return nil, fmt.Errorf("api_key and app_key required")
	}

	// 2. Resolve endpoint
	endpoint := utils.GetMainEndpoint(cfg, "https://api.", "dd_url")

	// 3. Create transport
	transport := httputils.CreateHTTPTransport(cfg)

	// 4. Wrap with ResetClient
	resetClient := httputils.NewResetClient(
		30*time.Second,
		func() *http.Client {
			return &http.Client{
				Timeout:   10 * time.Second,
				Transport: transport,
			}
		},
	)

	return &Client{
		httpClient: resetClient,
		endpoint:   endpoint,
		apiKey:     apiKey,
		appKey:     appKey,
	}, nil
}

// ConnectionRequest represents the JSON:API request structure
type ConnectionRequest struct {
	Data ConnectionRequestData `json:"data"`
}

// ConnectionRequestData represents the data section of the JSON:API request
type ConnectionRequestData struct {
	Type       string                      `json:"type"`
	Attributes ConnectionRequestAttributes `json:"attributes"`
}

// ConnectionRequestAttributes represents the attributes section of the request
type ConnectionRequestAttributes struct {
	Name        string            `json:"name"`
	RunnerID    string            `json:"runner_id"`
	Integration IntegrationConfig `json:"integration"`
}

// IntegrationConfig represents the integration configuration in the request
type IntegrationConfig struct {
	Type        string                 `json:"type"`
	Credentials map[string]interface{} `json:"credentials"`
}

// buildConnectionRequest builds an API request from a connection definition
func buildConnectionRequest(definition ConnectionDefinition, runnerID, name string) ConnectionRequest {
	credentials := map[string]interface{}{
		"type": definition.Credentials.Type,
	}

	for key, value := range definition.Credentials.AdditionalFields {
		credentials[key] = value
	}

	return ConnectionRequest{
		Data: ConnectionRequestData{
			Type: "action_connection",
			Attributes: ConnectionRequestAttributes{
				Name:     name,
				RunnerID: runnerID,
				Integration: IntegrationConfig{
					Type:        definition.IntegrationType,
					Credentials: credentials,
				},
			},
		},
	}
}

func (c *Client) CreateConnection(ctx context.Context, definition ConnectionDefinition, runnerID string) error {
	name := GenerateConnectionName(definition, runnerID)

	reqBody := buildConnectionRequest(definition, runnerID, name)

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.endpoint + "/api/v2/actions/connections"

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("DD-API-KEY", c.apiKey)
	httpReq.Header.Set("DD-APPLICATION-KEY", c.appKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "datadog-agent/"+version.AgentVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("API request failed: %d - %s", resp.StatusCode, string(respBody))
	}

	return nil
}
