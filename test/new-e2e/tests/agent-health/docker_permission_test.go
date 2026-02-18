// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package agenthealth contains E2E tests for the agent health reporting functionality.
package agenthealth

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

// issueState mirrors healthplatformimpl.IssueState (can't import from main module)
type issueState string

const (
	issueStateNew      issueState = "new"
	issueStateOngoing  issueState = "ongoing"
	issueStateResolved issueState = "resolved"
)

// persistedIssue mirrors healthplatformimpl.PersistedIssue
type persistedIssue struct {
	IssueID    string     `json:"issue_id"`
	State      issueState `json:"state"`
	FirstSeen  string     `json:"first_seen"`
	LastSeen   string     `json:"last_seen"`
	ResolvedAt string     `json:"resolved_at,omitempty"`
}

// persistedState mirrors healthplatformimpl.PersistedState
type persistedState struct {
	UpdatedAt string                     `json:"updated_at"`
	Issues    map[string]*persistedIssue `json:"issues"`
}

const (
	// dockerPermissionsCheckID mirrors dockerpermissions.CheckID
	dockerPermissionsCheckID = "docker-socket-permissions"
)

const (
	expectedIssueID = "docker-file-tailing-disabled"

	// persistenceFilePath is the absolute path to the health platform persistence file on the e2e VM
	persistenceFilePath = "/opt/datadog-agent/run/health-platform/issues.json"
)

type dockerPermissionSuite struct {
	e2e.BaseSuite[dockerPermissionEnv]
}

// TestDockerPermissionSuite runs the docker permission health check test
func TestDockerPermissionSuite(t *testing.T) {
	e2e.Run(t, &dockerPermissionSuite{},
		e2e.WithPulumiProvisioner(dockerPermissionEnvProvisioner(), nil),
	)
}

// TestDockerPermissionIssueLifecycle tests the full lifecycle of a docker permission issue:
//  1. Detection: agent detects the issue and reports it to fakeintake
//  2. Persistence: issue is persisted to disk with "new" state
//  3. Restart resilience: after agent restart, issue is loaded from disk and transitions to "ongoing"
//  4. Resolution: after fixing permissions and restarting, the issue is marked "resolved" on disk
func (suite *dockerPermissionSuite) TestDockerPermissionIssueLifecycle() {
	host := suite.Env().RemoteHost
	agent := suite.Env().Agent
	fakeIntake := suite.Env().Fakeintake.Client()

	// =========================================================================
	// Phase 1: Issue Detection
	// =========================================================================
	suite.T().Log("=== Phase 1: Issue Detection ===")

	// Verify agent is running
	require.EventuallyWithT(suite.T(), func(t *assert.CollectT) {
		assert.True(t, agent.Client.IsReady(), "Agent should be ready")
	}, 2*time.Minute, 10*time.Second, "Agent not ready")

	// Verify containers are running
	output := host.MustExecute("docker ps --format '{{.Names}}' | grep spam")
	suite.T().Logf("Running containers: %s", output)
	assert.Contains(suite.T(), output, "spam", "Busybox containers should be running")

	// Wait for health report to be sent to fakeintake
	var healthPayloads []*aggregator.AgentHealthPayload
	require.EventuallyWithT(suite.T(), func(t *assert.CollectT) {
		var err error
		healthPayloads, err = fakeIntake.GetAgentHealth()
		assert.NoError(t, err)
		assert.NotEmpty(t, healthPayloads)
	}, 2*time.Minute, 10*time.Second, "Health report not received in FakeIntake within timeout")

	// Verify the docker permission issue is present and has correct metadata
	latestReport := healthPayloads[len(healthPayloads)-1]
	require.NotNil(suite.T(), latestReport.HealthReport, "Health report is nil")

	dockerIssue := findIssue(suite.T(), latestReport, expectedIssueID)
	require.NotNil(suite.T(), dockerIssue, "Docker permission issue should be detected")

	// Verify issue metadata
	assert.Equal(suite.T(), "docker-file-tailing-disabled", dockerIssue.Id)
	assert.Equal(suite.T(), "docker_file_tailing_disabled", dockerIssue.IssueName)
	assert.Equal(suite.T(), "permissions", dockerIssue.Category)
	assert.Equal(suite.T(), "medium", dockerIssue.Severity)
	assert.Equal(suite.T(), "logs-agent", dockerIssue.Location)
	assert.Equal(suite.T(), "logs", dockerIssue.Source)

	// Verify issue tags
	assert.Contains(suite.T(), dockerIssue.Tags, "docker")
	assert.Contains(suite.T(), dockerIssue.Tags, "logs")
	assert.Contains(suite.T(), dockerIssue.Tags, "permissions")
	assert.Contains(suite.T(), dockerIssue.Tags, "file-tailing")

	// Verify remediation is provided
	assert.NotNil(suite.T(), dockerIssue.Remediation, "Remediation should be provided")
	assert.NotEmpty(suite.T(), dockerIssue.Remediation.Summary, "Remediation summary should not be empty")
	assert.NotEmpty(suite.T(), dockerIssue.Remediation.Steps, "Remediation steps should not be empty")

	suite.T().Log("Phase 1 passed: docker permission issue detected with correct metadata")

	// =========================================================================
	// Phase 2: Verify persistence file has "new" state
	// =========================================================================
	suite.T().Log("=== Phase 2: Persistence file — state should be 'new' ===")

	persistedIss := suite.readPersistedIssue(dockerPermissionsCheckID)
	require.NotNil(suite.T(), persistedIss, "Docker permission issue should be persisted under check ID %s", dockerPermissionsCheckID)

	assert.Equal(suite.T(), expectedIssueID, persistedIss.IssueID)
	assert.NotEmpty(suite.T(), persistedIss.FirstSeen, "Issue should have first_seen timestamp")
	assert.NotEmpty(suite.T(), persistedIss.LastSeen, "Issue should have last_seen timestamp")
	assert.Contains(suite.T(), []issueState{issueStateNew, issueStateOngoing},
		persistedIss.State, "Initial issue state should be 'new' or 'ongoing'")

	initialFirstSeen := persistedIss.FirstSeen
	suite.T().Logf("Phase 2 passed: persisted issue state=%s, first_seen=%s", persistedIss.State, persistedIss.FirstSeen)

	// =========================================================================
	// Phase 3: Restart agent — issue should be loaded from disk and become "ongoing"
	// =========================================================================
	suite.T().Log("=== Phase 3: Restart agent — state should transition to 'ongoing' ===")

	// Flush fakeintake to distinguish pre/post restart payloads
	require.NoError(suite.T(), fakeIntake.FlushServerAndResetAggregators(), "Failed to flush fakeintake")

	host.MustExecute("sudo systemctl restart datadog-agent")
	suite.T().Log("Agent restarted")

	// Wait for agent to be ready after restart
	require.EventuallyWithT(suite.T(), func(t *assert.CollectT) {
		assert.True(t, agent.Client.IsReady(), "Agent should be ready after restart")
	}, 2*time.Minute, 10*time.Second, "Agent not ready after restart")

	// Wait for a new health report after restart
	require.EventuallyWithT(suite.T(), func(t *assert.CollectT) {
		payloads, err := fakeIntake.GetAgentHealth()
		if !assert.NoError(t, err) || !assert.NotEmpty(t, payloads, "Should receive health report after restart") {
			return
		}
		latest := payloads[len(payloads)-1]
		assert.NotNil(t, findIssue(suite.T(), latest, expectedIssueID),
			"Docker permission issue should still be present after restart")
	}, 2*time.Minute, 10*time.Second, "Health report with issue not received after restart")

	// Verify persistence file: state should be "ongoing" (loaded from disk + re-confirmed by check)
	persistedIss = suite.readPersistedIssue(dockerPermissionsCheckID)
	require.NotNil(suite.T(), persistedIss, "Issue should still be persisted after restart")

	assert.Equal(suite.T(), issueStateOngoing, persistedIss.State,
		"Issue should transition to 'ongoing' after being re-detected post-restart")
	assert.Equal(suite.T(), initialFirstSeen, persistedIss.FirstSeen,
		"first_seen should be preserved across restart")

	suite.T().Logf("Phase 3 passed: post-restart state=%s, first_seen=%s (preserved), last_seen=%s",
		persistedIss.State, persistedIss.FirstSeen, persistedIss.LastSeen)

	// =========================================================================
	// Phase 4: Fix the permission issue and verify resolution
	// =========================================================================
	suite.T().Log("=== Phase 4: Fix permissions — add dd-agent to docker group ===")

	// Add dd-agent to the docker group so it can access the Docker socket
	host.MustExecute("sudo usermod -aG docker dd-agent")
	suite.T().Log("Added dd-agent to docker group")

	// Verify the group was added
	groupOutput := host.MustExecute("groups dd-agent")
	suite.T().Logf("dd-agent groups: %s", groupOutput)
	assert.Contains(suite.T(), groupOutput, "docker", "dd-agent should be in docker group")

	// Restart agent so it picks up the new group membership
	// On restart, the docker socket check will run and find the socket is now reachable,
	// clearing the issue and marking it "resolved" in the persistence file.
	host.MustExecute("sudo systemctl restart datadog-agent")
	suite.T().Log("Agent restarted after permission fix")

	require.EventuallyWithT(suite.T(), func(t *assert.CollectT) {
		assert.True(t, agent.Client.IsReady(), "Agent should be ready after permission fix")
	}, 2*time.Minute, 10*time.Second, "Agent not ready after permission fix")

	// Wait for the persistence file to reflect the resolved state
	// The check runs immediately on start, detects no issue, and ClearIssuesForCheck sets state to "resolved"
	require.EventuallyWithT(suite.T(), func(t *assert.CollectT) {
		pi := suite.readPersistedIssueForAssert(t, dockerPermissionsCheckID)
		if !assert.NotNil(t, pi, "Issue should still be tracked in persistence file") {
			return
		}
		assert.Equal(t, issueStateResolved, pi.State, "Issue should be 'resolved' after permission fix")
	}, 2*time.Minute, 10*time.Second, "Issue not resolved in persistence file after permission fix")

	// Read the final state for detailed logging
	persistedIss = suite.readPersistedIssue(dockerPermissionsCheckID)
	require.NotNil(suite.T(), persistedIss)

	assert.Equal(suite.T(), issueStateResolved, persistedIss.State)
	assert.NotEmpty(suite.T(), persistedIss.ResolvedAt, "Issue should have resolved_at timestamp")
	assert.Equal(suite.T(), initialFirstSeen, persistedIss.FirstSeen,
		"first_seen should be preserved through the entire lifecycle")

	suite.T().Logf("Phase 4 passed: state=%s, resolved_at=%s, first_seen=%s (preserved)",
		persistedIss.State, persistedIss.ResolvedAt, persistedIss.FirstSeen)

	suite.T().Log("=== Full lifecycle test passed ===")
}

// ============================================================================
// Helper methods
// ============================================================================

// readPersistedIssue reads the persistence file from the remote host and returns the issue for the given check ID.
// Fails the test if the file cannot be read or parsed.
func (suite *dockerPermissionSuite) readPersistedIssue(checkID string) *persistedIssue {
	suite.T().Helper()

	var raw string
	require.EventuallyWithT(suite.T(), func(t *assert.CollectT) {
		var err error
		raw, err = suite.Env().RemoteHost.Execute("cat " + persistenceFilePath)
		assert.NoError(t, err, "Persistence file should exist")
		assert.NotEmpty(t, raw, "Persistence file should not be empty")
	}, 30*time.Second, 5*time.Second, "Persistence file not found")

	var state persistedState
	require.NoError(suite.T(), json.Unmarshal([]byte(raw), &state), "Persistence file should be valid JSON")
	return state.Issues[checkID]
}

// readPersistedIssueForAssert reads the persistence file and returns the issue for the given check ID.
// For use inside EventuallyWithT callbacks — returns nil if the file can't be read yet.
func (suite *dockerPermissionSuite) readPersistedIssueForAssert(t *assert.CollectT, checkID string) *persistedIssue {
	suite.T().Helper()

	raw, err := suite.Env().RemoteHost.Execute("cat " + persistenceFilePath)
	if !assert.NoError(t, err) || !assert.NotEmpty(t, raw) {
		return nil
	}

	var state persistedState
	if !assert.NoError(t, json.Unmarshal([]byte(raw), &state)) {
		return nil
	}
	return state.Issues[checkID]
}

// findIssue searches for an issue by ID in a health report payload.
// Returns nil if not found and logs all found issues for debugging.
func findIssue(t *testing.T, report *aggregator.AgentHealthPayload, issueID string) *healthplatform.Issue {
	t.Helper()

	if report.HealthReport == nil {
		return nil
	}

	for _, issue := range report.Issues {
		if issue.Id == issueID {
			return issue
		}
	}

	// Log all found issues for debugging
	var debugMsg strings.Builder
	debugMsg.WriteString(fmt.Sprintf("Issue '%s' not found. Found %d issues:", issueID, len(report.Issues)))
	count := 1
	for _, issue := range report.Issues {
		debugMsg.WriteString(fmt.Sprintf("\n  #%d: ID='%s', Category='%s', Tags=%v", count, issue.Id, issue.Category, issue.Tags))
		count++
	}
	t.Log(debugMsg.String())
	return nil
}
