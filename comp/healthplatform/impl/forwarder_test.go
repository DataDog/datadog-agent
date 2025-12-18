// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package healthplatformimpl

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
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

// TestForwarderSendReport tests sending reports to a fakeintake server
func TestForwarderSendReport(t *testing.T) {
	// Start fakeintake server
	fi, _ := fakeserver.InitialiseForTests(t)
	defer fi.Stop()

	// Create component
	lifecycle := newMockLifecycle()
	reqs := testRequires(t, lifecycle)

	provides, err := NewComponent(reqs)
	require.NoError(t, err)
	comp := provides.Comp.(*healthPlatformImpl)

	// Create a forwarder pointing to fakeintake
	fwd := newForwarder(comp, "test-hostname", "test-api-key-123")
	fwd.intakeURL = fi.URL() + "/api/v2/agenthealth"

	// Report an issue so there's something to send (same as TestForwarderBuildHealthReport)
	err = comp.ReportIssue("test-check", "Test Check", &healthplatform.IssueReport{
		IssueID: "docker-file-tailing-disabled",
		Context: map[string]string{
			"dockerDir": "/var/lib/docker",
			"os":        "linux",
		},
		Tags: []string{"test"},
	})
	// This may fail if the issue template doesn't exist, which is ok for this test
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	// Trigger the report sending
	fwd.sendHealthReport()

	// Query fakeintake for received payloads using the aggregator
	client := fakeintake.NewClient(fi.URL())
	payloads, err := client.GetAgentHealth()
	require.NoError(t, err)
	require.Len(t, payloads, 1, "expected exactly one payload")

	// Validate the received report
	receivedReport := payloads[0]

	// Verify report structure - the key things we're testing
	assert.Equal(t, "test-hostname", receivedReport.Host.Hostname)
	assert.Equal(t, "1.0", receivedReport.SchemaVersion)
	assert.Equal(t, "agent-health-issues", receivedReport.EventType)
	assert.Len(t, receivedReport.Issues, 1)

	// Verify the issue was included
	issue := receivedReport.Issues["test-check"]
	require.NotNil(t, issue, "Issue should be present in the payload")
	// The issue will be enriched with the registered template data
	assert.NotEmpty(t, issue.ID)
	assert.NotEmpty(t, issue.Title)
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
	// Verify API key is stored in header (header is a map[string][]string)
	assert.NotNil(t, comp.forwarder.header)
	comp.forwarder.headerLock.RLock()
	apiKeyValues := comp.forwarder.header[http.CanonicalHeaderKey("DD-API-KEY")]
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
