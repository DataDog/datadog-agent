// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockResetClient wraps an http.Client to implement the ResetClient interface for testing
type mockResetClient struct {
	client *http.Client
}

func (m *mockResetClient) Do(req *http.Request) (*http.Response, error) {
	return m.client.Do(req)
}

func TestCreateConnection_CorrectHTTPRequest(t *testing.T) {
	var receivedHeaders http.Header
	var receivedBody string
	var receivedMethod string
	var receivedPath string

	// Mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		receivedHeaders = r.Header

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		receivedBody = string(body)

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data": {"id": "conn-123"}}`))
	}))
	defer server.Close()

	// Create client with mock server endpoint
	client := &Client{
		httpClient: &mockResetClient{client: &http.Client{}},
		endpoint:   server.URL,
		apiKey:     "test-api-key",
		appKey:     "test-app-key",
	}

	httpDef := ConnectionDefinition{
		BundleID:        "com.datadoghq.http",
		IntegrationType: "HTTP",
		Credentials: CredentialConfig{
			Type:             "HTTPNoAuth",
			AdditionalFields: nil,
		},
	}

	err := client.CreateConnection(context.Background(), httpDef, "runner-abc123")

	require.NoError(t, err, "CreateConnection should not return error for 201 response")

	// Verify request details
	assert.Equal(t, "POST", receivedMethod, "Method should be POST")
	assert.Equal(t, "/api/v2/actions/connections", receivedPath, "Path should be correct")
	assert.Equal(t, "test-api-key", receivedHeaders.Get("DD-API-KEY"), "DD-API-KEY header should match")
	assert.Equal(t, "test-app-key", receivedHeaders.Get("DD-APPLICATION-KEY"), "DD-APPLICATION-KEY header should match")
	assert.Equal(t, "application/json", receivedHeaders.Get("Content-Type"), "Content-Type should be application/json")
	assert.Contains(t, receivedHeaders.Get("User-Agent"), "datadog-agent/", "User-Agent should contain datadog-agent/")
	assert.Contains(t, receivedBody, `"name":"HTTP (runner-abc123)"`, "Body should contain connection name")
}

func TestCreateConnection_StatusCodeHandling(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedError  bool
		errorSubstring string
	}{
		{
			name:          "201 Created - success",
			statusCode:    http.StatusCreated,
			responseBody:  `{"data": {"id": "conn-123"}}`,
			expectedError: false,
		},
		{
			name:           "400 Bad Request",
			statusCode:     http.StatusBadRequest,
			responseBody:   `{"errors": ["invalid request"]}`,
			expectedError:  true,
			errorSubstring: "400",
		},
		{
			name:           "403 Forbidden",
			statusCode:     http.StatusForbidden,
			responseBody:   `{"errors": ["forbidden"]}`,
			expectedError:  true,
			errorSubstring: "403",
		},
		{
			name:           "500 Server Error",
			statusCode:     http.StatusInternalServerError,
			responseBody:   `{"errors": ["internal error"]}`,
			expectedError:  true,
			errorSubstring: "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock HTTP server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			// Create client with mock server endpoint
			client := &Client{
				httpClient: &mockResetClient{client: &http.Client{}},
				endpoint:   server.URL,
				apiKey:     "test-api-key",
				appKey:     "test-app-key",
			}

			httpDef := ConnectionDefinition{
				BundleID:        "com.datadoghq.http",
				IntegrationType: "HTTP",
				Credentials: CredentialConfig{
					Type:             "HTTPNoAuth",
					AdditionalFields: nil,
				},
			}

			err := client.CreateConnection(context.Background(), httpDef, "runner-abc123")

			if tt.expectedError {
				require.Error(t, err, "Should return error for non-2xx status")
				assert.Contains(t, err.Error(), tt.errorSubstring, "Error should contain status code")
				assert.Contains(t, err.Error(), tt.responseBody, "Error should contain response body")
			} else {
				require.NoError(t, err, "Should not return error for 201 status")
			}
		})
	}
}

func TestGetBundleKeyForDefinition(t *testing.T) {
	// Test with known definition
	httpDef := ConnectionDefinition{
		BundleID:        "com.datadoghq.http",
		IntegrationType: "HTTP",
		Credentials: CredentialConfig{
			Type: "HTTPNoAuth",
		},
	}

	key := getBundleKeyForDefinition(httpDef)
	assert.Equal(t, "http", key, "Should return correct bundle key for HTTP")

	// Test with unknown definition
	unknownDef := ConnectionDefinition{
		BundleID:        "com.datadoghq.unknown",
		IntegrationType: "Unknown",
		Credentials: CredentialConfig{
			Type: "Unknown",
		},
	}

	key = getBundleKeyForDefinition(unknownDef)
	assert.Equal(t, "unknown", key, "Should return 'unknown' for unrecognized bundle")
}

func TestAutoCreateConnections_AllBundlesSuccess(t *testing.T) {
	createdConnections := []string{}

	// Mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		createdConnections = append(createdConnections, string(body))

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data": {"id": "conn-123"}}`))
	}))
	defer server.Close()

	cfg := mock.New(t)
	cfg.SetWithoutSource("api_key", "test-api-key")
	cfg.SetWithoutSource("app_key", "test-app-key")
	cfg.SetWithoutSource("dd_url", server.URL)

	runnerURN := "urn:dd:apps:on-prem-runner:us1:12345:runner-abc123"
	allowlist := []string{"com.datadoghq.http.*", "com.datadoghq.kubernetes.*", "com.datadoghq.script"}

	err := AutoCreateConnections(context.Background(), cfg, runnerURN, allowlist)

	require.NoError(t, err, "AutoCreateConnections should return nil")
	assert.Len(t, createdConnections, 3, "Should create 3 connections")

	// Verify each connection was created
	allBodies := strings.Join(createdConnections, " ")
	assert.Contains(t, allBodies, `"name":"HTTP (runner-abc123)"`)
	assert.Contains(t, allBodies, `"name":"Kubernetes (runner-abc123)"`)
	assert.Contains(t, allBodies, `"name":"Script (runner-abc123)"`)
}

func TestAutoCreateConnections_PartialFailures(t *testing.T) {
	requestCount := 0

	// Mock HTTP server - fail HTTP, succeed others
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		body, _ := io.ReadAll(r.Body)

		// Fail if it's the HTTP bundle
		if strings.Contains(string(body), `"name":"HTTP (runner-abc123)"`) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"errors": ["internal error"]}`))
			return
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data": {"id": "conn-123"}}`))
	}))
	defer server.Close()

	cfg := mock.New(t)
	cfg.SetWithoutSource("api_key", "test-api-key")
	cfg.SetWithoutSource("app_key", "test-app-key")
	cfg.SetWithoutSource("dd_url", server.URL)

	runnerURN := "urn:dd:apps:on-prem-runner:us1:12345:runner-abc123"
	allowlist := []string{"com.datadoghq.http.*", "com.datadoghq.kubernetes.*", "com.datadoghq.script"}

	err := AutoCreateConnections(context.Background(), cfg, runnerURN, allowlist)

	// Should return nil even with failures (non-blocking)
	require.NoError(t, err, "AutoCreateConnections should not propagate errors")
	assert.Equal(t, 3, requestCount, "Should attempt to create all 3 connections")
}

func TestAutoCreateConnections_NoRelevantBundles(t *testing.T) {
	requestCount := 0

	// Mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data": {"id": "conn-123"}}`))
	}))
	defer server.Close()

	cfg := mock.New(t)
	cfg.SetWithoutSource("api_key", "test-api-key")
	cfg.SetWithoutSource("app_key", "test-app-key")
	cfg.SetWithoutSource("dd_url", server.URL)

	runnerURN := "urn:dd:apps:on-prem-runner:us1:12345:runner-abc123"
	allowlist := []string{"com.datadoghq.gitlab.*"} // No matching bundles

	err := AutoCreateConnections(context.Background(), cfg, runnerURN, allowlist)

	require.NoError(t, err, "AutoCreateConnections should return nil")
	assert.Equal(t, 0, requestCount, "Should not make any HTTP requests")
}

func TestAutoCreateConnections_PartialAllowlist(t *testing.T) {
	createdConnections := []string{}

	// Mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		createdConnections = append(createdConnections, string(body))

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data": {"id": "conn-123"}}`))
	}))
	defer server.Close()

	cfg := mock.New(t)
	cfg.SetWithoutSource("api_key", "test-api-key")
	cfg.SetWithoutSource("app_key", "test-app-key")
	cfg.SetWithoutSource("dd_url", server.URL)

	runnerURN := "urn:dd:apps:on-prem-runner:us1:12345:runner-abc123"
	allowlist := []string{"com.datadoghq.http.*", "com.datadoghq.script"} // Only HTTP and Script

	err := AutoCreateConnections(context.Background(), cfg, runnerURN, allowlist)

	require.NoError(t, err, "AutoCreateConnections should return nil")
	assert.Len(t, createdConnections, 2, "Should create 2 connections")

	// Verify only HTTP and Script were created
	allBodies := strings.Join(createdConnections, " ")
	assert.Contains(t, allBodies, `"name":"HTTP (runner-abc123)"`)
	assert.Contains(t, allBodies, `"name":"Script (runner-abc123)"`)
	assert.NotContains(t, allBodies, `"name":"Kubernetes (runner-abc123)"`, "Should not create Kubernetes connection")
}
