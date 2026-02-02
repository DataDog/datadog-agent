// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConnectionAPIClient_MissingCredentials(t *testing.T) {
	tests := []struct {
		name    string
		apiKey  string
		appKey  string
		wantErr string
	}{
		{
			name:    "missing api_key",
			apiKey:  "",
			appKey:  "valid-app-key",
			wantErr: "api_key and app_key required",
		},
		{
			name:    "missing app_key",
			apiKey:  "valid-api-key",
			appKey:  "",
			wantErr: "api_key and app_key required",
		},
		{
			name:    "missing both keys",
			apiKey:  "",
			appKey:  "",
			wantErr: "api_key and app_key required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mock.New(t)
			cfg.SetWithoutSource("api_key", tt.apiKey)
			cfg.SetWithoutSource("app_key", tt.appKey)

			client, err := NewConnectionAPIClient(cfg)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, client)
		})
	}
}

func TestNewConnectionAPIClient_ValidCredentials(t *testing.T) {
	cfg := mock.New(t)
	cfg.SetWithoutSource("api_key", "test-api-key")
	cfg.SetWithoutSource("app_key", "test-app-key")
	cfg.SetWithoutSource("site", "datadoghq.com")

	client, err := NewConnectionAPIClient(cfg)

	require.NoError(t, err)
	require.NotNil(t, client)
	assert.NotNil(t, client.httpClient)
	assert.NotEmpty(t, client.endpoint)
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
	runnerID := "runner-123"
	name := "HTTP (runner-123)"

	request := buildConnectionRequest(httpDef, runnerID, name)

	assert.Equal(t, "action_connection", request.Data.Type)
	assert.Equal(t, name, request.Data.Attributes.Name)
	assert.Equal(t, runnerID, request.Data.Attributes.RunnerID)
	assert.Equal(t, "HTTP", request.Data.Attributes.Integration.Type)
	assert.Equal(t, "HTTPNoAuth", request.Data.Attributes.Integration.Credentials["type"])
	assert.Len(t, request.Data.Attributes.Integration.Credentials, 1)
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
	runnerID := "runner-456"
	name := "Script (runner-456)"

	request := buildConnectionRequest(scriptDef, runnerID, name)

	assert.Equal(t, "action_connection", request.Data.Type)
	assert.Equal(t, name, request.Data.Attributes.Name)
	assert.Equal(t, runnerID, request.Data.Attributes.RunnerID)
	assert.Equal(t, "Script", request.Data.Attributes.Integration.Type)
	assert.Equal(t, "Script", request.Data.Attributes.Integration.Credentials["type"])
	assert.Equal(t, "/etc/dd-action-runner/config/credentials/script.yaml",
		request.Data.Attributes.Integration.Credentials["configFileLocation"])
	assert.Len(t, request.Data.Attributes.Integration.Credentials, 2)
}
