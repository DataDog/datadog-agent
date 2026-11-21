// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/require"
)

type workloadProofFunc func(context.Context, pkgconfigmodel.Reader, *common.AuthConfig, common.WorkloadTokenRequest) (string, error)

func (f workloadProofFunc) GenerateWorkloadAuthProof(ctx context.Context, cfg pkgconfigmodel.Reader, auth *common.AuthConfig, r common.WorkloadTokenRequest) (string, error) {
	return f(ctx, cfg, auth, r)
}

func TestGetWorkloadToken(t *testing.T) {
	request := common.WorkloadTokenRequest{Grants: []string{"par_enrollment", "another_grant"}, JWKThumbprint: strings.Repeat("A", 43)}
	var signed common.WorkloadTokenRequest
	calls := 0
	provider := workloadProofFunc(func(_ context.Context, _ pkgconfigmodel.Reader, auth *common.AuthConfig, r common.WorkloadTokenRequest) (string, error) {
		require.Equal(t, "org-uuid", auth.OrgUUID)
		signed = r
		return "signed-proof", nil
	})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v2/delegated-token?token_type=workload", r.URL.RequestURI())
		require.Equal(t, "Delegated signed-proof", r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("DD-API-KEY"))
		require.Equal(t, "application/vnd.api+json", r.Header.Get("Content-Type"))
		var body struct {
			Data struct {
				Type       string                      `json:"type"`
				Attributes common.WorkloadTokenRequest `json:"attributes"`
			} `json:"data"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "delegated_token_request", body.Data.Type)
		require.Equal(t, signed, body.Data.Attributes)
		fmt.Fprint(w, `{"data":{"type":"token","attributes":{"access_token":"opaque-token","expires":"1800000000"}}}`)
	}))
	defer server.Close()
	cfg := mock.New(t)
	cfg.SetInTest("dd_url", server.URL)
	cfg.SetInTest("skip_ssl_validation", true)
	cfg.SetInTest("api_key", "existing-intake-key")
	token, err := GetWorkloadToken(context.Background(), cfg, provider, &common.AuthConfig{OrgUUID: "org-uuid"}, request, "")
	require.NoError(t, err)
	require.Equal(t, "opaque-token", token)
	require.Equal(t, 1, calls)
	require.Equal(t, "existing-intake-key", cfg.GetString("api_key"))
}

func TestGetWorkloadTokenFailures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantError string
	}{
		{"denied", 403, `sensitive-response`, "workload token request failed (HTTP 403)"},
		{"unavailable", 503, `sensitive-response`, "workload token request failed (HTTP 503)"},
		{"redirect", 307, "", "workload token request failed (HTTP 307)"},
		{"grant disabled", 403, `{"errors":[{"title":"Forbidden","detail":"Workload grant is disabled for this organization"}]}`, "workload token request failed (HTTP 403): Workload grant is disabled for this organization"},
		{"error title", 400, `{"errors":[{"title":"Unknown workload grant"}]}`, "workload token request failed (HTTP 400): Unknown workload grant"},
		{"error metadata", 403, `{"errors":[{"detail":"Workload grant denied","meta":{"proof":"sensitive-proof"}}]}`, "workload token request failed (HTTP 403): Workload grant denied"},
		{"token in error response", 403, `{"data":{"type":"token","attributes":{"access_token":"sensitive-token"}}}`, "workload token request failed (HTTP 403)"},
		{"intake key in error response", 403, `{"data":{"attributes":{"api_key":"sensitive-key"}}}`, "workload token request failed (HTTP 403)"},
		{"invalid JSON", 200, `sensitive-response`, "invalid workload token response"},
		{"intake response", 200, `{"data":{"attributes":{"api_key":"sensitive-response"}}}`, "invalid workload token response"},
		{"missing token", 200, `{"data":{"type":"token","attributes":{}}}`, "invalid workload token response"},
		{"oversized", 200, strings.Repeat("x", maxResponseBodySize+1), "workload token response exceeds size limit"},
		{"oversized error", 403, `{"errors":[{"detail":"sensitive-response"}]}` + strings.Repeat(" ", maxResponseBodySize), "workload token response exceeds size limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.Header().Set("Location", "/redirect-target")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			cfg := mock.New(t)
			cfg.SetInTest("dd_url", server.URL)
			cfg.SetInTest("skip_ssl_validation", true)
			provider := workloadProofFunc(func(context.Context, pkgconfigmodel.Reader, *common.AuthConfig, common.WorkloadTokenRequest) (string, error) {
				return "sensitive-proof", nil
			})
			token, err := GetWorkloadToken(context.Background(), cfg, provider, &common.AuthConfig{OrgUUID: "org"}, common.WorkloadTokenRequest{Grants: []string{"par_enrollment"}}, "")
			require.EqualError(t, err, tc.wantError)
			require.Empty(t, token)
			require.NotContains(t, err.Error(), "sensitive")
			require.Equal(t, 1, calls, "no retry or redirect with the same proof")
		})
	}
}

func TestGetWorkloadTokenStopsBeforeExchange(t *testing.T) {
	cfg := mock.New(t)
	provider := workloadProofFunc(func(ctx context.Context, _ pkgconfigmodel.Reader, _ *common.AuthConfig, _ common.WorkloadTokenRequest) (string, error) {
		return "", ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GetWorkloadToken(ctx, cfg, provider, &common.AuthConfig{OrgUUID: "org"}, common.WorkloadTokenRequest{Grants: []string{"par_enrollment"}}, "datadoghq.com")
	require.ErrorIs(t, err, context.Canceled)
	// An insecure target must be rejected before resolving credentials.
	provider = workloadProofFunc(func(context.Context, pkgconfigmodel.Reader, *common.AuthConfig, common.WorkloadTokenRequest) (string, error) {
		t.Fatal("proof should not be requested")
		return "", nil
	})
	cfg.SetInTest("dd_url", "http://127.0.0.1:1")
	_, err = GetWorkloadToken(context.Background(), cfg, provider, nil, common.WorkloadTokenRequest{Grants: []string{"par_enrollment"}}, "")
	require.ErrorContains(t, err, "HTTPS")
	_, err = GetWorkloadToken(context.Background(), cfg, provider, nil, common.WorkloadTokenRequest{}, "datadoghq.com")
	require.ErrorContains(t, err, "grants")
}

func TestGetWorkloadTokenWithExpiry(t *testing.T) {
	provider := workloadProofFunc(func(context.Context, pkgconfigmodel.Reader, *common.AuthConfig, common.WorkloadTokenRequest) (string, error) {
		return "signed-proof", nil
	})
	request := common.WorkloadTokenRequest{Grants: []string{"remote_config"}}

	for _, tc := range []struct {
		name       string
		body       string
		wantExpiry time.Time
	}{
		{
			name:       "unix seconds string",
			body:       `{"data":{"type":"token","attributes":{"access_token":"rc-token","expires":"1800000000"}}}`,
			wantExpiry: time.Unix(1800000000, 0).UTC(),
		},
		{
			name:       "unix seconds integer",
			body:       `{"data":{"type":"token","attributes":{"access_token":"rc-token","expires":1800000000}}}`,
			wantExpiry: time.Unix(1800000000, 0).UTC(),
		},
		{
			name:       "rfc3339 string",
			body:       `{"data":{"type":"token","attributes":{"access_token":"rc-token","expires":"2027-01-15T12:00:00Z"}}}`,
			wantExpiry: time.Date(2027, 1, 15, 12, 0, 0, 0, time.UTC),
		},
		{
			name:       "missing expiry falls back to zero",
			body:       `{"data":{"type":"token","attributes":{"access_token":"rc-token"}}}`,
			wantExpiry: time.Time{},
		},
		{
			name:       "empty expiry falls back to zero",
			body:       `{"data":{"type":"token","attributes":{"access_token":"rc-token","expires":""}}}`,
			wantExpiry: time.Time{},
		},
		{
			name:       "unparseable expiry falls back to zero",
			body:       `{"data":{"type":"token","attributes":{"access_token":"rc-token","expires":"not-a-timestamp"}}}`,
			wantExpiry: time.Time{},
		},
		{
			name:       "zero or negative expiry falls back to zero",
			body:       `{"data":{"type":"token","attributes":{"access_token":"rc-token","expires":"-10"}}}`,
			wantExpiry: time.Time{},
		},
		{
			name:       "unexpected object expiry does not reject valid token",
			body:       `{"data":{"type":"token","attributes":{"access_token":"rc-token","expires":{"bad":true}}}}`,
			wantExpiry: time.Time{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			cfg := mock.New(t)
			cfg.SetInTest("dd_url", server.URL)
			cfg.SetInTest("skip_ssl_validation", true)

			tok, err := GetWorkloadTokenWithExpiry(context.Background(), cfg, provider, &common.AuthConfig{OrgUUID: "org-uuid"}, request, "")
			require.NoError(t, err)
			require.Equal(t, "rc-token", tok.Token)
			require.Equal(t, tc.wantExpiry, tok.ExpiresAt)
		})
	}
}

func TestGetWorkloadTokenHTTPStatusClassification(t *testing.T) {
	provider := workloadProofFunc(func(context.Context, pkgconfigmodel.Reader, *common.AuthConfig, common.WorkloadTokenRequest) (string, error) {
		return "signed-proof", nil
	})
	request := common.WorkloadTokenRequest{Grants: []string{"remote_config"}}

	for _, tc := range []struct {
		name       string
		status     int
		body       string
		wantDenied bool
	}{
		{
			name:       "403 denied",
			status:     http.StatusForbidden,
			body:       `{"errors":[{"detail":"Workload grant denied"}]}`,
			wantDenied: true,
		},
		{
			name:       "503 transient",
			status:     http.StatusServiceUnavailable,
			body:       `{"errors":[{"detail":"Signing unavailable"}]}`,
			wantDenied: false,
		},
		{
			name:       "400 bad request",
			status:     http.StatusBadRequest,
			body:       `{"errors":[{"detail":"Unknown workload grant"}]}`,
			wantDenied: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			cfg := mock.New(t)
			cfg.SetInTest("dd_url", server.URL)
			cfg.SetInTest("skip_ssl_validation", true)

			tok, err := GetWorkloadTokenWithExpiry(context.Background(), cfg, provider, &common.AuthConfig{OrgUUID: "org-uuid"}, request, "")
			require.Error(t, err)
			require.Empty(t, tok.Token)
			var httpErr *common.WorkloadTokenHTTPError
			require.ErrorAs(t, err, &httpErr)
			require.Equal(t, tc.status, httpErr.StatusCode)
			if tc.wantDenied {
				require.ErrorIs(t, err, common.ErrWorkloadTokenDenied)
			} else {
				require.NotErrorIs(t, err, common.ErrWorkloadTokenDenied)
			}
		})
	}
}
