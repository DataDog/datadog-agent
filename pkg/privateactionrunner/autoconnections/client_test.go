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

func TestNewConnectionAPIClient_ValidCredentials(t *testing.T) {
	cfg := mock.New(t)
	apiKey := "test-api-key"
	appKey := "test-app-key"
	ddSite := "datadoghq.com"

	client, err := NewConnectionAPIClient(cfg, ddSite, apiKey, appKey)

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
