// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package healthplatformimpl

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
	"github.com/DataDog/datadog-agent/pkg/version"
)

// mockIssueProvider is a test implementation of issueProvider
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

// TestForwarderBuildReport tests report building
func TestForwarderBuildReport(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetInTest("api_key", "test-api-key")
	provider := newMockIssueProvider()

	fwd := newForwarder(cfg, provider, logmock.New(t), "test-host")

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

	report := fwd.buildReport(provider.issues)

	assert.Equal(t, "agent-health-issues", report.EventType)
	assert.Equal(t, "test-host", report.Host.Hostname)
	assert.Equal(t, version.AgentVersion, report.Host.AgentVersion)
	assert.Len(t, report.Issues, 2)
	assert.NotEmpty(t, report.EmittedAt)

	// Verify timestamp is valid RFC3339
	_, err := time.Parse(time.RFC3339, report.EmittedAt)
	assert.NoError(t, err)
}

// TestForwarderSend tests sending reports to a mock server
func TestForwarderSend(t *testing.T) {
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
	provider := newMockIssueProvider()

	fwd := newForwarder(cfg, provider, logmock.New(t), "test-host")
	fwd.intakeURL = server.URL

	provider.addIssue("check-1", &healthplatform.Issue{
		Id:       "issue-1",
		Title:    "Test Issue",
		Severity: "high",
	})

	report := fwd.buildReport(provider.issues)
	err := fwd.send(report)
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
func TestForwarderSendNoIssues(t *testing.T) {
	requestCount := int32(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.NewMock(t)
	cfg.SetInTest("api_key", "test-api-key")
	provider := newMockIssueProvider() // No issues

	fwd := newForwarder(cfg, provider, logmock.New(t), "test-host")
	fwd.intakeURL = server.URL

	fwd.sendHealthReport()

	// Verify no request was sent
	assert.Equal(t, int32(0), atomic.LoadInt32(&requestCount))
}

// TestForwarderSendError tests error handling when server returns error
func TestForwarderSendError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := config.NewMock(t)
	cfg.SetInTest("api_key", "test-api-key")
	provider := newMockIssueProvider()

	fwd := newForwarder(cfg, provider, logmock.New(t), "test-host")
	fwd.intakeURL = server.URL

	provider.addIssue("check-1", &healthplatform.Issue{
		Id:       "issue-1",
		Title:    "Test Issue",
		Severity: "high",
	})

	// This should not panic - error is logged internally
	fwd.sendHealthReport()
}

// TestForwarderStartStop tests the forwarder lifecycle
func TestForwarderStartStop(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetInTest("health_platform.forwarder.interval", 100*time.Millisecond)
	cfg.SetInTest("api_key", "test-api-key")

	provider := newMockIssueProvider()

	fwd := newForwarder(cfg, provider, logmock.New(t), "test-host")

	// Start the forwarder
	fwd.Start()

	// Give it a moment to start the goroutine
	time.Sleep(50 * time.Millisecond)

	// Stop the forwarder - should complete gracefully
	fwd.Stop()
}

// TestForwarderWithComponent tests forwarder integration with the health platform component
func TestForwarderWithComponent(t *testing.T) {
	lifecycle := newMockLifecycle()
	reqs := testRequires(t, lifecycle)

	// Set API key to enable forwarder
	reqs.Config.SetInTest("api_key", "test-api-key")

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp.(*healthPlatformImpl)

	// Verify forwarder was created
	require.NotNil(t, comp.forwarder)
	assert.Equal(t, "test-hostname", comp.forwarder.hostname)

	// Start component
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Stop component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}

// TestForwarderWithoutAPIKey tests that forwarder is created but send fails gracefully without API key
func TestForwarderWithoutAPIKey(t *testing.T) {
	lifecycle := newMockLifecycle()
	reqs := testRequires(t, lifecycle)

	// Don't set API key

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp.(*healthPlatformImpl)

	// Verify forwarder was still created (API key check happens at send time)
	assert.NotNil(t, comp.forwarder)
}

// TestForwarderSendWithoutAPIKey tests that send fails gracefully without API key
func TestForwarderSendWithoutAPIKey(t *testing.T) {
	cfg := config.NewMock(t)
	// Don't set API key
	provider := newMockIssueProvider()

	fwd := newForwarder(cfg, provider, logmock.New(t), "test-host")

	provider.addIssue("check-1", &healthplatform.Issue{
		Id:       "issue-1",
		Title:    "Test Issue",
		Severity: "high",
	})

	report := fwd.buildReport(provider.issues)
	err := fwd.send(report)

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
				cfg.SetInTest("site", tt.site)
			}
			if tt.ddURL != "" {
				cfg.SetInTest("dd_url", tt.ddURL)
			}

			url := buildIntakeURL(cfg)
			assert.Equal(t, tt.expected, url)
		})
	}
}
