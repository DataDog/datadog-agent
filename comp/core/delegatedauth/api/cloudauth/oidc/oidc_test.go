// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package oidc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
)

const testOrgUUID = "11111111-2222-3333-4444-555555555555"

func makeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	encode := func(v any) string {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	return encode(map[string]string{"alg": "RS256", "typ": "JWT"}) + "." + encode(claims) + ".c2lnbmF0dXJl"
}

func scopedClaims() map[string]any {
	return map[string]any{
		"iss": "https://issuer.example",
		"sub": "system:serviceaccount:datadog:datadog-agent",
		"aud": "datadog/" + testOrgUUID,
		"exp": time.Now().Add(time.Hour).Unix(),
	}
}

func writeToken(t *testing.T, path, token string) {
	t.Helper()
	tmp := path + ".tmp"
	require.NoError(t, os.WriteFile(tmp, []byte(token), 0o600))
	require.NoError(t, os.Rename(tmp, path))
}

func newTestAuth(t *testing.T, token string) (*OIDCAuth, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	writeToken(t, path, token)
	auth, err := NewOIDCAuth(&cloudauthconfig.OIDCProviderConfig{TokenFile: path})
	require.NoError(t, err)
	return auth, path
}

func TestNewOIDCAuthValidatesConfig(t *testing.T) {
	_, err := NewOIDCAuth(nil)
	assert.ErrorContains(t, err, "token_file is required")

	_, err = NewOIDCAuth(&cloudauthconfig.OIDCProviderConfig{})
	assert.ErrorContains(t, err, "token_file is required")

	_, err = NewOIDCAuth(&cloudauthconfig.OIDCProviderConfig{TokenFile: "relative/token"})
	assert.ErrorContains(t, err, "absolute path")
}

func TestGenerateAuthProof(t *testing.T) {
	token := makeJWT(t, scopedClaims())
	auth, _ := newTestAuth(t, token)

	proof, err := auth.GenerateAuthProof(context.Background(), nil, &common.AuthConfig{OrgUUID: testOrgUUID})
	require.NoError(t, err)
	assert.Equal(t, "oidc-"+token, proof, "proof must not carry an :<org_uuid> suffix")
}

func TestGenerateAuthProofRereadsRotatedFile(t *testing.T) {
	first := makeJWT(t, scopedClaims())
	auth, path := newTestAuth(t, first)
	authConfig := &common.AuthConfig{OrgUUID: testOrgUUID}

	proof, err := auth.GenerateAuthProof(context.Background(), nil, authConfig)
	require.NoError(t, err)
	assert.Equal(t, "oidc-"+first, proof)

	claims := scopedClaims()
	claims["jti"] = "rotated"
	second := makeJWT(t, claims)
	writeToken(t, path, second)

	proof, err = auth.GenerateAuthProof(context.Background(), nil, authConfig)
	require.NoError(t, err)
	assert.Equal(t, "oidc-"+second, proof)
}

// Every reason is pinned so a change to the ported check shows up here.
func TestProofRefusals(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	encode := func(v any) string {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	jwt := func(payload string) string {
		return encode(map[string]string{"alg": "RS256"}) + "." + payload + ".c2lnbmF0dXJl"
	}
	claims := func(mutate func(map[string]any)) string {
		c := map[string]any{"aud": "datadog/" + testOrgUUID, "exp": now.Add(time.Hour).Unix()}
		if mutate != nil {
			mutate(c)
		}
		return jwt(encode(c))
	}
	scope := func(reason string) string { return "workload token is not scoped to Datadog: " + reason }
	valid := claims(nil)

	tests := map[string]struct {
		content string
		wantErr string // empty means the proof is "oidc-" + the trimmed content
	}{
		"scoped":                                {valid, ""},
		"scoped with newline":                   {valid + "\n", ""},
		"matching dd_org_uuid":                  {claims(func(c map[string]any) { c["dd_org_uuid"] = testOrgUUID }), ""},
		"audience list of one":                  {claims(func(c map[string]any) { c["aud"] = []string{"datadog/" + testOrgUUID} }), ""},
		"wrong org audience":                    {claims(func(c map[string]any) { c["aud"] = "datadog/00000000-0000-0000-0000-000000000000" }), scope("aud is a datadog/ audience that does not match the configured org UUID")},
		"non-Datadog audience":                  {claims(func(c map[string]any) { c["aud"] = "https://other.example" }), scope("aud is not a Datadog audience")},
		"non-Datadog audience with dd_org_uuid": {claims(func(c map[string]any) { c["aud"], c["dd_org_uuid"] = "https://other.example", testOrgUUID }), scope("aud is not a Datadog audience")},
		"multiple audiences":                    {claims(func(c map[string]any) { c["aud"] = []string{"datadog/" + testOrgUUID, "https://other.example"} }), scope("aud has 2 values; only a single Datadog audience for the configured org is accepted")},
		"non-string audience":                   {claims(func(c map[string]any) { c["aud"] = 42 }), scope("aud is neither a string nor a list of strings")},
		"missing audience":                      {claims(func(c map[string]any) { delete(c, "aud") }), scope("no aud claim")},
		"uppercase AUD only":                    {jwt(encode(map[string]any{"AUD": "datadog/" + testOrgUUID, "exp": now.Add(time.Hour).Unix()})), scope("no aud claim")},
		"mismatched dd_org_uuid":                {claims(func(c map[string]any) { c["dd_org_uuid"] = "00000000-0000-0000-0000-000000000000" }), scope("dd_org_uuid does not match the configured org UUID")},
		"non-string dd_org_uuid":                {claims(func(c map[string]any) { c["dd_org_uuid"] = 1 }), scope("dd_org_uuid is not a string")},
		"missing exp":                           {claims(func(c map[string]any) { delete(c, "exp") }), scope("no exp claim")},
		"non-numeric exp":                       {claims(func(c map[string]any) { c["exp"] = "soon" }), scope("exp is not a number")},
		"expired":                               {claims(func(c map[string]any) { c["exp"] = now.Add(-time.Minute).Unix() }), scope("token expired at 2027-01-15T07:59:00Z")},
		"expires now":                           {claims(func(c map[string]any) { c["exp"] = now.Unix() }), scope("token expired at 2027-01-15T08:00:00Z")},
		"two segments":                          {"eyJhbGciOiJSUzI1NiJ9.eyJhdWQiOiJ4In0", scope("not a JWT")},
		"payload not base64url":                 {jwt("not*base64"), scope("JWT payload is not base64url")},
		"payload not a JSON object":             {jwt(encode([]string{"a"})), scope("JWT payload is not a JSON object")},
		"inner whitespace":                      {valid[:10] + " " + valid[10:], "workload token contains whitespace or non-printable characters"},
		"non-printable":                         {"abc\x01def", "workload token contains whitespace or non-printable characters"},
		"empty":                                 {"", "workload token is empty"},
		"whitespace only":                       {" \n\t", "workload token is empty"},
		"oversized":                             {strings.Repeat("a", maxTokenSize+1), "workload token exceeds 65536 bytes"},
	}
	dir := t.TempDir()
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(name, " ", "_"))
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o600))

			got, err := proof(path, testOrgUUID, now)
			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Equal(t, "oidc-"+strings.TrimSpace(tt.content), got)
				return
			}
			assert.Empty(t, got)
			assert.EqualError(t, err, tt.wantErr)
			var scopeErr *TokenScopeError
			assert.Equal(t, strings.HasPrefix(tt.wantErr, scope("")), errors.As(err, &scopeErr))
			if token := strings.TrimSpace(tt.content); token != "" {
				assert.NotContains(t, err.Error(), token)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		_, err := proof(filepath.Join(dir, "absent"), testOrgUUID, now)
		assert.ErrorContains(t, err, "opening workload token file: ")
	})
	t.Run("directory", func(t *testing.T) {
		_, err := proof(dir, testOrgUUID, now)
		assert.EqualError(t, err, "workload token file is not a regular file")
	})
	t.Run("missing org UUID", func(t *testing.T) {
		_, err := proof(filepath.Join(dir, "scoped"), "", now)
		assert.EqualError(t, err, "missing org UUID in delegated token configuration")
	})
}

func TestGenerateAuthProofMissingFile(t *testing.T) {
	auth, err := NewOIDCAuth(&cloudauthconfig.OIDCProviderConfig{TokenFile: filepath.Join(t.TempDir(), "absent")})
	require.NoError(t, err, "a token file mounted after startup must not fail construction")

	_, err = auth.GenerateAuthProof(context.Background(), nil, &common.AuthConfig{OrgUUID: testOrgUUID})
	assert.Error(t, err)
}

func TestGenerateAuthProofHonorsCanceledContext(t *testing.T) {
	auth, _ := newTestAuth(t, makeJWT(t, scopedClaims()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := auth.GenerateAuthProof(ctx, nil, &common.AuthConfig{OrgUUID: testOrgUUID})
	assert.ErrorIs(t, err, context.Canceled)
}
