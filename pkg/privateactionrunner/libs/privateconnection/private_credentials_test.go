// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package privateconnection

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAsTokenMap_ConvertsSliceToMap verifies that all tokens in the slice are accessible by
// name after the conversion.
func TestAsTokenMap_ConvertsSliceToMap(t *testing.T) {
	creds := PrivateCredentials{
		Tokens: []PrivateCredentialsToken{
			{Name: "api-key", Value: "secret-1"},
			{Name: "region", Value: "us-east-1"},
		},
	}

	m := creds.AsTokenMap()

	assert.Equal(t, "secret-1", m["api-key"])
	assert.Equal(t, "us-east-1", m["region"])
	assert.Len(t, m, 2)
}

// TestAsTokenMap_EmptyTokens verifies that an empty token slice produces an empty map rather
// than nil (so callers can safely do map lookups without a nil check).
func TestAsTokenMap_EmptyTokens(t *testing.T) {
	creds := PrivateCredentials{}
	m := creds.AsTokenMap()
	assert.NotNil(t, m)
	assert.Empty(t, m)
}

// TestAsTokenMap_DuplicateNamesLastWins verifies the behaviour when two tokens share the
// same name: the last entry in the slice overwrites the earlier one.
func TestAsTokenMap_DuplicateNamesLastWins(t *testing.T) {
	creds := PrivateCredentials{
		Tokens: []PrivateCredentialsToken{
			{Name: "token", Value: "first"},
			{Name: "token", Value: "second"},
		},
	}
	m := creds.AsTokenMap()
	assert.Equal(t, "second", m["token"])
}

// TestGetUsernamePasswordBasicAuth_Valid verifies that username and password are correctly
// extracted from a basic-auth credential.
func TestGetUsernamePasswordBasicAuth_Valid(t *testing.T) {
	creds := PrivateCredentials{
		Type: BasicAuthType,
		Tokens: []PrivateCredentialsToken{
			{Name: UsernameTokenName, Value: "admin"},
			{Name: PasswordTokenName, Value: "hunter2"},
		},
	}

	username, password, err := creds.GetUsernamePasswordBasicAuth()

	require.NoError(t, err)
	assert.Equal(t, "admin", username)
	assert.Equal(t, "hunter2", password)
}

// TestGetUsernamePasswordBasicAuth_WrongAuthType verifies that calling
// GetUsernamePasswordBasicAuth on a non-basic-auth credential returns an error. This
// guards against callers accidentally treating token-auth credentials as basic auth.
func TestGetUsernamePasswordBasicAuth_WrongAuthType(t *testing.T) {
	creds := PrivateCredentials{
		Type: TokenAuthType,
		Tokens: []PrivateCredentialsToken{
			{Name: "api-key", Value: "secret"},
		},
	}

	_, _, err := creds.GetUsernamePasswordBasicAuth()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a basic auth")
}

// TestGetUsernamePasswordBasicAuth_MissingTokensReturnsEmpty verifies that when the
// username/password tokens are absent, the function returns empty strings (not a panic or
// a fatal error), as the caller is responsible for validating the returned values.
func TestGetUsernamePasswordBasicAuth_MissingTokensReturnsEmpty(t *testing.T) {
	creds := PrivateCredentials{
		Type:   BasicAuthType,
		Tokens: []PrivateCredentialsToken{},
	}

	username, password, err := creds.GetUsernamePasswordBasicAuth()

	require.NoError(t, err)
	assert.Empty(t, username)
	assert.Empty(t, password)
}
