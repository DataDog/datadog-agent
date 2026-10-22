// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package resolver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

func TestResolveConnectionTokensV2FromConfig(t *testing.T) {
	config := configmock.New(t)
	config.Set(par.CredentialsValues, map[string]any{"api_token": map[string]any{"value": "secret-value"}}, model.SourceFile)
	resolver, err := NewPrivateCredentialResolver(config)
	require.NoError(t, err)
	conn := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
		TokensV2: []*privateactionspb.ConnectionTokenV2{{
			NameSegments: []string{"root_tokens", "token"},
			Source: &privateactionspb.ConnectionTokenV2_RunnerCredential_{
				RunnerCredential: &privateactionspb.ConnectionTokenV2_RunnerCredential{Key: "api_token"},
			},
		}},
	}

	credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	require.NoError(t, err)
	require.Len(t, credentials.Tokens, 1)
	assert.Equal(t, "token", credentials.Tokens[0].Name)
	assert.Equal(t, "secret-value", credentials.Tokens[0].Value)

	// Secret refresh replaces the compound setting through configAssignAtPath.
	config.Set(par.CredentialsValues, map[string]any{"api_token": map[string]any{"value": "rotated-value"}}, model.SourceSecret)
	credentials, err = resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	require.NoError(t, err)
	require.Len(t, credentials.Tokens, 1)
	assert.Equal(t, "rotated-value", credentials.Tokens[0].Value)

	// Invalid updates must fail instead of falling back to stale credentials.
	config.Set(par.CredentialsValues, map[string]any{"api_token": "invalid-secret-value"}, model.SourceSecret)
	credentials, err = resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	require.Error(t, err)
	assert.Nil(t, credentials)
	assert.NotContains(t, err.Error(), "invalid-secret-value")

	config.Set(par.CredentialsValues, map[string]any{}, model.SourceSecret)
	credentials, err = resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	require.ErrorContains(t, err, "requested runner credential is not available")
	assert.Nil(t, credentials)
}

func TestResolveConnectionTokensV2FromPlainText(t *testing.T) {
	resolver, err := NewPrivateCredentialResolver(nil)
	require.NoError(t, err)
	conn := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
		TokensV2: []*privateactionspb.ConnectionTokenV2{{
			NameSegments: []string{"root_tokens", "token"},
			Source: &privateactionspb.ConnectionTokenV2_PlainText_{
				PlainText: &privateactionspb.ConnectionTokenV2_PlainText{Value: "plain-value"},
			},
		}},
	}

	credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	require.NoError(t, err)
	require.Len(t, credentials.Tokens, 1)
	assert.Equal(t, "token", credentials.Tokens[0].Name)
	assert.Equal(t, "plain-value", credentials.Tokens[0].Value)
}

// refreshOnReadConfig simulates a secret refresh immediately after a config read.
type refreshOnReadConfig struct {
	model.Reader
	refresh func()
}

func (c *refreshOnReadConfig) Get(key string) any {
	value := c.Reader.Get(key)
	if key == par.CredentialsValues && c.refresh != nil {
		c.refresh()
		c.refresh = nil
	}
	return value
}

func TestResolveConnectionTokensV2UsesOneSnapshot(t *testing.T) {
	config := configmock.New(t)
	config.Set(par.CredentialsValues, map[string]any{"api_token": map[string]any{"value": "original"}}, model.SourceFile)
	liveConfig := &refreshOnReadConfig{Reader: config}
	resolver, err := NewPrivateCredentialResolver(liveConfig)
	require.NoError(t, err)
	conn := &privateactionspb.ConnectionInfo{CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2}
	for _, name := range []string{"first", "second"} {
		conn.TokensV2 = append(conn.TokensV2, &privateactionspb.ConnectionTokenV2{
			NameSegments: []string{"root_tokens", name},
			Source: &privateactionspb.ConnectionTokenV2_RunnerCredential_{
				RunnerCredential: &privateactionspb.ConnectionTokenV2_RunnerCredential{Key: "api_token"},
			},
		})
	}
	liveConfig.refresh = func() {
		config.Set(par.CredentialsValues, map[string]any{"api_token": map[string]any{"value": "rotated"}}, model.SourceSecret)
	}

	for _, expected := range []string{"original", "rotated"} {
		credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
		require.NoError(t, err)
		require.Len(t, credentials.Tokens, 2)
		for _, token := range credentials.Tokens {
			assert.Equal(t, expected, token.Value)
		}
	}
}

func TestNewPrivateCredentialResolverFromYAML(t *testing.T) {
	yaml := `
private_action_runner:
  credentials:
    values:
      api_token:
        value: resolved-value
`
	mockConfig := configmock.NewFromYAML(t, yaml)

	resolver, err := NewPrivateCredentialResolver(mockConfig)
	require.NoError(t, err)
	conn := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
		TokensV2: []*privateactionspb.ConnectionTokenV2{{
			NameSegments: []string{"root_tokens", "token"},
			Source: &privateactionspb.ConnectionTokenV2_RunnerCredential_{
				RunnerCredential: &privateactionspb.ConnectionTokenV2_RunnerCredential{Key: "api_token"},
			},
		}},
	}
	credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	require.NoError(t, err)
	require.Len(t, credentials.Tokens, 1)
	assert.Equal(t, "resolved-value", credentials.Tokens[0].Value)

	mockConfig.Set(par.CredentialsValues, map[string]any{"api_token": map[string]any{"value": "rotated-value"}}, model.SourceSecret)
	credentials, err = resolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	require.NoError(t, err)
	require.Len(t, credentials.Tokens, 1)
	assert.Equal(t, "rotated-value", credentials.Tokens[0].Value)
}

func TestNewPrivateCredentialResolverInvalidCredentials(t *testing.T) {
	for _, test := range []struct {
		name string
		yaml string
	}{
		{
			name: "scalar credential",
			yaml: `private_action_runner:
  credentials:
    values:
      api_token: secret-value
`,
		},
		{
			name: "unsupported restriction",
			yaml: `private_action_runner:
  credentials:
    values:
      api_token:
        value: secret-value
        allowed_actions: [some-action]
`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolver, err := NewPrivateCredentialResolver(configmock.NewFromYAML(t, test.yaml))
			require.Error(t, err)
			assert.Nil(t, resolver)
			assert.Contains(t, err.Error(), par.CredentialsValues)
			assert.NotContains(t, err.Error(), "secret-value")
		})
	}
}
