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

	credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), conn, ActionContext{}, nil)
	require.NoError(t, err)
	require.Len(t, credentials.Tokens, 1)
	assert.Equal(t, "token", credentials.Tokens[0].Name)
	assert.Equal(t, "secret-value", credentials.Tokens[0].Value)

	// Secret refresh replaces the compound setting through configAssignAtPath.
	config.Set(par.CredentialsValues, map[string]any{"api_token": map[string]any{"value": "rotated-value"}}, model.SourceSecret)
	credentials, err = resolver.ResolveConnectionInfoToCredential(context.Background(), conn, ActionContext{}, nil)
	require.NoError(t, err)
	require.Len(t, credentials.Tokens, 1)
	assert.Equal(t, "rotated-value", credentials.Tokens[0].Value)

	// Invalid updates must fail instead of falling back to stale credentials.
	config.Set(par.CredentialsValues, map[string]any{"api_token": "invalid-secret-value"}, model.SourceSecret)
	credentials, err = resolver.ResolveConnectionInfoToCredential(context.Background(), conn, ActionContext{}, nil)
	require.Error(t, err)
	assert.Nil(t, credentials)
	assert.NotContains(t, err.Error(), "invalid-secret-value")

	config.Set(par.CredentialsValues, map[string]any{}, model.SourceSecret)
	credentials, err = resolver.ResolveConnectionInfoToCredential(context.Background(), conn, ActionContext{}, nil)
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

	credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), conn, ActionContext{}, nil)
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
		credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), conn, ActionContext{}, nil)
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
	credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), conn, ActionContext{}, nil)
	require.NoError(t, err)
	require.Len(t, credentials.Tokens, 1)
	assert.Equal(t, "resolved-value", credentials.Tokens[0].Value)

	mockConfig.Set(par.CredentialsValues, map[string]any{"api_token": map[string]any{"value": "rotated-value"}}, model.SourceSecret)
	credentials, err = resolver.ResolveConnectionInfoToCredential(context.Background(), conn, ActionContext{}, nil)
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

func runnerCredentialConnection(keys ...string) *privateactionspb.ConnectionInfo {
	conn := &privateactionspb.ConnectionInfo{CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2}
	for _, key := range keys {
		conn.TokensV2 = append(conn.TokensV2, &privateactionspb.ConnectionTokenV2{
			NameSegments: []string{"root_tokens", key},
			Source: &privateactionspb.ConnectionTokenV2_RunnerCredential_{
				RunnerCredential: &privateactionspb.ConnectionTokenV2_RunnerCredential{Key: key},
			},
		})
	}
	return conn
}

func TestRunnerCredentialAllowedActions(t *testing.T) {
	for _, test := range []struct {
		name, policy, bundle, action string
		allowed                      bool
	}{
		{name: "omitted", bundle: "com.datadoghq.http", action: "sendRequest", allowed: true},
		{name: "empty", policy: "[]", bundle: "com.datadoghq.mongodb", action: "find"},
		{name: "integration wildcard", policy: "[com.datadoghq.mongodb.*]", bundle: "com.datadoghq.mongodb", action: "find", allowed: true},
		{name: "connection test", policy: "[com.datadoghq.mongodb.*]", bundle: "com.datadoghq.mongodb", action: "testConnection", allowed: true},
		{name: "other integration", policy: "[com.datadoghq.mongodb.*]", bundle: "com.datadoghq.http", action: "sendRequest"},
		{name: "bundle boundary", policy: "[com.datadoghq.mongodb.*]", bundle: "com.datadoghq.mongodbother", action: "find"},
		{name: "sub bundle", policy: "[com.datadoghq.gitlab.*]", bundle: "com.datadoghq.gitlab.issues", action: "getIssue", allowed: true},
		{name: "sub bundle wildcard", policy: "[com.datadoghq.gitlab.issues.*]", bundle: "com.datadoghq.gitlab.issues", action: "getIssue", allowed: true},
		{name: "sibling bundle", policy: "[com.datadoghq.gitlab.issues.*]", bundle: "com.datadoghq.gitlab.users", action: "getUser"},
		{name: "exact", policy: "[com.datadoghq.mongodb.find]", bundle: "com.datadoghq.mongodb", action: "find", allowed: true},
		{name: "multiple", policy: "[com.datadoghq.mongodb.find, com.datadoghq.mongodb.countDocuments]", bundle: "com.datadoghq.mongodb", action: "countDocuments", allowed: true},
		{name: "exact denies write", policy: "[com.datadoghq.mongodb.find]", bundle: "com.datadoghq.mongodb", action: "insertOne"},
		{name: "no implicit connection test", policy: "[com.datadoghq.mongodb.find]", bundle: "com.datadoghq.mongodb", action: "testConnection"},
		{name: "case sensitive", policy: "[com.datadoghq.mongodb.find]", bundle: "com.datadoghq.mongodb", action: "Find"},
		{name: "missing context", policy: "[com.datadoghq.mongodb.*]"},
		{name: "missing action", policy: "[com.datadoghq.mongodb.*]", bundle: "com.datadoghq.mongodb"},
		{name: "wildcard action", policy: "[com.datadoghq.mongodb.*]", bundle: "com.datadoghq.mongodb", action: "*"},
		{name: "malformed bundle", policy: "[com.datadoghq.mongodb.*]", bundle: "com.datadoghq.mongodb..nested", action: "find"},
	} {
		t.Run(test.name, func(t *testing.T) {
			yaml := "private_action_runner:\n  credentials:\n    values:\n      password:\n        value: secret-value\n"
			if test.policy != "" {
				yaml += "        allowed_actions: " + test.policy + "\n"
			}
			resolver, err := NewPrivateCredentialResolver(configmock.NewFromYAML(t, yaml))
			require.NoError(t, err)
			credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), runnerCredentialConnection("password"), ActionContext{BundleID: test.bundle, ActionName: test.action}, nil)
			if test.allowed {
				require.NoError(t, err)
				require.Len(t, credentials.Tokens, 1)
				assert.Equal(t, "secret-value", credentials.Tokens[0].Value)
			} else {
				require.ErrorContains(t, err, "not allowed for action")
				assert.Contains(t, err.Error(), "password")
				assert.NotContains(t, err.Error(), "secret-value")
				assert.Nil(t, credentials)
			}
		})
	}
}

func TestRunnerCredentialInvalidPolicies(t *testing.T) {
	for _, policy := range []string{
		"null", "~", "", "com.datadoghq.mongodb.find", "42", "{}", "[null]", "[true]", "[42]",
		"['']", "['*']", "[com.datadoghq.*]", "[com.datadoghq.mongodb.find*]",
		"[com.datadoghq.mongodb.*.find]", "[com.datadoghq..find]", "[' com.datadoghq.mongodb.find']",
	} {
		t.Run(policy, func(t *testing.T) {
			config := configmock.NewFromYAML(t, "private_action_runner:\n  credentials:\n    values:\n      password:\n        value: secret-value\n        allowed_actions: "+policy+"\n")
			resolver, err := NewPrivateCredentialResolver(config)
			require.Error(t, err)
			assert.Nil(t, resolver)
			assert.NotContains(t, err.Error(), "secret-value")
		})
	}
}

func TestRunnerCredentialPolicyRefreshAndAllOrNothing(t *testing.T) {
	config := configmock.New(t)
	setPolicy := func(policy any) {
		config.Set(par.CredentialsValues, map[string]any{
			"username": map[string]any{"value": "user-value"},
			"password": map[string]any{"value": "secret-value", "allowed_actions": policy},
		}, model.SourceSecret)
	}
	setPolicy([]string{"com.datadoghq.mongodb.*"})
	resolver, err := NewPrivateCredentialResolver(config)
	require.NoError(t, err)
	conn := runnerCredentialConnection("username", "password")
	action := ActionContext{BundleID: "com.datadoghq.mongodb", ActionName: "find"}
	credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), conn, action, nil)
	require.NoError(t, err)
	require.Len(t, credentials.Tokens, 2)

	for _, policy := range []any{[]string{}, []string{"com.datadoghq.http.*"}, nil, "invalid-secret-policy"} {
		setPolicy(policy)
		credentials, err = resolver.ResolveConnectionInfoToCredential(context.Background(), conn, action, nil)
		require.Error(t, err)
		assert.Nil(t, credentials) // Even the unrestricted username must not escape.
		assert.NotContains(t, err.Error(), "secret-value")
		assert.NotContains(t, err.Error(), "invalid-secret-policy")
	}
}

func TestRunnerCredentialHTTPDomains(t *testing.T) {
	for _, test := range []struct {
		name, domains, bundle, action, target, testPath string
		allowed                                         bool
	}{
		{name: "allowed", domains: "[api.example.com]", bundle: "com.datadoghq.http", action: "request", target: "https://api.example.com/path", allowed: true},
		{name: "other domain", domains: "[api.example.com]", bundle: "com.datadoghq.http", action: "request", target: "https://other.example.com"},
		{name: "wildcard", domains: "['*.example.com']", bundle: "com.datadoghq.http", action: "request", target: "https://api.example.com", allowed: true},
		{name: "empty denies", domains: "[]", bundle: "com.datadoghq.http", action: "request", target: "https://api.example.com"},
		{name: "non http denied", domains: "[api.example.com]", bundle: "com.datadoghq.mongodb", action: "find", target: "https://api.example.com"},
		{name: "unknown http action", domains: "[api.example.com]", bundle: "com.datadoghq.http", action: "unknown", target: "https://api.example.com"},
		{name: "missing url", domains: "[api.example.com]", bundle: "com.datadoghq.http", action: "request"},
		{name: "invalid url", domains: "[api.example.com]", bundle: "com.datadoghq.http", action: "request", target: "https://api.example.com/%invalid-secret"},
		{name: "checksum", domains: "[api.example.com]", bundle: "com.datadoghq.http", action: "getChecksumFromURL", target: "https://api.example.com", allowed: true},
		{name: "test connection", domains: "[api.example.com]", bundle: "com.datadoghq.http", action: "testConnection", target: "https://ignored.example.net", testPath: "/health", allowed: true},
		{name: "absolute test path denied", domains: "[api.example.com]", bundle: "com.datadoghq.http", action: "testConnection", testPath: "https://evil.test/secret-value"},
		{name: "scheme relative test path denied", domains: "[api.example.com]", bundle: "com.datadoghq.http", action: "testConnection", testPath: "//evil.test/secret-value"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := configmock.NewFromYAML(t, "private_action_runner:\n  credentials:\n    values:\n      password:\n        value: secret-value\n        allowed_http_domains: "+test.domains+"\n")
			resolver, err := NewPrivateCredentialResolver(config)
			require.NoError(t, err)
			conn := runnerCredentialConnection("password")
			if test.testPath != "" {
				for _, token := range []struct {
					segments []string
					value    string
				}{
					{[]string{"base_url"}, "https://api.example.com"},
					{[]string{"testing", "path"}, test.testPath},
				} {
					conn.TokensV2 = append(conn.TokensV2, &privateactionspb.ConnectionTokenV2{
						NameSegments: token.segments,
						Source:       &privateactionspb.ConnectionTokenV2_PlainText_{PlainText: &privateactionspb.ConnectionTokenV2_PlainText{Value: token.value}},
					})
				}
			}
			credentials, err := resolver.ResolveConnectionInfoToCredential(context.Background(), conn, ActionContext{BundleID: test.bundle, ActionName: test.action, HTTPRequestURL: test.target}, nil)
			if test.allowed {
				require.NoError(t, err)
				require.Len(t, credentials.HTTPDomainRestrictions, 1)
				require.Len(t, credentials.Tokens, 1)
				assert.Equal(t, "secret-value", credentials.Tokens[0].Value)
			} else {
				require.Error(t, err)
				assert.Nil(t, credentials)
				assert.NotContains(t, err.Error(), "secret-value")
				assert.NotContains(t, err.Error(), "invalid-secret")
			}
		})
	}
}

func TestRunnerCredentialInvalidDomainPolicies(t *testing.T) {
	for _, policy := range []string{"null", "api.example.com", "{}", "[null]", "[42]", "['https://api.example.com']", "['api.example.com:443']", "['*']"} {
		t.Run(policy, func(t *testing.T) {
			config := configmock.NewFromYAML(t, "private_action_runner:\n  credentials:\n    values:\n      password:\n        value: secret-value\n        allowed_http_domains: "+policy+"\n")
			resolver, err := NewPrivateCredentialResolver(config)
			require.Error(t, err)
			assert.Nil(t, resolver)
			assert.NotContains(t, err.Error(), "secret-value")
		})
	}
}
