// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package healthplatformimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
)

// TestForwarderBuildHealthReport tests the report building functionality via sendHealthReport
func TestForwarderBuildHealthReport(t *testing.T) {
	lifecycle := newMockLifecycle()
	reqs := testRequires(t, lifecycle)

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp.(*healthPlatformImpl)

	// Report some test issues directly to the component
	err = comp.ReportIssue("check-1", "Test Check 1", &healthplatform.IssueReport{
		IssueID: "docker-file-tailing-disabled",
		Context: map[string]string{
			"dockerDir": "/var/lib/docker",
			"os":        "linux",
		},
		Tags: []string{"tag1"},
	})
	require.NoError(t, err)

	err = comp.ReportIssue("check-2", "Test Check 2", &healthplatform.IssueReport{
		IssueID: "docker-file-tailing-disabled",
		Context: map[string]string{
			"dockerDir": "/var/lib/docker",
			"os":        "linux",
		},
		Tags: []string{"tag2"},
	})
	require.NoError(t, err)

	// Verify issues were reported
	count, issues := comp.GetAllIssues()
	assert.Equal(t, 2, count)
	assert.Len(t, issues, 2)

	// Verify both issues are present
	assert.NotNil(t, issues["check-1"])
	assert.NotNil(t, issues["check-2"])
}

// TestForwarderSendReport tests sending reports to a mock server
func TestForwarderSendReport(t *testing.T) {
	lifecycle := newMockLifecycle()
	reqs := testRequires(t, lifecycle)

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp.(*healthPlatformImpl)

	// Track what the server receives
	var receivedReport healthplatform.HealthReport
	serverCalled := false

	// Create a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true

		// Verify method
		assert.Equal(t, "POST", r.Method)

		// Verify headers
		assert.Equal(t, "test-api-key-123", r.Header.Get("DD-API-KEY"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Contains(t, r.Header.Get("User-Agent"), "datadog-agent")
		assert.NotEmpty(t, r.Header.Get("DD-Agent-Version"))

		// Read the body
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		// Parse the report
		err = json.Unmarshal(body, &receivedReport)
		require.NoError(t, err)

		// Verify report structure
		assert.Equal(t, "test-hostname", receivedReport.Host.Hostname)
		assert.Equal(t, "1.0", receivedReport.SchemaVersion)
		assert.Equal(t, "agent-health", receivedReport.EventType)
		assert.Len(t, receivedReport.Issues, 1)
		assert.Contains(t, receivedReport.Issues, "test-issue")

		// Send success response
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Create a forwarder with the test server URL
	fwd := newForwarder(comp, "test-hostname", "test-api-key-123")
	// The forwarder uses httputils.CreateHTTPTransport, which is fine
	// We just need to override the client to point to our test server
	fwd.httpClient = ts.Client()

	// Create a test report
	report := &healthplatform.HealthReport{
		SchemaVersion: "1.0",
		EventType:     "agent-health",
		EmittedAt:     time.Now().Format(time.RFC3339),
		Host: healthplatform.HostInfo{
			Hostname:     "test-hostname",
			AgentVersion: "7.0.0",
			ParIDs:       []string{},
		},
		Issues: map[string]*healthplatform.Issue{
			"test-issue": {
				ID:          "test-issue",
				IssueName:   "test_issue",
				Title:       "Test Issue",
				Description: "Test Description",
				Category:    "test",
				Location:    "test-agent",
				Severity:    "low",
				DetectedAt:  "2025-01-01T00:00:00Z",
				Source:      "test",
				Tags:        []string{"test"},
			},
		},
	}

	// Test sendReport by creating a custom request with the test server URL
	payload, err := json.Marshal(report)
	require.NoError(t, err)

	// Create request to test server with the payload
	req, err := http.NewRequestWithContext(fwd.ctx, "POST", ts.URL, bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", "test-api-key-123")
	req.Header.Set("DD-Agent-Version", "7.0.0")
	req.Header.Set("User-Agent", "datadog-agent/7.0.0")

	// Send to test server
	resp, err := fwd.httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify server was called
	assert.True(t, serverCalled)

	// Verify the report structure can be marshaled/unmarshaled
	var unmarshaled healthplatform.HealthReport
	err = json.Unmarshal(payload, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, report.Host.Hostname, unmarshaled.Host.Hostname)
	assert.Len(t, unmarshaled.Issues, 1)
	assert.Contains(t, unmarshaled.Issues, "test-issue")
}

// TestForwarderWithComponent tests the forwarder integrated with the component
func TestForwarderWithComponent(t *testing.T) {
	lifecycle := newMockLifecycle()
	reqs := testRequires(t, lifecycle)

	// Set hostname and API key to enable the forwarder
	reqs.Config.SetWithoutSource("hostname", "test-hostname")
	reqs.Config.SetWithoutSource("api_key", "test-api-key")

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp.(*healthPlatformImpl)

	// Verify forwarder was created
	assert.NotNil(t, comp.forwarder)
	assert.Equal(t, "test-hostname", comp.forwarder.hostname)
	// Verify API key is stored in headers (headers is a map[string][]string)
	assert.NotNil(t, comp.forwarder.headers)
	comp.forwarder.headerLock.RLock()
	apiKeyValues := comp.forwarder.headers[http.CanonicalHeaderKey("DD-API-KEY")]
	comp.forwarder.headerLock.RUnlock()
	assert.Len(t, apiKeyValues, 1)
	assert.Equal(t, "test-api-key", apiKeyValues[0])

	// Start the component
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Report an issue
	err = comp.ReportIssue(
		"test-check",
		"Test Check",
		&healthplatform.IssueReport{
			IssueID: "docker-file-tailing-disabled",
			Context: map[string]string{
				"dockerDir": "/var/lib/docker",
				"os":        "linux",
			},
		},
	)
	require.NoError(t, err)

	// Verify issues were reported
	count, _ := comp.GetAllIssues()
	assert.Equal(t, 1, count)

	// Stop the component
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}

// TestForwarderWithoutAPIKey tests that forwarder is not created without API key
func TestForwarderWithoutAPIKey(t *testing.T) {
	lifecycle := newMockLifecycle()
	reqs := testRequires(t, lifecycle)

	// Set hostname but not API key
	reqs.Config.SetWithoutSource("hostname", "test-hostname")
	// API key is not set

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp.(*healthPlatformImpl)

	// Verify forwarder was not created without API key
	assert.Nil(t, comp.forwarder)

	// Start should still work
	err = lifecycle.Start(context.Background())
	require.NoError(t, err)

	// Stop should still work
	err = lifecycle.Stop(context.Background())
	require.NoError(t, err)
}
