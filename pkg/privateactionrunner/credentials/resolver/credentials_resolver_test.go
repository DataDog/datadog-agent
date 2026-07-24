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

	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

func plainTextToken(name, value string) *privateactionspb.ConnectionToken {
	return &privateactionspb.ConnectionToken{
		NameSegments: []string{privateconnection.RootTokenGroupName, name},
		TokenValue: &privateactionspb.ConnectionToken_PlainText_{
			PlainText: &privateactionspb.ConnectionToken_PlainText{Value: value},
		},
	}
}

func tokenValues(creds *privateconnection.PrivateCredentials) map[string]string {
	out := make(map[string]string, len(creds.Tokens))
	for _, tok := range creds.Tokens {
		out[tok.Name] = tok.Value
	}
	return out
}

func TestResolveTokenAuthPlainTextSecretHandle(t *testing.T) {
	secretResolver := secretsmock.New(t)
	// The handle for "ENC[aws_secrets;My-Secrets;password]" is the content inside ENC[...].
	secretResolver.SetSecrets(map[string]string{
		"aws_secrets;My-Secrets;password": "s3cr3t",
	})

	r := NewPrivateCredentialResolver(secretResolver)
	connInfo := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_TOKEN_AUTH,
		Tokens: []*privateactionspb.ConnectionToken{
			plainTextToken("token", "ENC[aws_secrets;My-Secrets;password]"),
		},
	}

	creds, err := r.ResolveConnectionInfoToCredential(context.Background(), connInfo, nil)
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t", tokenValues(creds)["token"])
}

func TestResolveTokenAuthPlainTextPassthrough(t *testing.T) {
	secretResolver := secretsmock.New(t)

	r := NewPrivateCredentialResolver(secretResolver)
	connInfo := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_TOKEN_AUTH,
		Tokens: []*privateactionspb.ConnectionToken{
			plainTextToken("token", "plain-value"),
		},
	}

	creds, err := r.ResolveConnectionInfoToCredential(context.Background(), connInfo, nil)
	require.NoError(t, err)
	assert.Equal(t, "plain-value", tokenValues(creds)["token"])
}

func TestResolveTokenAuthNilResolverLeavesHandleUntouched(t *testing.T) {
	r := NewPrivateCredentialResolver(nil)
	connInfo := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_TOKEN_AUTH,
		Tokens: []*privateactionspb.ConnectionToken{
			plainTextToken("token", "ENC[aws_secrets;My-Secrets;password]"),
		},
	}

	creds, err := r.ResolveConnectionInfoToCredential(context.Background(), connInfo, nil)
	require.NoError(t, err)
	assert.Equal(t, "ENC[aws_secrets;My-Secrets;password]", tokenValues(creds)["token"])
}

func TestResolveBasicAuthPlainTextPasswordSecretHandle(t *testing.T) {
	secretResolver := secretsmock.New(t)
	secretResolver.SetSecrets(map[string]string{
		"aws_secrets;My-Secrets;password": "s3cr3t",
	})

	r := NewPrivateCredentialResolver(secretResolver)
	connInfo := &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_BASIC_AUTH,
		Tokens: []*privateactionspb.ConnectionToken{
			plainTextToken(privateconnection.UsernameTokenName, "admin"),
			plainTextToken(privateconnection.PasswordTokenName, "ENC[aws_secrets;My-Secrets;password]"),
		},
	}

	creds, err := r.ResolveConnectionInfoToCredential(context.Background(), connInfo, nil)
	require.NoError(t, err)
	values := tokenValues(creds)
	assert.Equal(t, "admin", values[privateconnection.UsernameTokenName])
	assert.Equal(t, "s3cr3t", values[privateconnection.PasswordTokenName])
}
