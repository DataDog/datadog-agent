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

	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

func TestCredentialCatalogConfiguredValues(t *testing.T) {
	catalog := NewCredentialCatalog(map[string]string{"api_token": "secret-value"})

	value, err := catalog.Resolve("api_token")
	require.NoError(t, err)
	assert.Equal(t, "secret-value", value)
	assert.Equal(t, []CredentialDescriptor{{Key: "api_token", Source: "configured"}}, catalog.List())
	_, err = catalog.Resolve("ENC[not-operator-configured]")
	assert.EqualError(t, err, "requested runner credential is not available")
}

func TestResolveConnectionTokensV2FromRunnerCatalog(t *testing.T) {
	catalog := NewCredentialCatalog(map[string]string{"api_token": "secret-value"})
	resolver := NewPrivateCredentialResolver(catalog)
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
}
