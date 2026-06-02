// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

//go:embed fixtures/ping_icmp_agent_config.yaml
var pingICMPAgentConfig string

const pingICMPIssueID = "ping-icmp-permissions"

type pingICMPPermissionSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestPingICMPPermissionSuite runs the ping ICMP socket permission health issue lifecycle test.
func TestPingICMPPermissionSuite(t *testing.T) {
	e2e.Run(t, &pingICMPPermissionSuite{},
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(pingICMPAgentConfig),
				),
			),
		)),
	)
}

// TestPingICMPPermissionIssueLifecycle tests the full lifecycle of the ping ICMP socket
// permission issue:
//
//  1. IssueDetection: the agent starts without CAP_NET_RAW; the startup check detects
//     that it cannot open a raw ICMP socket and reports the issue to fakeintake.
//  2. RestartResilience: after an agent restart the issue transitions to ONGOING and
//     first_seen is preserved.
//  3. Resolution: granting cap_net_raw+ep to the agent binary and restarting makes the
//     issue disappear from subsequent health reports.
func (suite *pingICMPPermissionSuite) TestPingICMPPermissionIssueLifecycle() {
	host := suite.Env().RemoteHost
	agent := suite.Env().Agent
	fakeIntake := suite.Env().FakeIntake.Client()

	var initialFirstSeen string

	// =========================================================================
	// Phase 1: Issue Detection
	// The agent runs as dd-agent without CAP_NET_RAW by default.  The startup
	// health check tries to open AF_INET/SOCK_RAW/IPPROTO_ICMP and gets EPERM,
	// which is reported as the ping-icmp-permissions issue.
	// =========================================================================
	suite.T().Run("IssueDetection", func(t *testing.T) {
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady(), "Agent should be ready")
		}, 2*time.Minute, 10*time.Second, "Agent not ready")

		var pingIssue *healthplatform.Issue
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			assert.NotEmpty(ct, payloads, "Health report not received in fakeintake")
			if len(payloads) > 0 {
				latest := payloads[len(payloads)-1]
				pingIssue = findIssue(t, latest, pingICMPIssueID)
				assert.NotNil(ct, pingIssue, "ping ICMP permission issue should be detected")
			}
		}, 2*time.Minute, 10*time.Second, "Ping ICMP permission issue not received in fakeintake")

		require.NotNil(t, pingIssue)
		assert.Equal(t, pingICMPIssueID, pingIssue.Id)
		assert.Equal(t, "ping_icmp_permissions", pingIssue.IssueName)
		assert.Equal(t, "permissions", pingIssue.Category)
		assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, pingIssue.Severity)
		assert.Equal(t, "ping", pingIssue.Source)
		assert.NotNil(t, pingIssue.Remediation, "Remediation should be provided")
		assert.NotEmpty(t, pingIssue.Remediation.Steps, "Remediation steps should not be empty")

		require.NotNil(t, pingIssue.PersistedIssue, "PersistedIssue should be populated")
		assert.Contains(t,
			[]healthplatform.IssueState{
				healthplatform.IssueState_ISSUE_STATE_NEW,
				healthplatform.IssueState_ISSUE_STATE_ONGOING,
			},
			pingIssue.PersistedIssue.State)
		assert.NotEmpty(t, pingIssue.PersistedIssue.FirstSeen)

		initialFirstSeen = pingIssue.PersistedIssue.FirstSeen
		t.Logf("Phase 1 passed: ping ICMP permission issue detected, first_seen=%s", initialFirstSeen)
	})

	// =========================================================================
	// Phase 2: Restart Resilience
	// =========================================================================
	suite.T().Run("RestartResilience", func(t *testing.T) {
		require.NoError(t, fakeIntake.FlushServerAndResetAggregators(), "Failed to flush fakeintake")
		require.NoError(t, agent.Client.Restart(), "Failed to restart agent")

		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady(), "Agent should be ready after restart")
		}, 2*time.Minute, 10*time.Second, "Agent not ready after restart")

		var pingIssue *healthplatform.Issue
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			require.NoError(ct, err)
			require.NotEmpty(ct, payloads, "Should receive health report after restart")
			pingIssue = findIssue(t, payloads[len(payloads)-1], pingICMPIssueID)
			assert.NotNil(ct, pingIssue, "Ping ICMP issue should still be present after restart")
		}, 2*time.Minute, 10*time.Second, "Health report with issue not received after restart")

		require.NotNil(t, pingIssue.PersistedIssue, "PersistedIssue should be populated after restart")
		assert.Equal(t, healthplatform.IssueState_ISSUE_STATE_ONGOING, pingIssue.PersistedIssue.State,
			"Issue should be ONGOING after restart")
		assert.Equal(t, initialFirstSeen, pingIssue.PersistedIssue.FirstSeen,
			"first_seen should be preserved across restart")

		t.Logf("Phase 2 passed: issue persists as ONGOING after restart, first_seen=%s", initialFirstSeen)
	})

	// =========================================================================
	// Phase 3: Resolution
	// Grant cap_net_raw+ep to the agent binary so raw ICMP sockets succeed,
	// then restart.  The issue should not appear in any subsequent health report.
	// =========================================================================
	suite.T().Run("Resolution", func(t *testing.T) {
		// Cleanup: revoke the capability so the environment returns to its original
		// broken state, ensuring the test can be re-run without manual intervention.
		t.Cleanup(func() {
			host.Execute("sudo setcap -r /opt/datadog-agent/bin/agent/agent")
			_ = agent.Client.Restart()
		})

		host.MustExecute("sudo setcap cap_net_raw+ep /opt/datadog-agent/bin/agent/agent")
		t.Log("Granted CAP_NET_RAW to agent binary")

		require.NoError(t, fakeIntake.FlushServerAndResetAggregators(), "Failed to flush fakeintake before resolution restart")
		require.NoError(t, agent.Client.Restart(), "Failed to restart agent after granting capability")

		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady(), "Agent should be ready after capability grant")
		}, 2*time.Minute, 10*time.Second, "Agent not ready after capability grant")

		require.Never(t, func() bool {
			payloads, err := fakeIntake.GetAgentHealth()
			if err != nil {
				return false
			}
			for _, payload := range payloads {
				if findIssue(t, payload, pingICMPIssueID) != nil {
					return true
				}
			}
			return false
		}, 2*time.Minute, 10*time.Second, "Ping ICMP permission issue still present after granting CAP_NET_RAW")

		t.Log("Phase 3 passed: ping ICMP permission issue resolved and absent from health reports")
	})

	suite.T().Log("=== Full ping ICMP permission lifecycle test passed ===")
}
