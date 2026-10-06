// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package phonehome

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDiscovery(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		body, retry string
		want        discovery
	}{
		{"scope", 200, `{"data":{"attributes":{"valid":true,"api_key_scopes":["private_action_runner_enroll"]}}}`, "", discovery{eligible: true, reason: "scope_present", delay: pollInterval}},
		{"rc_not_par", 200, `{"data":{"attributes":{"valid":true,"api_key_scopes":["remote_config_read"]}}}`, "", discovery{reason: "missing_enrollment_scope", delay: pollInterval}},
		{"invalid", 200, `{"data":{"attributes":{"valid":false,"api_key_scopes":["private_action_runner_enroll"]}}}`, "", discovery{reason: "invalid_credentials", delay: slowRetry}},
		{"unauthorized", 401, "secret body", "", discovery{reason: "invalid_credentials", delay: slowRetry}},
		{"forbidden_not_missing_scope", 403, "secret body", "", discovery{reason: "discovery_http_403", delay: slowRetry}},
		{"outage", 503, "", "", discovery{reason: "discovery_unavailable", transient: true}},
		{"throttled", 429, "", "3600", discovery{reason: "discovery_rate_limited", delay: time.Hour}},
		{"malformed", 200, `{}`, "", discovery{reason: "invalid_discovery_response", transient: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "GET", r.Method)
				require.Equal(t, "/api/v2/validate", r.URL.Path)
				require.Equal(t, "fake-api-key", r.Header.Get("DD-API-KEY"))
				require.Empty(t, r.URL.RawQuery)
				require.Empty(t, r.Header.Get("Authorization"))
				w.Header().Set("Retry-After", tc.retry)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			require.Equal(t, tc.want, validate(context.Background(), srv.Client(), srv.URL, "fake-api-key"))
		})
	}
	now := time.Now().Truncate(time.Second)
	require.Equal(t, 2*time.Hour, retryAfter(now.Add(2*time.Hour).UTC().Format(http.TimeFormat), now))
	for _, value := range []string{"-1", "invalid", "0"} {
		require.Equal(t, pollInterval, retryAfter(value, now))
	}
}

func TestDiscoveryRefusesRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { t.Error("credential leaked to redirect target") }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer srv.Close()
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	result := validate(context.Background(), client, srv.URL, "secret")
	require.False(t, result.eligible)
	require.Equal(t, "discovery_http_302", result.reason)
}
