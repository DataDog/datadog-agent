// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package oidc

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	"github.com/stretchr/testify/require"
)

type tokenFunc func(context.Context, string) (string, error)

func (f tokenFunc) Token(ctx context.Context, aud string) (string, error) { return f(ctx, aud) }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProofProtocolAndAudience(t *testing.T) {
	for _, mode := range []string{"claims", "suffix"} {
		t.Run(mode, func(t *testing.T) {
			a, err := New(tokenFunc(func(_ context.Context, aud string) (string, error) {
				require.Equal(t, "datadog/org-123", aud)
				return "  header.payload.signature\n", nil
			}), "", "custom", mode)
			require.NoError(t, err)
			proof, err := a.GenerateAuthProof(context.Background(), nil, &common.AuthConfig{OrgUUID: "org-123"})
			require.NoError(t, err)
			want := "oidc-header.payload.signature"
			if mode == "suffix" {
				want += ":org-123"
			}
			require.Equal(t, want, proof)
		})
	}
}

func TestFileRotationAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	a, err := FromConfig(&cloudauthconfig.OIDCProviderConfig{Source: "file", TokenFile: path})
	require.NoError(t, err)
	for _, token := range []string{"header.first.signature", "header.second.signature"} {
		require.NoError(t, os.WriteFile(path+".new", []byte(token), 0600))
		require.NoError(t, os.Rename(path+".new", path))
		proof, err := a.GenerateAuthProof(context.Background(), nil, &common.AuthConfig{OrgUUID: "org"})
		require.NoError(t, err)
		require.Equal(t, "oidc-"+token, proof)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = a.GenerateAuthProof(ctx, nil, &common.AuthConfig{OrgUUID: "org"})
	require.ErrorIs(t, err, context.Canceled)
}

func TestRejectMalformedTokensWithoutDisclosure(t *testing.T) {
	for _, token := range []string{"", "secret-token", "a.b.", "a.b.c\r\nAuthorization: secret", strings.Repeat("a", maxTokenSize) + ".b.c"} {
		a, err := New(tokenFunc(func(context.Context, string) (string, error) { return token, nil }), "", "test", "")
		require.NoError(t, err)
		proof, err := a.GenerateAuthProof(context.Background(), nil, &common.AuthConfig{OrgUUID: "org"})
		require.Error(t, err)
		require.Empty(t, proof)
		require.NotContains(t, err.Error(), "secret")
	}
}

func TestMetadataSources(t *testing.T) {
	for _, provider := range []string{"gcp", "azure"} {
		t.Run(provider, func(t *testing.T) {
			a, err := FromConfig(&cloudauthconfig.OIDCProviderConfig{Source: provider, Audience: "api://datadog"})
			require.NoError(t, err)
			s := a.source.(*metadataSource)
			require.Nil(t, s.client.Transport.(*http.Transport).Proxy)
			require.ErrorIs(t, s.client.CheckRedirect(nil, nil), http.ErrUseLastResponse)
			s.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				body := "header.payload.signature"
				if provider == "gcp" {
					require.Equal(t, "metadata.google.internal", r.URL.Host)
					require.Equal(t, "Google", r.Header.Get("Metadata-Flavor"))
					require.Equal(t, "api://datadog", r.URL.Query().Get("audience"))
					require.Equal(t, "full", r.URL.Query().Get("format"))
				} else {
					require.Equal(t, "169.254.169.254", r.URL.Host)
					require.Equal(t, "true", r.Header.Get("Metadata"))
					require.Equal(t, "api://datadog", r.URL.Query().Get("resource"))
					body = `{"access_token":"header.payload.signature"}`
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			proof, err := a.GenerateAuthProof(context.Background(), nil, &common.AuthConfig{OrgUUID: "org"})
			require.NoError(t, err)
			require.Equal(t, "oidc-header.payload.signature", proof)
		})
	}
}

func TestMetadataFailuresAreBoundedAndRedacted(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		err    error
	}{
		{403, "secret", nil}, {200, `{"access_token":"secret`, nil},
		{200, strings.Repeat("x", maxTokenSize+1), nil}, {0, "", errors.New("secret")},
	} {
		s := &metadataSource{provider: "azure", client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			if tc.err != nil {
				return nil, tc.err
			}
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})}}
		_, err := s.Token(context.Background(), "audience")
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
}

func TestInvalidConfig(t *testing.T) {
	for _, c := range []*cloudauthconfig.OIDCProviderConfig{
		nil, {}, {Source: "file"}, {Source: "file", TokenFile: "relative"},
		{Source: "azure"}, {Source: "gcp", TokenFile: "/token"},
		{Source: "gcp", ClientID: "azure-id"}, {Source: "gcp", OrgUUIDSource: "invalid"},
	} {
		_, err := FromConfig(c)
		require.Error(t, err)
	}
}
