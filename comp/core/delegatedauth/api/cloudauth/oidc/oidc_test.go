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

	oidcauth "github.com/DataDog/datadog-api-client-go/v2/auth/oidc"
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

func TestGenerateAuthProofRefusesUnscopedTokens(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"wrong org audience", func(c map[string]any) { c["aud"] = "datadog/00000000-0000-0000-0000-000000000000" }},
		{"multiple audiences", func(c map[string]any) { c["aud"] = []string{"datadog/" + testOrgUUID, "https://other.example"} }},
		{"non-Datadog audience with dd_org_uuid", func(c map[string]any) {
			c["aud"] = "https://other.example"
			c["dd_org_uuid"] = testOrgUUID
		}},
		{"mismatched dd_org_uuid", func(c map[string]any) { c["dd_org_uuid"] = "00000000-0000-0000-0000-000000000000" }},
		{"missing audience", func(c map[string]any) { delete(c, "aud") }},
		{"missing exp", func(c map[string]any) { delete(c, "exp") }},
		{"expired", func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := scopedClaims()
			tt.mutate(claims)
			token := makeJWT(t, claims)
			auth, _ := newTestAuth(t, token)

			proof, err := auth.GenerateAuthProof(context.Background(), nil, &common.AuthConfig{OrgUUID: testOrgUUID})
			require.Error(t, err)
			assert.Empty(t, proof)
			var scopeErr *TokenScopeError
			assert.ErrorAs(t, err, &scopeErr)
			assert.NotContains(t, err.Error(), token)
		})
	}
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

// The port must keep refusing exactly what the client library refuses, with the same reasons.
func TestMatchesClientLibrary(t *testing.T) {
	encode := func(v any) string {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	jwt := func(payload string) string {
		return encode(map[string]string{"alg": "RS256"}) + "." + payload + ".c2lnbmF0dXJl"
	}
	withClaims := func(mutate func(map[string]any)) string {
		claims := scopedClaims()
		mutate(claims)
		return jwt(encode(claims))
	}
	contents := map[string]string{
		"scoped":                    withClaims(func(map[string]any) {}),
		"scoped with newline":       withClaims(func(map[string]any) {}) + "\n",
		"matching dd_org_uuid":      withClaims(func(c map[string]any) { c["dd_org_uuid"] = testOrgUUID }),
		"wrong org audience":        withClaims(func(c map[string]any) { c["aud"] = "datadog/00000000-0000-0000-0000-000000000000" }),
		"non-Datadog audience":      withClaims(func(c map[string]any) { c["aud"] = "https://other.example" }),
		"audience list of one":      withClaims(func(c map[string]any) { c["aud"] = []string{"datadog/" + testOrgUUID} }),
		"multiple audiences":        withClaims(func(c map[string]any) { c["aud"] = []string{"datadog/" + testOrgUUID, "x"} }),
		"non-string audience":       withClaims(func(c map[string]any) { c["aud"] = 42 }),
		"missing audience":          withClaims(func(c map[string]any) { delete(c, "aud") }),
		"mismatched dd_org_uuid":    withClaims(func(c map[string]any) { c["dd_org_uuid"] = "other" }),
		"non-string dd_org_uuid":    withClaims(func(c map[string]any) { c["dd_org_uuid"] = 1 }),
		"missing exp":               withClaims(func(c map[string]any) { delete(c, "exp") }),
		"non-numeric exp":           withClaims(func(c map[string]any) { c["exp"] = "soon" }),
		"expired":                   withClaims(func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() }),
		"uppercase AUD only":        jwt(encode(map[string]any{"AUD": "datadog/" + testOrgUUID, "exp": time.Now().Add(time.Hour).Unix()})),
		"payload not base64url":     jwt("not*base64"),
		"payload not a JSON object": jwt(encode([]string{"a"})),
		"two segments":              "eyJhbGciOiJSUzI1NiJ9.eyJhdWQiOiJ4In0",
		"inner whitespace":          withClaims(func(map[string]any) {})[:10] + " " + withClaims(func(map[string]any) {})[10:],
		"empty":                     "",
		"whitespace only":           " \n\t",
		"oversized":                 strings.Repeat("a", 64*1024+1),
		"non-printable":             "abc\x01def",
	}
	dir := t.TempDir()
	for name, content := range contents {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(name, " ", "_"))
			require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
			library, err := oidcauth.New(oidcauth.WithTokenFile(path))
			require.NoError(t, err)

			wantProof, wantErr := library.Proof(testOrgUUID)
			gotProof, gotErr := proof(path, testOrgUUID, time.Now())

			assert.Equal(t, wantProof, gotProof)
			if wantErr == nil {
				assert.NoError(t, gotErr)
				return
			}
			require.Error(t, gotErr)
			assert.Equal(t, wantErr.Error(), gotErr.Error())
			var libraryScopeErr *oidcauth.TokenScopeError
			var scopeErr *TokenScopeError
			assert.Equal(t, errors.As(wantErr, &libraryScopeErr), errors.As(gotErr, &scopeErr), "refusals must be scope errors exactly when the library's are")
		})
	}

	t.Run("missing file", func(t *testing.T) {
		path := filepath.Join(dir, "absent")
		library, err := oidcauth.New(oidcauth.WithTokenFile(path))
		require.NoError(t, err)
		_, wantErr := library.Proof(testOrgUUID)
		_, gotErr := proof(path, testOrgUUID, time.Now())
		require.Error(t, wantErr)
		assert.Equal(t, wantErr.Error(), gotErr.Error())
	})
	t.Run("directory", func(t *testing.T) {
		library, err := oidcauth.New(oidcauth.WithTokenFile(dir))
		require.NoError(t, err)
		_, wantErr := library.Proof(testOrgUUID)
		_, gotErr := proof(dir, testOrgUUID, time.Now())
		require.Error(t, wantErr)
		assert.Equal(t, wantErr.Error(), gotErr.Error())
	})
	t.Run("missing org UUID", func(t *testing.T) {
		path := filepath.Join(dir, "scoped")
		library, err := oidcauth.New(oidcauth.WithTokenFile(path))
		require.NoError(t, err)
		_, wantErr := library.Proof("")
		_, gotErr := proof(path, "", time.Now())
		require.Error(t, wantErr)
		assert.Equal(t, wantErr.Error(), gotErr.Error())
	})
}
