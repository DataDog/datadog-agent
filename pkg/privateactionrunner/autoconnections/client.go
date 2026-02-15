// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/jsonapi"
)

// ConnectionsClient is an HTTP client for creating connections via the Datadog API.
type ConnectionsClient struct {
	httpClient *http.Client
	baseUrl    string
	apiKey     string
	appKey     string
}

func NewConnectionsAPIClient(cfg model.Reader, ddSite, apiKey, appKey string) (*ConnectionsClient, error) {
	baseUrl := "https://api." + ddSite

	transport := httputils.CreateHTTPTransport(cfg)
	httpClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}

	return &ConnectionsClient{
		httpClient: httpClient,
		baseUrl:    baseUrl,
		apiKey:     apiKey,
		appKey:     appKey,
	}, nil
}

type ConnectionRequest struct {
	ID          string            `jsonapi:"primary,action_connection"`
	Name        string            `jsonapi:"attribute" json:"name" validate:"required"`
	RunnerID    string            `jsonapi:"attribute" json:"runner_id" validate:"required"`
	Tags        []string          `jsonapi:"attribute" json:"tags"`
	Integration IntegrationConfig `jsonapi:"attribute" json:"integration" validate:"required"`
}

type IntegrationConfig struct {
	Type        string                 `json:"type" validate:"required"`
	Credentials map[string]interface{} `json:"credentials" validate:"required"`
}

func buildConnectionRequest(definition ConnectionDefinition, runnerID, runnerName string, tags []string) ConnectionRequest {
	connectionName := GenerateConnectionName(definition, runnerName)

	credentials := map[string]interface{}{
		"type": definition.Credentials.Type,
	}

	for key, value := range definition.Credentials.AdditionalFields {
		credentials[key] = value
	}

	return ConnectionRequest{
		Name:     connectionName,
		RunnerID: runnerID,
		Tags:     tags,
		Integration: IntegrationConfig{
			Type:        definition.IntegrationType,
			Credentials: credentials,
		},
	}
}

func (c *ConnectionsClient) CreateConnection(ctx context.Context, definition ConnectionDefinition, runnerID, runnerName string, tags []string) error {
	reqBody := buildConnectionRequest(definition, runnerID, runnerName, tags)

	body, err := jsonapi.Marshal(reqBody, jsonapi.MarshalClientMode())
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseUrl + createConnectionEndpoint

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set(apiKeyHeader, c.apiKey)
	httpReq.Header.Set(appKeyHeader, c.appKey)
	httpReq.Header.Set(contentTypeHeader, contentType)
	httpReq.Header.Set(userAgentHeader, "datadog-agent/"+version.AgentVersion)

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
