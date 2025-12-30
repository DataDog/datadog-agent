// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package resolver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

func TestResolveAgentConfigurationCredentials(t *testing.T) {
	// Setup config with 2 script credentials
	cfg := &config.Config{
		Credentials: map[string]interface{}{
			"scripts": map[string]interface{}{
				"schemaId": "script-credentials-v1",
				"runPredefinedScript": map[string]interface{}{
					"echo": map[string]interface{}{
						"command": []interface{}{"echo", "Hello world"},
					},
					"echoParametrized": map[string]interface{}{
						"command": []interface{}{"echo", "{{ parameters.Name }}"},
					},
				},
			},
			"other-scripts": map[string]interface{}{
				"schemaId": "script-credentials-v1",
				"runPredefinedScript": map[string]interface{}{
					"bye": map[string]interface{}{
						"command": []interface{}{"echo", "Good bye"},
					},
				},
			},
		},
	}

	resolver := NewPrivateCredentialResolver(cfg)
	ctx := context.Background()

	// Test getting the first script credential
	connInfo1 := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_AGENT_CONFIGURATION,
		Tokens: []*privateactionspb.ConnectionToken{
			{
				NameSegments: []string{"root_tokens", "credential-name"},
				TokenValue: &privateactionspb.ConnectionToken_PlainText_{
					PlainText: &privateactionspb.ConnectionToken_PlainText{
						Value: "scripts",
					},
				},
			},
		},
	}

	creds1, err := resolver.ResolveConnectionInfoToCredential(ctx, connInfo1, nil)
	require.NoError(t, err)
	assert.Equal(t, privateconnection.ConfigAuthType, creds1.Type)

	// Verify we got the right credential
	credMap1, ok := creds1.ConfigData.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "script-credentials-v1", credMap1["schemaId"])

	runPredefinedScript1, ok := credMap1["runPredefinedScript"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, runPredefinedScript1, "echo")
	assert.Contains(t, runPredefinedScript1, "echoParametrized")
	assert.NotContains(t, runPredefinedScript1, "bye")

	// Test getting the second script credential
	connInfo2 := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_AGENT_CONFIGURATION,
		Tokens: []*privateactionspb.ConnectionToken{
			{
				NameSegments: []string{"root_tokens", "credential-name"},
				TokenValue: &privateactionspb.ConnectionToken_PlainText_{
					PlainText: &privateactionspb.ConnectionToken_PlainText{
						Value: "other-scripts",
					},
				},
			},
		},
	}

	creds2, err := resolver.ResolveConnectionInfoToCredential(ctx, connInfo2, nil)
	require.NoError(t, err)
	assert.Equal(t, privateconnection.ConfigAuthType, creds2.Type)

	// Verify we got the right credential
	credMap2, ok := creds2.ConfigData.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "script-credentials-v1", credMap2["schemaId"])

	runPredefinedScript2, ok := credMap2["runPredefinedScript"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, runPredefinedScript2, "bye")
	assert.NotContains(t, runPredefinedScript2, "echo")
	assert.NotContains(t, runPredefinedScript2, "echoParametrized")
}
