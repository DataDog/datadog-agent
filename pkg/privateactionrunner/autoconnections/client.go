// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	parutil "github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/jsonapi"
)

const (
	createConnectionInitialBackoff = 1 * time.Second
	createConnectionMaxBackoff     = 30 * time.Second
	// Bounded so a flaky API can't block startup indefinitely: connection
	// creation is best-effort (callers log-and-continue on failure), and we
	// don't want a single bad connection to delay the rest of the loop.
	createConnectionMaxElapsed = 5 * time.Minute
)

// ConnectionsClient is an HTTP client for creating connections via the Datadog API.
type ConnectionsClient struct {
	httpClient *http.Client
	baseUrl    string
	apiKey     string
	appKey     string
	retryOpts  parutil.RetryHTTPOptions
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
		retryOpts: parutil.RetryHTTPOptions{
			InitialInterval: createConnectionInitialBackoff,
			MaxInterval:     createConnectionMaxBackoff,
			MaxElapsedTime:  createConnectionMaxElapsed,
		},
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
	if c.appKey == "" {
		return errors.New("app key is required to create connections")
	}

	reqBody := buildConnectionRequest(definition, runnerID, runnerName, tags)

	body, err := jsonapi.Marshal(reqBody, jsonapi.MarshalClientMode())
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseUrl + createConnectionEndpoint

	_, err = parutil.RetryHTTPRequest(ctx, func() (struct{}, int, error) {
		statusCode, sendErr := c.sendCreateConnection(ctx, url, body)
		return struct{}{}, statusCode, sendErr
	}, c.retryOpts)
	return err
}

// sendCreateConnection performs a single connection-create POST and returns
// the HTTP status code along with any error. The status code is 0 for
// transport-level failures.
func (c *ConnectionsClient) sendCreateConnection(ctx context.Context, url string, body []byte) (int, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set(apiKeyHeader, c.apiKey)
	httpReq.Header.Set(appKeyHeader, c.appKey)
	httpReq.Header.Set(contentTypeHeader, contentType)
	httpReq.Header.Set(userAgentHeader, "datadog-agent/"+version.AgentVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("API request failed: %d - %s", resp.StatusCode, string(respBody))
	}

	return resp.StatusCode, nil
}
