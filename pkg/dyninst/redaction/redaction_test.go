// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package redaction

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNilConfig(t *testing.T) {
	var c *Config
	require.False(t, c.RedactIdentifier("password"))
	require.False(t, c.RedactType("crypto/tls.Config"))
}

func TestDefaultIdentifiers(t *testing.T) {
	c := NewConfig(nil, nil, nil)
	for _, name := range []string{
		"password", "Password", "PASSWORD",
		"pass_word", "pass-word", "$password", "pass@word",
		"apiKey", "api_key", "API-KEY",
		"accessToken", "access_token",
		"secret", "token", "ssn", "jwt",
	} {
		require.Truef(t, c.RedactIdentifier(name), "expected %q to be redacted", name)
	}
	for _, name := range []string{
		"", "username", "id", "count", "passwordStrength", "mypassword", "tokens",
	} {
		require.Falsef(t, c.RedactIdentifier(name), "expected %q not to be redacted", name)
	}
}

func TestExtraIdentifiersAreAdditiveAndNormalized(t *testing.T) {
	c := NewConfig([]string{"MotDePasse", "$Passwort", " geheim "}, nil, nil)
	require.True(t, c.RedactIdentifier("mot_de_passe"))
	require.True(t, c.RedactIdentifier("passwort"))
	require.True(t, c.RedactIdentifier("GEHEIM"))
	// Defaults still apply.
	require.True(t, c.RedactIdentifier("password"))
}

func TestExcludedIdentifiersUnredactDefaults(t *testing.T) {
	c := NewConfig(nil, nil, []string{"token", "x-api-key"})
	require.False(t, c.RedactIdentifier("token"))
	require.False(t, c.RedactIdentifier("xApiKey"))
	// A non-excluded default is unaffected.
	require.True(t, c.RedactIdentifier("password"))
}

func TestExclusionWinsOverExtra(t *testing.T) {
	c := NewConfig([]string{"token"}, nil, []string{"token"})
	require.False(t, c.RedactIdentifier("token"))
}

func TestRedactTypeExact(t *testing.T) {
	c := NewConfig(nil, []string{"crypto/tls.Config", "main.Credentials"}, nil)
	require.True(t, c.RedactType("crypto/tls.Config"))
	require.True(t, c.RedactType("main.Credentials"))
	require.False(t, c.RedactType("main.Credential"))
	require.False(t, c.RedactType(""))
}

func TestRedactTypePrefix(t *testing.T) {
	c := NewConfig(nil, []string{"main.Secret*", "internal/auth.*"}, nil)
	require.True(t, c.RedactType("main.SecretKey"))
	require.True(t, c.RedactType("main.Secret"))
	require.True(t, c.RedactType("internal/auth.Token"))
	require.False(t, c.RedactType("main.PublicValue"))
	require.False(t, c.RedactType("internal/authz.Token"))
}

func TestEmptyEntriesIgnored(t *testing.T) {
	c := NewConfig([]string{"", "   "}, []string{"", " "}, []string{""})
	require.True(t, c.RedactIdentifier("password"))
	require.False(t, c.RedactType("anything"))
}

func TestNormalizeIdentifier(t *testing.T) {
	for in, want := range map[string]string{
		"Password":     "password",
		"access_token": "accesstoken",
		"x-api-key":    "xapikey",
		"$pass@word":   "password",
		" Spaced Out ": "spaced out",
	} {
		require.Equalf(t, want, normalizeIdentifier(in), "normalize(%q)", in)
	}
}
