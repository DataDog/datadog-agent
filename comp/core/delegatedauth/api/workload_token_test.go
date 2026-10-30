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
		name   string
		status int
		body   string
	}{
		{"denied", 403, `sensitive-response`},
		{"unavailable", 503, `sensitive-response`},
		{"redirect", 307, ""},
		{"invalid JSON", 200, `sensitive-response`},
		{"intake response", 200, `{"data":{"attributes":{"api_key":"sensitive-response"}}}`},
		{"missing token", 200, `{"data":{"type":"token","attributes":{}}}`},
		{"oversized", 200, strings.Repeat("x", maxResponseBodySize+1)},
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
			require.Error(t, err)
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
