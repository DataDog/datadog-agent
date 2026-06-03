// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

// ============================================================================
// Environment definition
// ============================================================================

type dockerPermissionEnv struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Fakeintake *components.FakeIntake
	Docker     *components.RemoteHostDocker
}

// ============================================================================
// Test suite
// ============================================================================

type dockerPermissionSuite struct {
	e2e.BaseSuite[dockerPermissionEnv]
}

// TestDockerPermissionSuite runs the docker permission health check test.
func TestDockerPermissionSuite(t *testing.T) {
	e2e.Run(t, &dockerPermissionSuite{},
		e2e.WithPulumiProvisioner(dockerPermissionEnvProvisioner(), nil),
	)
}

// TestDockerPermissionIssueLifecycle verifies that restricting the docker socket
// permissions triggers the health issue as NEW in fakeintake, and that restoring
// them causes the issue to stop being reported (or be reported as RESOLVED).
//
// Cross-restart persistence is tested separately in TestResilienceSuite.
func (suite *dockerPermissionSuite) TestDockerPermissionIssueLifecycle() {
	host := suite.Env().RemoteHost
	agent := suite.Env().Agent
	fakeIntake := suite.Env().Fakeintake.Client()

	const issueID = "docker-socket-permissions"

	suite.T().Run("PreCondition", func(t *testing.T) {
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady())
		}, 2*time.Minute, 10*time.Second, "agent not ready")

		containers, err := suite.Env().Docker.Client.ListContainers()
		require.NoError(t, err)
		found := false
		for _, name := range containers {
			if strings.Contains(name, "spam") {
				found = true
				break
			}
		}
		assert.True(t, found, "busybox spam containers should be running")
	})

	suite.T().Run("IssueDetection", func(t *testing.T) {
		host.MustExecute("sudo chmod 660 /var/run/docker.sock")

		var issues []*healthplatform.Issue
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			issues = nil
			for _, p := range payloads {
				for _, iss := range findIssuesByID(t, p, issueID) {
					if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_NEW {
						issues = append(issues, iss)
					}
				}
			}
			assert.NotEmpty(ct, issues, "docker socket permission issue not found as NEW in fakeintake")
		}, defaultIssueTimeout, defaultIssuePollInterval, "docker socket permission issue not detected as NEW in fakeintake")

		require.NotEmpty(t, issues)
		issue := issues[0]
		assert.Equal(t, "docker-socket-permissions", issue.Id)
		assert.Equal(t, "docker_file_tailing_disabled", issue.IssueName)
		assert.Equal(t, "permissions", issue.Category)
		assert.Equal(t, "logs-agent", issue.Location)
		assert.Equal(t, "logs", issue.Source)
		assert.Contains(t, issue.Tags, "docker")
		assert.Contains(t, issue.Tags, "permissions")
		require.NotNil(t, issue.Remediation, "remediation should be provided")
		assert.NotEmpty(t, issue.Remediation.Summary)
		assert.NotEmpty(t, issue.Remediation.Steps)
	})

	suite.T().Run("Resolution", func(t *testing.T) {
		// Restore broken state on cleanup so infra can be re-used for re-runs.
		t.Cleanup(func() {
			host.MustExecute("sudo chmod 660 /var/run/docker.sock")
			_ = agent.Client.Restart()
		})

		host.MustExecute("sudo chmod 666 /var/run/docker.sock")
		perm := host.MustExecute("stat -c '%a' /var/run/docker.sock")
		assert.Contains(t, strings.TrimSpace(perm), "666", "docker socket should be world-accessible")

		require.NoError(t, fakeIntake.FlushServerAndResetAggregators())
		require.NoError(t, agent.Client.Restart())
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady())
		}, 2*time.Minute, 10*time.Second, "agent not ready after fix restart")

		require.Never(t, func() bool {
			payloads, _ := fakeIntake.GetAgentHealth()
			for _, p := range payloads {
				for _, iss := range findIssuesByID(t, p, issueID) {
					if iss.PersistedIssue == nil || iss.PersistedIssue.State != healthplatform.IssueState_ISSUE_STATE_RESOLVED {
						return true
					}
				}
			}
			return false
		}, defaultIssueAbsenceWindow, defaultIssuePollInterval, "issue still reported as non-resolved after fix")
	})
}
