// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package agenthealth contains E2E tests for the agent health reporting functionality.
package agenthealth

import (
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

const (
	expectedIssueID = "docker-file-tailing-disabled"
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
//  1. IssueDetection: agent detects the issue and reports it to fakeintake with correct metadata
//  2. RestartResilience: after agent restart, issue transitions to "ongoing" in the payload
//  3. Resolution: after fixing permissions and restarting, the issue disappears from fakeintake
func (suite *dockerPermissionSuite) TestDockerPermissionIssueLifecycle() {
	host := suite.Env().RemoteHost
	agent := suite.Env().Agent
	fakeIntake := suite.Env().Fakeintake.Client()

	// initialFirstSeen is captured in IssueDetection and verified in subsequent phases.
	var initialFirstSeen string

	// =========================================================================
	// Phase 1: Issue Detection
	// =========================================================================
	suite.T().Run("IssueDetection", func(t *testing.T) {
		// Verify agent is running
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady(), "Agent should be ready")
		}, 2*time.Minute, 10*time.Second, "Agent not ready")

		// Verify containers are running
		output := host.MustExecute("docker ps --format '{{.Names}}' | grep spam")
		t.Logf("Running containers: %s", output)
		assert.Contains(t, output, "spam", "Busybox containers should be running")

		// Wait for health report to be sent to fakeintake
		var latestReport *aggregator.AgentHealthPayload
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			assert.NotEmpty(ct, payloads, "Health report not received in FakeIntake")
			if len(payloads) > 0 {
				latestReport = payloads[len(payloads)-1]
			}
		}, 2*time.Minute, 10*time.Second, "Health report not received in FakeIntake within timeout")

		require.NotNil(t, latestReport.HealthReport, "Health report is nil")

		dockerIssue := findIssue(t, latestReport, expectedIssueID)
		require.NotNil(t, dockerIssue, "Docker permission issue should be detected")

		// Verify issue metadata
		assert.Equal(t, "docker-file-tailing-disabled", dockerIssue.Id)
		assert.Equal(t, "docker_file_tailing_disabled", dockerIssue.IssueName)
		assert.Equal(t, "permissions", dockerIssue.Category)
		assert.Equal(t, "medium", dockerIssue.Severity)
		assert.Equal(t, "logs-agent", dockerIssue.Location)
		assert.Equal(t, "logs", dockerIssue.Source)

		// Verify issue tags
		assert.Contains(t, dockerIssue.Tags, "docker")
		assert.Contains(t, dockerIssue.Tags, "logs")
		assert.Contains(t, dockerIssue.Tags, "permissions")
		assert.Contains(t, dockerIssue.Tags, "file-tailing")

		// Verify remediation is provided
		assert.NotNil(t, dockerIssue.Remediation, "Remediation should be provided")
		assert.NotEmpty(t, dockerIssue.Remediation.Summary, "Remediation summary should not be empty")
		assert.NotEmpty(t, dockerIssue.Remediation.Steps, "Remediation steps should not be empty")

		// Verify the PersistedIssue is populated in the payload — this is the black-box view of persistence
		require.NotNil(t, dockerIssue.PersistedIssue, "PersistedIssue should be populated in the health report payload")
		assert.Contains(t,
			[]healthplatform.IssueState{healthplatform.IssueState_ISSUE_STATE_NEW, healthplatform.IssueState_ISSUE_STATE_ONGOING},
			dockerIssue.PersistedIssue.State, "PersistedIssue state should be NEW or ONGOING")
		assert.NotEmpty(t, dockerIssue.PersistedIssue.FirstSeen, "PersistedIssue should have first_seen")
		assert.NotEmpty(t, dockerIssue.PersistedIssue.LastSeen, "PersistedIssue should have last_seen")

		// Capture first_seen for later phases
		initialFirstSeen = dockerIssue.PersistedIssue.FirstSeen
		t.Logf("Phase 1 passed: docker permission issue detected, first_seen=%s", initialFirstSeen)
	})

	// =========================================================================
	// Phase 2: Restart Resilience
	// =========================================================================
	suite.T().Run("RestartResilience", func(t *testing.T) {
		// Flush fakeintake to distinguish pre/post restart payloads
		require.NoError(t, fakeIntake.FlushServerAndResetAggregators(), "Failed to flush fakeintake")

		host.MustExecute("sudo systemctl restart datadog-agent")
		t.Log("Agent restarted")

		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady(), "Agent should be ready after restart")
		}, 2*time.Minute, 10*time.Second, "Agent not ready after restart")

		// Wait for a health report after restart with the issue still present and ONGOING state
		var postRestartIssue *healthplatform.Issue
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			if !assert.NoError(ct, err) || !assert.NotEmpty(ct, payloads, "Should receive health report after restart") {
				return
			}
			latest := payloads[len(payloads)-1]
			postRestartIssue = findIssue(t, latest, expectedIssueID)
			assert.NotNil(ct, postRestartIssue, "Docker permission issue should still be present after restart")
		}, 2*time.Minute, 10*time.Second, "Health report with issue not received after restart")

		require.NotNil(t, postRestartIssue.PersistedIssue, "PersistedIssue should be populated after restart")
		assert.Equal(t, healthplatform.IssueState_ISSUE_STATE_ONGOING, postRestartIssue.PersistedIssue.State,
			"PersistedIssue in payload should be ONGOING after restart")
		assert.Equal(t, initialFirstSeen, postRestartIssue.PersistedIssue.FirstSeen,
			"PersistedIssue first_seen in payload should be preserved across restart")

		t.Logf("Phase 2 passed: post-restart state=ONGOING, first_seen=%s (preserved)", initialFirstSeen)
	})

	// =========================================================================
	// Phase 3: Resolution
	// =========================================================================
	suite.T().Run("Resolution", func(t *testing.T) {
		// Cleanup: restore the environment by removing dd-agent from docker group and restarting
		// the agent. This ensures consecutive dev-mode runs start from the same initial state.
		t.Cleanup(func() {
			host.Execute("sudo gpasswd -d dd-agent docker || true")
			host.Execute("sudo systemctl restart datadog-agent")
		})

		// Add dd-agent to the docker group so it can access the Docker socket
		host.MustExecute("sudo usermod -aG docker dd-agent")
		t.Log("Added dd-agent to docker group")

		groupOutput := host.MustExecute("groups dd-agent")
		t.Logf("dd-agent groups: %s", groupOutput)
		assert.Contains(t, groupOutput, "docker", "dd-agent should be in docker group")

		// Flush fakeintake to get fresh post-restart payloads only
		require.NoError(t, fakeIntake.FlushServerAndResetAggregators(), "Failed to flush fakeintake before resolution restart")

		// Restart agent so it picks up the new group membership and re-runs the check
		host.MustExecute("sudo systemctl restart datadog-agent")
		t.Log("Agent restarted after permission fix")

		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady(), "Agent should be ready after permission fix")
		}, 2*time.Minute, 10*time.Second, "Agent not ready after permission fix")

		// Wait for a health report that no longer contains the docker permission issue
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			if !assert.NoError(ct, err) || !assert.NotEmpty(ct, payloads, "Should receive health report after permission fix") {
				return
			}
			latest := payloads[len(payloads)-1]
			issue := findIssue(t, latest, expectedIssueID)
			assert.Nil(ct, issue, "Docker permission issue should be resolved and absent from health report")
		}, 2*time.Minute, 10*time.Second, "Docker permission issue still present in health report after permission fix")

		t.Log("Phase 3 passed: docker permission issue resolved and absent from health report")
	})

	suite.T().Log("=== Full lifecycle test passed ===")
}

// ============================================================================
// Helper methods
// ============================================================================

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
