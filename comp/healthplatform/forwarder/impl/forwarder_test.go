// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package forwarderimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func newTestForwarder(t *testing.T, cfg config.Component) *forwarder {
	t.Helper()
	return newTestForwarderWithTelemetry(t, cfg, telemetrymock.New(t))
}

func newTestForwarderWithTelemetry(t *testing.T, cfg config.Component, tel telemetry.Component) *forwarder {
	t.Helper()
	return &forwarder{
		cfg:        cfg,
		intakeURL:  buildIntakeURL(cfg),
		httpClient: buildHTTPClient(cfg),
		log:        logmock.New(t),
		httpErrors: newHTTPErrorsCounter(tel),
	}
}

func TestSendCountsHTTPErrors(t *testing.T) {
	statuses := []int{http.StatusBadRequest, http.StatusForbidden, http.StatusInternalServerError, http.StatusMovedPermanently}
	next := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(statuses[next])
		next++
	}))
	defer server.Close()

	cfg := config.NewMock(t)
	cfg.SetInTest("api_key", "test-api-key")
	tel := telemetrymock.New(t)
	fwd := newTestForwarderWithTelemetry(t, cfg, tel)
	fwd.intakeURL = server.URL
	// Don't follow the 301, so the forwarder sees it as a non-2xx response.
	fwd.httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	for range statuses {
		_, err := fwd.Send(context.Background(), &healthplatform.HealthReport{})
		require.Error(t, err)
	}

	metrics, err := tel.GetCountMetric("forwarder", "http_errors")
	require.NoError(t, err)
	got := map[string]float64{}
	for _, m := range metrics {
		assert.Equal(t, "health_platform", m.Tags()["forwarder"])
		got[m.Tags()["code"]] = m.Value()
	}
	// 3xx is a non-2xx response but not an HTTP error: it must not be counted.
	assert.Equal(t, map[string]float64{"4xx": 2, "5xx": 1}, got)
}

func TestStatusClass(t *testing.T) {
	for status, want := range map[int]string{
		200: "", 204: "", 301: "", 399: "",
		400: "4xx", 403: "4xx", 404: "4xx", 429: "4xx", 499: "4xx",
		500: "5xx", 503: "5xx", 599: "5xx",
		0: "", 600: "",
	} {
		assert.Equal(t, want, statusClass(status), "status %d", status)
	}
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
		name      string
		site      string
		ddURL     string
		healthURL string
		expected  string
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
			name:     "main dd_url is ignored",
			site:     "datad0g.com",
			ddURL:    "https://agent.datad0g.com",
			expected: "https://agenthealth-intake.datad0g.com./api/v2/agenthealth",
		},
		{
			name:      "health_platform.forwarder.dd_url overrides site",
			site:      "datadoghq.eu",
			healthURL: "https://proxy.example.com",
			expected:  "https://proxy.example.com/api/v2/agenthealth",
		},
		{
			name:      "health_platform.forwarder.dd_url wins over main dd_url",
			ddURL:     "https://agent.datad0g.com",
			healthURL: "https://proxy.example.com",
			expected:  "https://proxy.example.com/api/v2/agenthealth",
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
			if tt.healthURL != "" {
				cfg.SetInTest("health_platform.forwarder.dd_url", tt.healthURL)
			}
			assert.Equal(t, tt.expected, buildIntakeURL(cfg))
		})
	}
}
