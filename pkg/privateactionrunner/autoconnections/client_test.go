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
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/jsonapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConnectionAPIClient_ValidCredentials(t *testing.T) {
	cfg := mock.New(t)
	apiKey := "test-api-key"
	appKey := "test-app-key"
	ddSite := "datadoghq.com"

	client, err := NewConnectionsAPIClient(cfg, ddSite, apiKey, appKey)

	require.NoError(t, err)
	require.NotNil(t, client)
	assert.NotNil(t, client.httpClient)
	assert.NotEmpty(t, client.baseUrl)
	assert.Equal(t, "test-api-key", client.apiKey)
	assert.Equal(t, "test-app-key", client.appKey)
}

func TestBuildConnectionRequest_NoAdditionalFields(t *testing.T) {
	httpDef := ConnectionDefinition{
		BundleID:        "com.datadoghq.http",
		IntegrationType: "HTTP",
		Credentials: CredentialConfig{
			Type:             "HTTPNoAuth",
			AdditionalFields: nil,
		},
	}
	runnerID := "2112072a-b24c-4f23-b80e-d4e93484cf3a"
	runnerName := "runner-123"
	connectionName := "HTTP (runner-123)"

	request := buildConnectionRequest(httpDef, runnerID, runnerName)

	assert.Equal(t, connectionName, request.Name)
	assert.Equal(t, runnerID, request.RunnerID)
	assert.Equal(t, "HTTP", request.Integration.Type)
	assert.Equal(t, "HTTPNoAuth", request.Integration.Credentials["type"])
	assert.Len(t, request.Integration.Credentials, 1)
}

func TestBuildConnectionRequest_WithAdditionalFields(t *testing.T) {
	scriptDef := ConnectionDefinition{
		BundleID:        "com.datadoghq.script",
		IntegrationType: "Script",
		Credentials: CredentialConfig{
			Type: "Script",
			AdditionalFields: map[string]interface{}{
				"configFileLocation": "/etc/dd-action-runner/config/credentials/script.yaml",
			},
		},
	}
	runnerID := "2112072a-b24c-4f23-b80e-d4e93484cf3a"
	runnerName := "runner-456"
	connectionName := "Script (runner-456)"

	request := buildConnectionRequest(scriptDef, runnerID, runnerName)

	assert.Equal(t, connectionName, request.Name)
	assert.Equal(t, runnerID, request.RunnerID)
	assert.Equal(t, "Script", request.Integration.Type)
	assert.Equal(t, "Script", request.Integration.Credentials["type"])
	assert.Equal(t, "/etc/dd-action-runner/config/credentials/script.yaml",
		request.Integration.Credentials["configFileLocation"])
	assert.Len(t, request.Integration.Credentials, 2)
}

func TestCreateConnection_Success(t *testing.T) {
	var receivedMethod string
	var receivedPath string
	var receivedHeaders http.Header
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		receivedHeaders = r.Header

		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data": {"id": "conn-123"}}`))
	}))
	defer server.Close()

	client := &ConnectionsClient{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseUrl:    server.URL,
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

	err := client.CreateConnection(context.Background(), httpDef, "runner-id-123", "runner-name-abc")

	require.NoError(t, err)
	assert.Equal(t, "POST", receivedMethod)
	assert.Equal(t, "/api/v2/actions/connections", receivedPath)
	assert.Equal(t, "test-api-key", receivedHeaders.Get("DD-API-KEY"))
	assert.Equal(t, "test-app-key", receivedHeaders.Get("DD-APPLICATION-KEY"))
	assert.Contains(t, receivedHeaders.Get("User-Agent"), "datadog-agent/")
	assert.Contains(t, receivedBody, `"name":"HTTP (runner-name-abc)"`)
	assert.Contains(t, receivedBody, `"runner_id":"runner-id-123"`)
}

func TestCreateConnection_ErrorResponses(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		responseBody  string
		expectedError string
	}{
		{
			name:          "400 Bad Request",
			statusCode:    http.StatusBadRequest,
			responseBody:  `{"errors": ["invalid request"]}`,
			expectedError: "400",
		},
		{
			name:          "403 Forbidden",
			statusCode:    http.StatusForbidden,
			responseBody:  `{"errors": ["forbidden"]}`,
			expectedError: "403",
		},
		{
			name:          "500 Internal Server Error",
			statusCode:    http.StatusInternalServerError,
			responseBody:  `{"errors": ["server error"]}`,
			expectedError: "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := &ConnectionsClient{
				httpClient: &http.Client{Timeout: 10 * time.Second},
				baseUrl:    server.URL,
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

			err := client.CreateConnection(context.Background(), httpDef, "runner-id-123", "runner-name-abc")

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedError)
			assert.Contains(t, err.Error(), tt.responseBody)
		})
	}
}

func TestBuildConnectionRequest_HTTPWithBaseURL(t *testing.T) {
	httpDef := ConnectionDefinition{
		BundleID:        "com.datadoghq.http",
		IntegrationType: "HTTP",
		IntegrationFields: map[string]interface{}{
			"base_url": "https://internal.example.com",
		},
		Credentials: CredentialConfig{
			Type:             "HTTPNoAuth",
			AdditionalFields: nil,
		},
	}
	runnerID := "runner-123"
	runnerName := "test-runner"

	request := buildConnectionRequest(httpDef, runnerID, runnerName)

	assert.Equal(t, "HTTP (test-runner)", request.Name)
	assert.Equal(t, runnerID, request.RunnerID)
	assert.Equal(t, "HTTP", request.Integration.Type)
	assert.Equal(t, "https://internal.example.com", request.Integration.BaseURL)
	assert.Equal(t, "HTTPNoAuth", request.Integration.Credentials["type"])
	assert.Len(t, request.Integration.Credentials, 1)
}

func TestBuildConnectionRequest_KubernetesNoIntegrationFields(t *testing.T) {
	k8sDef := ConnectionDefinition{
		BundleID:          "com.datadoghq.kubernetes",
		IntegrationType:   "Kubernetes",
		IntegrationFields: nil,
		Credentials: CredentialConfig{
			Type:             "KubernetesServiceAccount",
			AdditionalFields: nil,
		},
	}
	runnerID := "runner-123"
	runnerName := "test-runner"

	request := buildConnectionRequest(k8sDef, runnerID, runnerName)

	assert.Equal(t, "Kubernetes (test-runner)", request.Name)
	assert.Equal(t, runnerID, request.RunnerID)
	assert.Equal(t, "Kubernetes", request.Integration.Type)
	assert.Empty(t, request.Integration.BaseURL)
	assert.Equal(t, "KubernetesServiceAccount", request.Integration.Credentials["type"])
	assert.Len(t, request.Integration.Credentials, 1)
}

func TestBuildConnectionRequest_JSONStructureMatchesAPISpec(t *testing.T) {
	tests := []struct {
		name                 string
		definition           ConnectionDefinition
		runnerID             string
		runnerName           string
		expectedJSONContains []string
	}{
		{
			name: "HTTP with base_url",
			definition: ConnectionDefinition{
				BundleID:        "com.datadoghq.http",
				IntegrationType: "HTTP",
				IntegrationFields: map[string]interface{}{
					"base_url": "https://internal.example.com",
				},
				Credentials: CredentialConfig{
					Type: "HTTPNoAuth",
				},
			},
			runnerID:   "runner-123",
			runnerName: "My OnPrem Connection",
			expectedJSONContains: []string{
				`"type":"action_connection"`,
				`"name":"HTTP (My OnPrem Connection)"`,
				`"runner_id":"runner-123"`,
				`"integration":{`,
				`"type":"HTTP"`,
				`"base_url":"https://internal.example.com"`,
				`"credentials":{`,
				`"type":"HTTPNoAuth"`,
			},
		},
		{
			name: "Kubernetes with service account",
			definition: ConnectionDefinition{
				BundleID:        "com.datadoghq.kubernetes",
				IntegrationType: "Kubernetes",
				Credentials: CredentialConfig{
					Type: "KubernetesServiceAccount",
				},
			},
			runnerID:   "runner-123",
			runnerName: "My Kubernetes OnPrem Connection",
			expectedJSONContains: []string{
				`"type":"action_connection"`,
				`"name":"Kubernetes (My Kubernetes OnPrem Connection)"`,
				`"runner_id":"runner-123"`,
				`"integration":{`,
				`"type":"Kubernetes"`,
				`"credentials":{`,
				`"type":"KubernetesServiceAccount"`,
			},
		},
		{
			name: "Script with config file location",
			definition: ConnectionDefinition{
				BundleID:        "com.datadoghq.script",
				IntegrationType: "Script",
				Credentials: CredentialConfig{
					Type: "Script",
					AdditionalFields: map[string]interface{}{
						"configFileLocation": "/path/to/config",
					},
				},
			},
			runnerID:   "runner-123",
			runnerName: "My Script OnPrem Connection",
			expectedJSONContains: []string{
				`"type":"action_connection"`,
				`"name":"Script (My Script OnPrem Connection)"`,
				`"runner_id":"runner-123"`,
				`"integration":{`,
				`"type":"Script"`,
				`"credentials":{`,
				`"type":"Script"`,
				`"configFileLocation":"/path/to/config"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := buildConnectionRequest(tt.definition, tt.runnerID, tt.runnerName)

			jsonBytes, err := jsonapi.Marshal(request, jsonapi.MarshalClientMode())
			require.NoError(t, err)

			jsonString := string(jsonBytes)
			for _, expected := range tt.expectedJSONContains {
				assert.Contains(t, jsonString, expected, "JSON should contain: %s", expected)
			}
		})
	}
}
