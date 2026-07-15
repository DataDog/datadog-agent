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
	"github.com/DataDog/datadog-agent/pkg/version"
)

func newTestForwarder(t *testing.T, cfg config.Component) *forwarder {
	t.Helper()
	return &forwarder{
		cfg:        cfg,
		intakeURL:  buildIntakeURL(cfg),
		httpClient: buildHTTPClient(cfg),
		log:        logmock.New(t),
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

	require.NoError(t, fwd.Send(context.Background(), report))

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

	err := fwd.Send(context.Background(), &healthplatform.HealthReport{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code: 500")
}

func TestSendNoAPIKey(t *testing.T) {
	cfg := config.NewMock(t)
	fwd := newTestForwarder(t, cfg)

	err := fwd.Send(context.Background(), &healthplatform.HealthReport{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API key not configured")
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
