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
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// mockIssueProvider is a test implementation of IssueProvider
type mockIssueProvider struct {
	issues map[string]*healthplatform.Issue
}

func newMockIssueProvider() *mockIssueProvider {
	return &mockIssueProvider{
		issues: make(map[string]*healthplatform.Issue),
	}
}

func (m *mockIssueProvider) GetAllIssues() (int, map[string]*healthplatform.Issue) {
	count := 0
	for _, issue := range m.issues {
		if issue != nil {
			count++
		}
	}
	return count, m.issues
}

func (m *mockIssueProvider) addIssue(checkID string, issue *healthplatform.Issue) {
	m.issues[checkID] = issue
}

// newTestForwarder creates a forwarder for white-box testing
func newTestForwarder(t *testing.T, cfg config.Component, provider *mockIssueProvider, hostname string) *forwarder {
	interval := cfg.GetDuration("health_platform.forwarder.interval")
	if interval <= 0 {
		interval = defaultReporterInterval
	}
	return &forwarder{
		cfg:        cfg,
		intakeURL:  buildIntakeURL(cfg),
		interval:   interval,
		hostname:   hostname,
		provider:   provider,
		httpClient: buildHTTPClient(cfg),
		log:        logmock.New(t),
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
}

// TestForwarderBuildReport tests report building
func TestReporterBuildReport(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetWithoutSource("api_key", "test-api-key")
	provider := newMockIssueProvider()

	rptr := newTestForwarder(t, cfg, provider, "test-host")

	// Add some test issues
	provider.addIssue("check-1", &healthplatform.Issue{
		Id:       "issue-1",
		Title:    "Test Issue 1",
		Severity: "high",
	})
	provider.addIssue("check-2", &healthplatform.Issue{
		Id:       "issue-2",
		Title:    "Test Issue 2",
		Severity: "medium",
	})

	report := rptr.buildReport(provider.issues)

	assert.Equal(t, "agent-health-issues", report.EventType)
	assert.Equal(t, flavor.DefaultAgent, report.Service)
	assert.Equal(t, "test-host", report.Host.Hostname)
	assert.Equal(t, version.AgentVersion, report.Host.GetAgentVersion())
	assert.Len(t, report.Issues, 2)
	assert.NotEmpty(t, report.EmittedAt)

	// Verify timestamp is valid RFC3339
	_, err := time.Parse(time.RFC3339, report.EmittedAt)
	assert.NoError(t, err)
}

// TestForwarderSend tests sending reports to a mock server
func TestReporterSend(t *testing.T) {
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
	cfg.SetWithoutSource("api_key", "test-api-key")
	provider := newMockIssueProvider()

	rptr := newTestForwarder(t, cfg, provider, "test-host")
	rptr.intakeURL = server.URL

	provider.addIssue("check-1", &healthplatform.Issue{
		Id:       "issue-1",
		Title:    "Test Issue",
		Severity: "high",
	})

	report := rptr.buildReport(provider.issues)
	err := rptr.send(report)
	require.NoError(t, err)

	// Verify request headers
	assert.Equal(t, "application/json", receivedRequest.Header.Get("Content-Type"))
	assert.Equal(t, "test-api-key", receivedRequest.Header.Get("DD-API-KEY")) // API key read from config at request time
	assert.Equal(t, version.AgentVersion, receivedRequest.Header.Get("DD-Agent-Version"))
	assert.Contains(t, receivedRequest.Header.Get("User-Agent"), "datadog-agent/")

	// Verify body can be unmarshaled
	var receivedReport healthplatform.HealthReport
	err = json.Unmarshal(receivedBody, &receivedReport)
	require.NoError(t, err)
	assert.Equal(t, "test-host", receivedReport.Host.Hostname)
}

// TestForwarderSendNoIssues tests that no request is sent when there are no issues
func TestReporterSendNoIssues(t *testing.T) {
	requestCount := int32(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.NewMock(t)
	cfg.SetWithoutSource("api_key", "test-api-key")
	provider := newMockIssueProvider() // No issues

	rptr := newTestForwarder(t, cfg, provider, "test-host")
	rptr.intakeURL = server.URL

	rptr.sendHealthReport()

	// Verify no request was sent
	assert.Equal(t, int32(0), atomic.LoadInt32(&requestCount))
}

// TestForwarderSendError tests error handling when server returns error
func TestReporterSendError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := config.NewMock(t)
	cfg.SetWithoutSource("api_key", "test-api-key")
	provider := newMockIssueProvider()

	rptr := newTestForwarder(t, cfg, provider, "test-host")
	rptr.intakeURL = server.URL

	provider.addIssue("check-1", &healthplatform.Issue{
		Id:       "issue-1",
		Title:    "Test Issue",
		Severity: "high",
	})

	// This should not panic - error is logged internally
	rptr.sendHealthReport()
}

// TestForwarderStartStop tests the reporter lifecycle
func TestReporterStartStop(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetWithoutSource("health_platform.forwarder.interval", 100*time.Millisecond)
	cfg.SetWithoutSource("api_key", "test-api-key")

	provider := newMockIssueProvider()

	rptr := newTestForwarder(t, cfg, provider, "test-host")

	// Start the forwarder
	rptr.start(context.Background()) //nolint:errcheck

	// Give it a moment to start the goroutine
	time.Sleep(50 * time.Millisecond)

	// Stop the forwarder - should complete gracefully
	rptr.stop(context.Background()) //nolint:errcheck
}

// TestForwarderSendWithoutAPIKey tests that send fails gracefully without API key
func TestReporterSendWithoutAPIKey(t *testing.T) {
	cfg := config.NewMock(t)
	// Don't set API key
	provider := newMockIssueProvider()

	rptr := newTestForwarder(t, cfg, provider, "test-host")

	provider.addIssue("check-1", &healthplatform.Issue{
		Id:       "issue-1",
		Title:    "Test Issue",
		Severity: "high",
	})

	report := rptr.buildReport(provider.issues)
	err := rptr.send(report)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "API key not configured")
}

// TestBuildIntakeURL tests URL building based on site configuration
func TestBuildIntakeURL(t *testing.T) {
	tests := []struct {
		name     string
		site     string
		ddURL    string
		expected string
	}{
		{
			name:     "default site",
			site:     "",
			ddURL:    "",
			expected: "https://event-platform-intake.datadoghq.com./api/v2/agenthealth", // FQDN with trailing dot
		},
		{
			name:     "eu site",
			site:     "datadoghq.eu",
			ddURL:    "",
			expected: "https://event-platform-intake.datadoghq.eu./api/v2/agenthealth", // FQDN with trailing dot
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
				cfg.SetWithoutSource("site", tt.site)
			}
			if tt.ddURL != "" {
				cfg.SetWithoutSource("dd_url", tt.ddURL)
			}

			url := buildIntakeURL(cfg)
			assert.Equal(t, tt.expected, url)
		})
	}
}

// TestForwarderNewWithHostname tests that New creates a forwarder with the given hostname
func TestReporterNewWithHostname(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetWithoutSource("api_key", "test-api-key")
	provider := newMockIssueProvider()

	rptr := New(logmock.New(t), cfg, "test-hostname")
	rptr.SetProvider(provider)

	concreteReporter, ok := rptr.(*forwarder)
	require.True(t, ok)
	assert.Equal(t, "test-hostname", concreteReporter.hostname)
}
