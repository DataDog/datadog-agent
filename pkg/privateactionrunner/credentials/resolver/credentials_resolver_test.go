// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package resolver

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	http "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

func TestResolveConnectionInfoToCredentialConnectionTokensV2(t *testing.T) {
	t.Setenv("PAR_DATABASE_PASSWORD", "resolved-password")
	t.Setenv("PAR_DATABASE_URL", "https://database.example.com")
	connInfo := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
		TokensV2: []*privateactionspb.ConnectionTokenV2{
			newV2PlainTextToken([]string{privateconnection.RootTokenGroupName, "username"}, "database-user"),
			newV2EnvironmentVariableToken([]string{privateconnection.RootTokenGroupName, "password"}, "PAR_DATABASE_PASSWORD"),
			newV2EnvironmentVariableToken([]string{http.BaseUrlTokenName}, "PAR_DATABASE_URL"),
		},
	}
	want := &privateconnection.PrivateCredentials{
		Type: privateconnection.TokenAuthType,
		Tokens: []privateconnection.PrivateCredentialsToken{
			{Name: "username", Value: "database-user"},
			{Name: "password", Value: "resolved-password"},
		},
		HttpDetails: privateconnection.HttpDetails{
			BaseURL:       "https://database.example.com",
			Headers:       []privateconnection.PrivateCredentialsToken{},
			UrlParameters: []privateconnection.PrivateCredentialsToken{},
		},
	}

	got, err := NewPrivateCredentialResolver().ResolveConnectionInfoToCredential(context.Background(), connInfo, nil)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestResolveConnectionInfoToCredentialConnectionTokensV2MissingEnvironmentVariable(t *testing.T) {
	name := "PAR_ENVIRONMENT_VARIABLE_THAT_IS_NOT_SET"
	require.NoError(t, os.Unsetenv(name))
	connInfo := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
		TokensV2: []*privateactionspb.ConnectionTokenV2{
			newV2EnvironmentVariableToken([]string{privateconnection.RootTokenGroupName, "password"}, name),
		},
	}

	got, err := NewPrivateCredentialResolver().ResolveConnectionInfoToCredential(context.Background(), connInfo, nil)
	assert.Nil(t, got)
	assert.EqualError(t, err, "environment variable \"PAR_ENVIRONMENT_VARIABLE_THAT_IS_NOT_SET\" for connection token \"password\" is not set")
}

func TestResolveConnectionInfoToCredentialConnectionTokensV2RejectsDatadogAgentSecret(t *testing.T) {
	connInfo := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
		TokensV2: []*privateactionspb.ConnectionTokenV2{
			newV2DatadogAgentSecretToken([]string{privateconnection.RootTokenGroupName, "password"}, "ENC[database_password]"),
		},
	}

	got, err := NewPrivateCredentialResolver().ResolveConnectionInfoToCredential(context.Background(), connInfo, nil)
	assert.Nil(t, got)
	assert.EqualError(t, err, "Datadog Agent secret references are not supported by the private action runner for connection token \"password\"")
}

func newV2PlainTextToken(nameSegments []string, value string) *privateactionspb.ConnectionTokenV2 {
	return &privateactionspb.ConnectionTokenV2{
		NameSegments: nameSegments,
		Source: &privateactionspb.ConnectionTokenV2_PlainText_{
			PlainText: &privateactionspb.ConnectionTokenV2_PlainText{Value: value},
		},
	}
}

func newV2EnvironmentVariableToken(nameSegments []string, name string) *privateactionspb.ConnectionTokenV2 {
	return &privateactionspb.ConnectionTokenV2{
		NameSegments: nameSegments,
		Source: &privateactionspb.ConnectionTokenV2_EnvironmentVariable_{
			EnvironmentVariable: &privateactionspb.ConnectionTokenV2_EnvironmentVariable{Name: name},
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
