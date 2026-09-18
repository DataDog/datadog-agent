// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ddlog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestProxyWarningDoesNotLeakCredentials(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetInTest("proxy.http", "http://user-sentinel:password-sentinel@proxy.example:8080")
	cfg.SetInTest("proxy.no_proxy", []string{"example.test"})
	cfg.SetInTest("no_proxy_nonexact_match", false)
	var messages []string
	ddlog.SetLogObserver(func(_ ddlog.LogLevel, message string) { messages = append(messages, message) })
	t.Cleanup(func() { ddlog.SetLogObserver(nil) })
	req, err := http.NewRequest(http.MethodPost, "http://credential-warning.example.test", nil)
	require.NoError(t, err)
	proxy, err := New(cfg).httpClient.Transport.(*http.Transport).Proxy(req)
	require.NoError(t, err)
	require.Equal(t, "proxy.example:8080", proxy.Host)
	warning := strings.Join(messages, "\n")
	require.Contains(t, warning, "Deprecation warning")
	require.NotContains(t, warning, "sentinel")
}

func TestSendHonorsTLSSettings(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = &tls.Config{MaxVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	cfg := config.NewMock(t)
	cfg.SetInTest("api_key", "dummy")
	cfg.SetInTest("dd_url", server.URL)
	_, err := New(cfg).Send(context.Background(), &healthplatform.HealthReport{})
	require.Error(t, err, "untrusted certificate must not be accepted by default")
	require.Zero(t, calls.Load())
	cfg.SetInTest("skip_ssl_validation", true)
	_, err = New(cfg).Send(context.Background(), &healthplatform.HealthReport{})
	require.NoError(t, err)
	require.EqualValues(t, 1, calls.Load())
	cfg.SetInTest("min_tls_version", "tlsv1.3")
	_, err = New(cfg).Send(context.Background(), &healthplatform.HealthReport{})
	require.Error(t, err, "configured TLS minimum must not be downgraded")
	require.EqualValues(t, 1, calls.Load())
}

func TestSendRejectsCredentialRedirect(t *testing.T) {
	var received atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	cfg := config.NewMock(t)
	cfg.SetInTest("api_key", "dummy-key")
	fwd := newTestForwarder(t, cfg)
	fwd.intakeURL = redirect.URL
	n, err := fwd.Send(context.Background(), &healthplatform.HealthReport{})
	assert.Error(t, err)
	assert.Zero(t, n)
	assert.Zero(t, received.Load(), "redirect must never receive the API key")
}

func TestSendCanceledContext(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		received.Add(1)
	}))
	defer server.Close()
	cfg := config.NewMock(t)
	cfg.SetInTest("api_key", "dummy-key")
	fwd := newTestForwarder(t, cfg)
	fwd.intakeURL = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fwd.Send(ctx, &healthplatform.HealthReport{})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, received.Load())
}

func newTestForwarder(t *testing.T, cfg config.Component) *Client {
	t.Helper()
	return New(cfg)
}

func TestSend(t *testing.T) {
	var receivedRequest *http.Request
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRequest = r
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		receivedBody = buf
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.NewMock(t)
	cfg.SetInTest("api_key", "test-api-key")

	fwd := newTestForwarder(t, cfg)
	fwd.intakeURL = server.URL

	report := &healthplatform.HealthReport{
		EventType: "agent-health-issues",
		EmittedAt: time.Now().UTC().Format(time.RFC3339),
		Host:      &healthplatform.HostInfo{Hostname: "test-host"},
		Issues: map[string]*healthplatform.Issue{
			"issue-1": {Id: "issue-1", Title: "Test Issue", Severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH},
		},
	}

	n, err := fwd.Send(context.Background(), report)
	require.NoError(t, err)
	assert.Positive(t, n)

	assert.Equal(t, "application/json", receivedRequest.Header.Get("Content-Type"))
	assert.Equal(t, "test-api-key", receivedRequest.Header.Get("DD-API-KEY"))
	assert.Equal(t, version.AgentVersion, receivedRequest.Header.Get("DD-Agent-Version"))
	assert.Contains(t, receivedRequest.Header.Get("User-Agent"), "datadog-agent/")

	var decoded healthplatform.HealthReport
	require.NoError(t, json.Unmarshal(receivedBody, &decoded))
	assert.Equal(t, "test-host", decoded.Host.Hostname)
}

func TestSendHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := config.NewMock(t)
	cfg.SetInTest("api_key", "test-api-key")

	fwd := newTestForwarder(t, cfg)
	fwd.intakeURL = server.URL

	n, err := fwd.Send(context.Background(), &healthplatform.HealthReport{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code: 500")
	assert.Zero(t, n)
}

func TestSendNoAPIKey(t *testing.T) {
	cfg := config.NewMock(t)
	fwd := newTestForwarder(t, cfg)

	n, err := fwd.Send(context.Background(), &healthplatform.HealthReport{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API key not configured")
	assert.Zero(t, n)
}

func TestBuildIntakeURL(t *testing.T) {
	tests := []struct {
		name     string
		site     string
		ddURL    string
		expected string
	}{
		{
			name:     "default site",
			expected: "https://agenthealth-intake.datadoghq.com./api/v2/agenthealth",
		},
		{
			name:     "eu site",
			site:     "datadoghq.eu",
			expected: "https://agenthealth-intake.datadoghq.eu./api/v2/agenthealth",
		},
		{
			name:     "custom dd_url overrides",
			site:     "datadoghq.eu",
			ddURL:    "https://custom.example.com",
			expected: "https://custom.example.com/api/v2/agenthealth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewMock(t)
			if tt.site != "" {
				cfg.SetInTest("site", tt.site)
			}
			if tt.ddURL != "" {
				cfg.SetInTest("dd_url", tt.ddURL)
			}
			assert.Equal(t, tt.expected, buildIntakeURL(cfg))
		})
	}
}
