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
	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

type fakeSecretResolver struct {
	value       string
	origin      string
	encodedData []byte
	passthrough bool
	calls       int
}

func (r *fakeSecretResolver) Resolve(data []byte, origin string, _ string, _ string, _ bool) ([]byte, error) {
	r.calls++
	r.origin = origin
	r.encodedData = data
	if r.passthrough {
		return data, nil
	}
	var encodedValues []string
	if err := yaml.Unmarshal(data, &encodedValues); err != nil {
		return nil, err
	}
	resolvedValues := make([]string, len(encodedValues))
	for i := range resolvedValues {
		resolvedValues[i] = r.value
	}
	return yaml.Marshal(resolvedValues)
}

func TestResolveConnectionTokens(t *testing.T) {
	t.Setenv("PAR_TEST_TOKEN", "from-env")
	secretResolver := &fakeSecretResolver{value: "from-secret"}
	resolver := NewPrivateCredentialResolver(secretResolver)

	credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), &privateactionspb.ConnectionInfo{
		ConnectionId:    "connection-id",
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS,
		ConnectionTokenCredentials: &privateactionspb.ConnectionTokenCredentials{Tokens: []*privateactionspb.ConnectionTokenCredentials_Token{
			{
				NameSegments: []string{privateconnection.RootTokenGroupName, "plain"},
				TokenValue: &privateactionspb.ConnectionTokenCredentials_Token_PlainText_{
					PlainText: &privateactionspb.ConnectionTokenCredentials_Token_PlainText{Value: "from-plain-text"},
				},
			},
			{
				NameSegments: []string{"headers", "Authorization"},
				TokenValue: &privateactionspb.ConnectionTokenCredentials_Token_EnvironmentVariable_{
					EnvironmentVariable: &privateactionspb.ConnectionTokenCredentials_Token_EnvironmentVariable{Name: "PAR_TEST_TOKEN"},
				},
			},
			{
				NameSegments: []string{privateconnection.RootTokenGroupName, "secret"},
				TokenValue: &privateactionspb.ConnectionTokenCredentials_Token_DatadogAgentSecret_{
					DatadogAgentSecret: &privateactionspb.ConnectionTokenCredentials_Token_DatadogAgentSecret{Handle: "secret-handle"},
				},
			},
			{
				NameSegments: []string{privateconnection.RootTokenGroupName, "second-secret"},
				TokenValue: &privateactionspb.ConnectionTokenCredentials_Token_DatadogAgentSecret_{
					DatadogAgentSecret: &privateactionspb.ConnectionTokenCredentials_Token_DatadogAgentSecret{Handle: "second-secret-handle"},
				},
			},
		}},
	}, nil)

	require.NoError(t, err)
	assert.Equal(t, privateconnection.TokenAuthType, credentials.Type)
	assert.Equal(t, map[string]string{
		"plain":         "from-plain-text",
		"secret":        "from-secret",
		"second-secret": "from-secret",
	}, credentials.AsTokenMap())
	assert.Equal(t, []privateconnection.PrivateCredentialsToken{{Name: "Authorization", Value: "from-env"}}, credentials.HttpDetails.Headers)
	assert.Equal(t, "private-action-runner/connection/connection-id", secretResolver.origin)
	var encodedSecrets []string
	require.NoError(t, yaml.Unmarshal(secretResolver.encodedData, &encodedSecrets))
	assert.Equal(t, []string{"ENC[secret-handle]", "ENC[second-secret-handle]"}, encodedSecrets)
	assert.Equal(t, 1, secretResolver.calls)
}

func TestResolveConnectionTokensRejectsUnavailableSources(t *testing.T) {
	t.Run("environment variable", func(t *testing.T) {
		resolver := NewPrivateCredentialResolver(nil)
		_, err := resolver.ResolveConnectionInfoToCredential(context.Background(), connectionInfoWithToken(
			&privateactionspb.ConnectionTokenCredentials_Token{
				NameSegments: []string{privateconnection.RootTokenGroupName, "token"},
				TokenValue: &privateactionspb.ConnectionTokenCredentials_Token_EnvironmentVariable_{
					EnvironmentVariable: &privateactionspb.ConnectionTokenCredentials_Token_EnvironmentVariable{Name: "PAR_TEST_UNSET_TOKEN"},
				},
			},
		), nil)
		require.ErrorContains(t, err, "is not set")
	})

	t.Run("Agent secret resolver", func(t *testing.T) {
		resolver := NewPrivateCredentialResolver(nil)
		_, err := resolver.ResolveConnectionInfoToCredential(context.Background(), connectionInfoWithToken(
			&privateactionspb.ConnectionTokenCredentials_Token{
				NameSegments: []string{privateconnection.RootTokenGroupName, "token"},
				TokenValue: &privateactionspb.ConnectionTokenCredentials_Token_DatadogAgentSecret_{
					DatadogAgentSecret: &privateactionspb.ConnectionTokenCredentials_Token_DatadogAgentSecret{Handle: "secret-handle"},
				},
			},
		), nil)
		require.ErrorContains(t, err, "Agent secret resolver is unavailable")
	})

	t.Run("unresolved Agent secret", func(t *testing.T) {
		resolver := NewPrivateCredentialResolver(&fakeSecretResolver{passthrough: true})
		_, err := resolver.ResolveConnectionInfoToCredential(context.Background(), connectionInfoWithToken(
			&privateactionspb.ConnectionTokenCredentials_Token{
				NameSegments: []string{privateconnection.RootTokenGroupName, "token"},
				TokenValue: &privateactionspb.ConnectionTokenCredentials_Token_DatadogAgentSecret_{
					DatadogAgentSecret: &privateactionspb.ConnectionTokenCredentials_Token_DatadogAgentSecret{Handle: "secret-handle"},
				},
			},
		), nil)
		require.ErrorContains(t, err, "was not resolved")
	})
}

func TestLegacyCredentialsIgnoreConnectionTokenPayload(t *testing.T) {
	resolver := NewPrivateCredentialResolver(nil)
	credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_TOKEN_AUTH,
		Tokens: []*privateactionspb.ConnectionToken{
			privateconnection.NewPlainTextToken([]string{privateconnection.RootTokenGroupName, "legacy"}, "legacy-value"),
		},
		ConnectionTokenCredentials: &privateactionspb.ConnectionTokenCredentials{Tokens: []*privateactionspb.ConnectionTokenCredentials_Token{
			{
				NameSegments: []string{privateconnection.RootTokenGroupName, "new"},
				TokenValue: &privateactionspb.ConnectionTokenCredentials_Token_PlainText_{
					PlainText: &privateactionspb.ConnectionTokenCredentials_Token_PlainText{Value: "new-value"},
				},
			},
		}},
	}, nil)

	require.NoError(t, err)
	assert.Equal(t, map[string]string{"legacy": "legacy-value"}, credentials.AsTokenMap())
}

func connectionInfoWithToken(token *privateactionspb.ConnectionTokenCredentials_Token) *privateactionspb.ConnectionInfo {
	return &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS,
		ConnectionTokenCredentials: &privateactionspb.ConnectionTokenCredentials{Tokens: []*privateactionspb.ConnectionTokenCredentials_Token{
			token,
		}},
	}
}
