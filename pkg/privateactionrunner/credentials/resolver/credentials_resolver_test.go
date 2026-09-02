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

	http "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

type fakeSecretResolver struct {
	values      map[string]string
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
	var handles []string
	if err := yaml.Unmarshal(data, &handles); err != nil {
		return nil, err
	}
	resolvedValues := make([]string, len(handles))
	for i, handle := range handles {
		resolvedValues[i] = r.values[handle]
	}
	return yaml.Marshal(resolvedValues)
}

func TestResolveConnectionInfoToCredentialConnectionTokensV2(t *testing.T) {
	connInfo := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
		TokensV2: []*privateactionspb.ConnectionTokenV2{
			newV2PlainTextToken([]string{privateconnection.RootTokenGroupName, "username"}, "database-user"),
			newV2PlainTextToken([]string{privateconnection.RootTokenGroupName, "password"}, "database-password"),
			newV2PlainTextToken([]string{http.BaseUrlTokenName}, "https://database.example.com"),
		},
	}
	want := &privateconnection.PrivateCredentials{
		Type: privateconnection.TokenAuthType,
		Tokens: []privateconnection.PrivateCredentialsToken{
			{Name: "username", Value: "database-user"},
			{Name: "password", Value: "database-password"},
		},
		HttpDetails: privateconnection.HttpDetails{
			BaseURL:       "https://database.example.com",
			Headers:       []privateconnection.PrivateCredentialsToken{},
			UrlParameters: []privateconnection.PrivateCredentialsToken{},
		},
	}

	got, err := NewPrivateCredentialResolver(nil, true).ResolveConnectionInfoToCredential(context.Background(), connInfo, nil)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestResolveConnectionInfoToCredentialConnectionTokensV2DatadogAgentSecrets(t *testing.T) {
	secretResolver := &fakeSecretResolver{values: map[string]string{
		"ENC[database_password]": "resolved-password",
		"ENC[api_key]":           "resolved-api-key",
	}}
	connInfo := &privateactionspb.ConnectionInfo{
		ConnectionId:    "connection-id",
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
		TokensV2: []*privateactionspb.ConnectionTokenV2{
			newV2DatadogAgentSecretToken([]string{privateconnection.RootTokenGroupName, "password"}, "ENC[database_password]"),
			newV2DatadogAgentSecretToken([]string{"headers", "X-API-Key"}, "ENC[api_key]"),
		},
	}

	got, err := NewPrivateCredentialResolver(secretResolver, true).ResolveConnectionInfoToCredential(context.Background(), connInfo, nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"password": "resolved-password"}, got.AsTokenMap())
	assert.Equal(t, []privateconnection.PrivateCredentialsToken{{Name: "X-API-Key", Value: "resolved-api-key"}}, got.HttpDetails.Headers)
	assert.Equal(t, "private-action-runner/connection/connection-id", secretResolver.origin)
	var encodedSecrets []string
	require.NoError(t, yaml.Unmarshal(secretResolver.encodedData, &encodedSecrets))
	assert.Equal(t, []string{"ENC[database_password]", "ENC[api_key]"}, encodedSecrets)
	assert.Equal(t, 1, secretResolver.calls)
}

func TestResolveConnectionInfoToCredentialConnectionTokensV2RejectsUnavailableSecretResolver(t *testing.T) {
	connInfo := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
		TokensV2: []*privateactionspb.ConnectionTokenV2{
			newV2DatadogAgentSecretToken([]string{privateconnection.RootTokenGroupName, "password"}, "ENC[database_password]"),
		},
	}

	got, err := NewPrivateCredentialResolver(nil, true).ResolveConnectionInfoToCredential(context.Background(), connInfo, nil)
	assert.Nil(t, got)
	assert.EqualError(t, err, "could not resolve Datadog Agent secrets: Datadog Agent secret resolver is unavailable")
}

func TestResolveConnectionInfoToCredentialConnectionTokensV2RejectsDisabledSecretManagement(t *testing.T) {
	secretResolver := &fakeSecretResolver{values: map[string]string{
		"ENC[database_password]": "resolved-password",
	}}
	connInfo := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
		TokensV2: []*privateactionspb.ConnectionTokenV2{
			newV2DatadogAgentSecretToken([]string{privateconnection.RootTokenGroupName, "password"}, "ENC[database_password]"),
		},
	}

	got, err := NewPrivateCredentialResolver(secretResolver, false).ResolveConnectionInfoToCredential(context.Background(), connInfo, nil)
	assert.Nil(t, got)
	assert.EqualError(t, err, "could not resolve Datadog Agent secrets: Datadog Agent secret management is disabled")
	assert.Zero(t, secretResolver.calls)
}

func TestResolveConnectionInfoToCredentialConnectionTokensV2RejectsUnresolvedSecret(t *testing.T) {
	connInfo := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
		TokensV2: []*privateactionspb.ConnectionTokenV2{
			newV2DatadogAgentSecretToken([]string{privateconnection.RootTokenGroupName, "password"}, "ENC[database_password]"),
		},
	}

	got, err := NewPrivateCredentialResolver(&fakeSecretResolver{passthrough: true}, true).ResolveConnectionInfoToCredential(context.Background(), connInfo, nil)
	assert.Nil(t, got)
	assert.EqualError(t, err, "could not resolve Datadog Agent secrets: secret handle \"ENC[database_password]\" was not resolved")
}

func newV2PlainTextToken(nameSegments []string, value string) *privateactionspb.ConnectionTokenV2 {
	return &privateactionspb.ConnectionTokenV2{
		NameSegments: nameSegments,
		Source: &privateactionspb.ConnectionTokenV2_PlainText_{
			PlainText: &privateactionspb.ConnectionTokenV2_PlainText{Value: value},
		},
	}
}

func newV2DatadogAgentSecretToken(nameSegments []string, handle string) *privateactionspb.ConnectionTokenV2 {
	return &privateactionspb.ConnectionTokenV2{
		NameSegments: nameSegments,
		Source: &privateactionspb.ConnectionTokenV2_DatadogAgentSecret_{
			DatadogAgentSecret: &privateactionspb.ConnectionTokenV2_DatadogAgentSecret{Handle: handle},
		},
	}
}
