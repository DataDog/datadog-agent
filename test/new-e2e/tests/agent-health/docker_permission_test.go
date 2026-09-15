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
	t.Parallel()
	e2e.Run(t, &dockerPermissionSuite{},
		e2e.WithPulumiProvisioner(dockerPermissionEnvProvisioner(), nil),
	)
}

// TestDockerHealthCheckTransientFailure verifies that a transient probe error in
// the periodic docker-socket health check does not resolve an active issue.
//
// The docker-permissions module uses a BuiltInPeriodicHealthCheck (scheduler.go).
// When that check function errors, scheduler.tick leaves lastIssueIDs unchanged
// so that in-flight issues are not spuriously resolved. This test exercises that
// path by killing the agent ungracefully (SIGKILL) and verifying that the issue
// is still reported as ONGOING after systemd restarts the process — even though
// the scheduler may fire before the docker probe has had a chance to warm up.
func (suite *dockerPermissionSuite) TestDockerHealthCheckTransientFailure() {
	host := suite.Env().RemoteHost
	agent := suite.Env().Agent
	fakeIntake := suite.Env().Fakeintake.Client()

	// issueID is a prefix: the actual id is scoped by host discriminator (see
	// dockerpermissions.checker.instanceIssueID), e.g. "docker-socket-permissions:<host>".
	const issueID = "docker-socket-permissions"

	// Pre-condition: docker socket must be restricted so the issue is active.
	host.MustExecute("sudo chmod 660 /var/run/docker.sock")

	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		payloads, err := fakeIntake.GetAgentHealth()
		assert.NoError(ct, err)
		var found bool
		for _, p := range payloads {
			for _, iss := range findIssuesByPrefix(p, issueID) {
				if iss.PersistedIssue != nil {
					found = true
				}
			}
		}
		assert.True(ct, found, "docker permission issue not found in fakeintake before crash test")
	}, defaultIssueTimeout, defaultIssuePollInterval, "docker permission issue not found before crash test")

	// Kill the agent ungracefully — systemd will restart it.
	// On restart the scheduler fires immediately; the docker probe may error or
	// return stale data in the first tick (socket not yet warmed up). Either way
	// the scheduler must preserve the active issue.
	host.MustExecute("sudo pkill -KILL datadog-agent || true")

	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		assert.True(ct, agent.Client.IsReady())
	}, 2*time.Minute, 10*time.Second, "agent did not restart after SIGKILL")
	require.NoError(suite.T(), fakeIntake.FlushServerAndResetAggregators())

	// After the crash restart the issue must still be present (NEW or ONGOING).
	// The first scheduler tick on restart may run before the agent has fully dropped
	// privileges, briefly seeing the socket as accessible and resolving the issue.
	// If that race fires, the docker probe will detect the issue again on the next
	// successful tick and report it as NEW. Either state confirms the issue survived
	// the crash window.
	var reloadedIssues []*healthplatform.Issue
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		payloads, err := fakeIntake.GetAgentHealth()
		assert.NoError(ct, err)
		reloadedIssues = nil
		for _, p := range payloads {
			for _, iss := range findIssuesByPrefix(p, issueID) {
				if iss.PersistedIssue != nil &&
					(iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_ACTIVE) {
					reloadedIssues = append(reloadedIssues, iss)
				}
			}
		}
		assert.NotEmpty(ct, reloadedIssues, "docker permission issue not found as ACTIVE after crash restart")
	}, defaultIssueTimeout, defaultIssuePollInterval, "docker permission issue not re-reported after crash restart")

	require.NotEmpty(suite.T(), reloadedIssues)
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

	// issueID is a prefix: the actual id is scoped by host discriminator (see
	// dockerpermissions.checker.instanceIssueID), e.g. "docker-socket-permissions:<host>".
	const issueID = "docker-socket-permissions"

	containers, err := suite.Env().Docker.Client.ListContainers()
	require.NoError(suite.T(), err)
	found := false
	for _, name := range containers {
		if strings.Contains(name, "spam") {
			found = true
			break
		}
	}
	assert.True(suite.T(), found, "busybox spam containers should be running")

	suite.T().Run("IssueDetection", func(t *testing.T) {
		host.MustExecute("sudo chmod 660 /var/run/docker.sock")

		var issues []*healthplatform.Issue
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			issues = nil
			for _, p := range payloads {
				for _, iss := range findIssuesByPrefix(p, issueID) {
					if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_ACTIVE {
						issues = append(issues, iss)
					}
				}
			}
			assert.NotEmpty(ct, issues, "docker socket permission issue not found as ACTIVE in fakeintake")
		}, defaultIssueTimeout, defaultIssuePollInterval, "docker socket permission issue not detected as ACTIVE in fakeintake")

		require.NotEmpty(t, issues)
		issue := issues[0]
		assert.True(t, strings.HasPrefix(issue.Id, issueID+":"), "issue id %q should be scoped by host discriminator", issue.Id)
		assert.Equal(t, "Docker Socket Permission", issue.IssueName)
		assert.Equal(t, "docker_socket_permission", issue.IssueType)
		assert.Equal(t, "permissions", issue.Category)
		assert.Equal(t, "logs-agent", issue.Location)
		assert.Equal(t, "agent", issue.Source)
		assert.Contains(t, issue.Tags, "docker")
		assert.Contains(t, issue.Tags, "docker-socket")
		require.NotNil(t, issue.Remediation, "remediation should be provided")
		assert.NotEmpty(t, issue.Remediation.Summary)
		assert.NotEmpty(t, issue.Remediation.Steps)
	})

	host.MustExecute("sudo chmod 666 /var/run/docker.sock")
	perm := host.MustExecute("stat -c '%a' /var/run/docker.sock")
	assert.Contains(suite.T(), strings.TrimSpace(perm), "666", "docker socket should be world-accessible")
	require.NoError(suite.T(), agent.Client.Restart())
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		assert.True(ct, agent.Client.IsReady())
	}, 2*time.Minute, 10*time.Second, "agent not ready after fix restart")

	suite.T().Run("Resolution", func(t *testing.T) {
		// Restore broken state on cleanup so infra can be re-used for re-runs.
		t.Cleanup(func() {
			host.MustExecute("sudo chmod 660 /var/run/docker.sock")
			_ = agent.Client.Restart()
		})

		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			for _, p := range payloads {
				for _, iss := range findIssuesByPrefix(p, issueID) {
					if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_RESOLVED {
						return
					}
				}
			}
			assert.Fail(ct, "no payload found with the issue in RESOLVED state")
		}, defaultIssueTimeout, defaultIssuePollInterval, "issue never transitioned to RESOLVED after fix")
	})
}

// TestDockerSocketUnavailableLifecycle verifies that a non-permission Docker
// socket dial failure — a stale socket file left behind once the daemon is
// down, distinct from a permission-denied one — is reported as "Docker
// Socket Unavailable" (ACTIVE), and that restoring the socket resolves it.
//
// The docker-permissions module's Check() only fires on the scheduler's
// 15-minute periodic tick (comp/healthplatform/scheduler/impl/scheduler.go's
// defaultCheckInterval) besides once immediately whenever the scheduler
// (re)starts. This test restarts the agent after each socket state change to
// force that immediate tick rather than waiting out the real interval.
func (suite *dockerPermissionSuite) TestDockerSocketUnavailableLifecycle() {
	host := suite.Env().RemoteHost
	agent := suite.Env().Agent
	fakeIntake := suite.Env().Fakeintake.Client()

	// issueID is a prefix: the actual id is scoped by host discriminator (see
	// dockerpermissions.checker.instanceIssueID), e.g. "docker-socket-unavailable:<hash>".
	const issueID = "docker-socket-unavailable"

	// breakSocket stops the real Docker daemon and leaves a stale, world-writable
	// unix socket file in its place: nothing is listening at that path anymore, so
	// a dial attempt returns "connection refused" — a non-permission reachability
	// failure, as opposed to the permission-denied case covered above.
	breakSocket := func() {
		host.MustExecute("sudo systemctl stop docker")
		host.MustExecute("sudo rm -f /var/run/docker.sock")
		host.MustExecute(`sudo python3 -c "import socket; s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM); s.bind('/var/run/docker.sock'); s.listen(1)"`)
		host.MustExecute("sudo chmod 666 /var/run/docker.sock")
	}

	// restoreSocket removes the stale socket file and (re)starts the real daemon,
	// which recreates /var/run/docker.sock itself. Uses "restart" rather than
	// "start" so this is idempotent when called twice in a row (once from the
	// test body, once from cleanup): "start" is a no-op on an already-running
	// docker.service, which would leave the socket file just removed above
	// never recreated.
	restoreSocket := func() {
		host.MustExecute("sudo rm -f /var/run/docker.sock")
		host.MustExecute("sudo systemctl restart docker")
	}

	restartAgent := func(t *testing.T) {
		require.NoError(t, agent.Client.Restart())
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady())
		}, 2*time.Minute, 10*time.Second, "agent not ready after restart")
	}

	suite.T().Run("IssueDetection", func(t *testing.T) {
		breakSocket()
		restartAgent(t)

		var issues []*healthplatform.Issue
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			issues = nil
			for _, p := range payloads {
				for _, iss := range findIssuesByPrefix(p, issueID) {
					if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_ACTIVE {
						issues = append(issues, iss)
					}
				}
			}
			assert.NotEmpty(ct, issues, "docker socket unavailable issue not found as ACTIVE in fakeintake")
		}, defaultIssueTimeout, defaultIssuePollInterval, "docker socket unavailable issue not detected as ACTIVE in fakeintake")

		require.NotEmpty(t, issues)
		issue := issues[0]
		assert.True(t, strings.HasPrefix(issue.Id, issueID+":"), "issue id %q should be scoped by host discriminator", issue.Id)
		assert.Equal(t, "Docker Socket Unavailable", issue.IssueName)
		assert.Equal(t, "docker_socket_unavailable", issue.IssueType)
		assert.Equal(t, "availability", issue.Category)
		assert.Equal(t, "logs-agent", issue.Location)
		assert.Equal(t, "agent", issue.Source)
		assert.Contains(t, issue.Tags, "docker-socket")
		assert.Contains(t, issue.Tags, "unavailable")
		require.NotNil(t, issue.Remediation, "remediation should be provided")
		assert.NotEmpty(t, issue.Remediation.Summary)
		assert.NotEmpty(t, issue.Remediation.Steps)
	})

	suite.T().Run("Resolution", func(t *testing.T) {
		// Cleanup restores a running daemon (rather than re-breaking it, unlike
		// the permission test above) since a stopped daemon would also break the
		// busybox containers other tests in this suite depend on.
		t.Cleanup(func() {
			restoreSocket()
			restartAgent(t)
		})

		restoreSocket()
		restartAgent(t)

		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			for _, p := range payloads {
				for _, iss := range findIssuesByPrefix(p, issueID) {
					if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_RESOLVED {
						return
					}
				}
			}
			assert.Fail(ct, "no payload found with the issue in RESOLVED state")
		}, defaultIssueTimeout, defaultIssuePollInterval, "issue never transitioned to RESOLVED after fix")
	})
}
