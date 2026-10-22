// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package phonehome

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryOptInPlatform(t *testing.T) {
	t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "false")
	cfg := configmock.New(t)
	cfg.SetInTest("api_key", "scope-test-key")
	cfg.SetInTest("private_action_runner.enabled", true)
	cfg.SetInTest("dd_url", "https://api.datadoghq.com")
	cfg.SetInTest("remote_configuration.rc_dd_url", "https://config.datadoghq.com")
	t.Setenv("DD_PAR_PHONE_HOME_POC", "true")
	rc := NewSource(cfg, &statusService{})
	poller := NewDiscovery(cfg, rc)
	require.Equal(t, runtime.GOOS == "windows", poller == nil)
	t.Setenv("DD_PAR_PHONE_HOME_POC", "false")
	require.Nil(t, NewDiscovery(cfg, rc))
	t.Setenv("DD_PAR_PHONE_HOME_POC", "true")
	require.Nil(t, NewDiscovery(cfg, nil))
}

func TestDiscoveryRCEndpointFQDN(t *testing.T) {
	t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "false")
	t.Setenv("DD_PAR_PHONE_HOME_POC", "true")
	for _, site := range []string{"datadoghq.com", "datadoghq.eu", "us3.datadoghq.com"} {
		for _, fqdn := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fqdn=%t", site, fqdn), func(t *testing.T) {
				cfg := configmock.New(t)
				cfg.SetInTest("api_key", "key")
				cfg.SetInTest("site", site)
				cfg.SetInTest("dd_url", "")
				cfg.SetInTest("convert_dd_site_fqdn.enabled", fqdn)
				cfg.SetInTest("private_action_runner.enabled", true)
				cfg.SetInTest("remote_configuration.enabled", true)
				rc := &statusService{rc: &pbgo.OrgStatusResponse{Enabled: true, Authorized: true}}
				for _, endpoint := range []string{"", "https://config." + site, "https://config." + site + "."} {
					cfg.SetInTest("remote_configuration.rc_dd_url", endpoint)
					poller := NewDiscovery(cfg, NewSource(cfg, rc))
					if runtime.GOOS == "windows" {
						require.Nil(t, poller)
						continue
					}
					poller.SetPARDiscoveryDemand(true)
					require.True(t, poller.gate(), endpoint)
				}
				for _, endpoint := range []string{"http://config." + site + ".", "https://config." + site + ".invalid", "https://config." + site + "/.", "https://config." + site + ".:443", "https://config." + site + "..", "https://config.us5.datadoghq.com."} {
					cfg.SetInTest("remote_configuration.rc_dd_url", endpoint)
					poller := NewDiscovery(cfg, NewSource(cfg, rc))
					if runtime.GOOS == "windows" {
						require.Nil(t, poller)
						continue
					}
					poller.SetPARDiscoveryDemand(true)
					require.False(t, poller.gate(), endpoint)
				}
			})
		}
	}
}

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
			require.Equal(t, tc.want, validate(context.Background(), srv.Client(), srv.URL, "fake-api-key", time.Now()))
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
	result := validate(context.Background(), client, srv.URL, "secret", time.Now())
	require.False(t, result.eligible)
	require.Equal(t, "discovery_http_302", result.reason)
}
