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

// resilienceBaseConfig is the shared base agent config; short forwarder interval reduces test latency.
//
//go:embed fixtures/resilience_agent_config.yaml
var resilienceBaseConfig string

// resilienceBrokenConfig triggers the invalid-config health issue via a schema
// violation (agent_ipc.port must be an integer). The invalidconfig check only
// validates once at startup, so every state transition below goes through a
// restart. Schema violations are non-fatal by design — the agent falls back
// to defaults for the offending field rather than failing to start.
var resilienceBrokenConfig = resilienceBaseConfig + "agent_ipc:\n  port: not-a-number\n"

const resilienceIssueID = "invalid-config"

// resilienceSuite tests cross-restart persistence and issue recurrence (framework-level, not issue-specific).
// It uses the invalid-config issue purely as a trigger, since it's simple to flip on/off via a config field
// without needing Docker, Kubernetes, or root filesystem access.
type resilienceSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestResilienceSuite runs the health platform resilience tests.
func TestResilienceSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &resilienceSuite{},
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(resilienceBrokenConfig),
				),
			),
		)),
	)
}

// TestHealthPlatformResilience verifies that a health issue persists across a graceful restart,
// re-reported as ACTIVE with the same first_seen timestamp.
func (suite *resilienceSuite) TestHealthPlatformResilience() {
	agent := suite.Env().Agent
	fakeIntake := suite.Env().FakeIntake.Client()

	var initialIssues []*healthplatform.Issue
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		payloads, err := fakeIntake.GetAgentHealth()
		assert.NoError(ct, err)
		initialIssues = nil
		for _, p := range payloads {
			for _, iss := range findIssuesByPrefix(p, resilienceIssueID) {
				if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_ACTIVE {
					initialIssues = append(initialIssues, iss)
				}
			}
		}
		assert.NotEmpty(ct, initialIssues, "issue not found as ACTIVE in fakeintake")
	}, defaultIssueTimeout, defaultIssuePollInterval, "issue not detected in fakeintake")

	require.NotEmpty(suite.T(), initialIssues)
	var firstSeen string
	for _, iss := range initialIssues {
		if iss.PersistedIssue != nil && firstSeen == "" {
			firstSeen = iss.PersistedIssue.FirstSeen
		}
	}

	require.NoError(suite.T(), agent.Client.Restart())
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		assert.True(ct, agent.Client.IsReady())
	}, 2*time.Minute, 10*time.Second, "agent not ready after restart")
	require.NoError(suite.T(), fakeIntake.FlushServerAndResetAggregators())

	// After restart the issue must be re-reported as ACTIVE (loaded from on-disk store).
	var reloadedIssues []*healthplatform.Issue
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		payloads, err := fakeIntake.GetAgentHealth()
		assert.NoError(ct, err)
		reloadedIssues = nil
		for _, p := range payloads {
			for _, iss := range findIssuesByPrefix(p, resilienceIssueID) {
				if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_ACTIVE {
					reloadedIssues = append(reloadedIssues, iss)
				}
			}
		}
		assert.NotEmpty(ct, reloadedIssues, "issue not found as ACTIVE in fakeintake after restart")
	}, defaultIssueTimeout, defaultIssuePollInterval, "issue not re-reported as ACTIVE after restart")

	require.NotEmpty(suite.T(), reloadedIssues)
	require.NotNil(suite.T(), reloadedIssues[0].PersistedIssue)
	if firstSeen != "" {
		assert.Equal(suite.T(), firstSeen, reloadedIssues[0].PersistedIssue.FirstSeen,
			"first_seen must be preserved across restart")
	}
}

// TestHealthPlatformIssueRecurrence verifies that a resolved issue re-appears as NEW (not ONGOING)
// when the problem recurs, with first_seen reset to the new detection time.
func (suite *resilienceSuite) TestHealthPlatformIssueRecurrence() {
	fakeIntake := suite.Env().FakeIntake.Client()

	// Capture first_seen from the initial detection (issue may already be present
	// depending on test execution order within the suite).
	var originalFirstSeen string
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		payloads, err := fakeIntake.GetAgentHealth()
		assert.NoError(ct, err)
		for _, p := range payloads {
			for _, iss := range findIssuesByPrefix(p, resilienceIssueID) {
				if iss.PersistedIssue != nil && iss.PersistedIssue.FirstSeen != "" && originalFirstSeen == "" {
					originalFirstSeen = iss.PersistedIssue.FirstSeen
				}
			}
		}
		assert.NotEmpty(ct, originalFirstSeen, "issue not found in fakeintake")
	}, defaultIssueTimeout, defaultIssuePollInterval, "issue not detected in fakeintake before recurrence test")

	// Fix the issue.
	suite.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(resilienceBaseConfig),
			),
		),
	))
	agent := suite.Env().Agent
	require.NoError(suite.T(), agent.Client.Restart())
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		assert.True(ct, agent.Client.IsReady())
	}, 2*time.Minute, 10*time.Second, "agent not ready after fix")

	// Wait for the RESOLVED state to appear in fakeintake before re-breaking.
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		payloads, err := fakeIntake.GetAgentHealth()
		assert.NoError(ct, err)
		for _, p := range payloads {
			for _, iss := range findIssuesByPrefix(p, resilienceIssueID) {
				if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_RESOLVED {
					return
				}
			}
		}
		assert.Fail(ct, "no payload found with the issue in RESOLVED state")
	}, defaultIssueTimeout, defaultIssuePollInterval, "issue never transitioned to RESOLVED after fix")

	// Re-break: deploy the invalid config again.
	suite.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(resilienceBrokenConfig),
			),
		),
	))
	require.NoError(suite.T(), agent.Client.Restart())
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		assert.True(ct, agent.Client.IsReady())
	}, 2*time.Minute, 10*time.Second, "agent not ready after recurrence")
	require.NoError(suite.T(), fakeIntake.FlushServerAndResetAggregators())

	// Issue must reappear with a reset first_seen.
	var recurrentIssues []*healthplatform.Issue
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		payloads, err := fakeIntake.GetAgentHealth()
		assert.NoError(ct, err)
		recurrentIssues = nil
		for _, p := range payloads {
			for _, iss := range findIssuesByPrefix(p, resilienceIssueID) {
				if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_ACTIVE {
					recurrentIssues = append(recurrentIssues, iss)
				}
			}
		}
		assert.NotEmpty(ct, recurrentIssues, "re-broken issue not found as ACTIVE in fakeintake after recurrence")
	}, defaultIssueTimeout, defaultIssuePollInterval, "issue not re-detected after recurrence")

	require.NotEmpty(suite.T(), recurrentIssues)
	require.NotNil(suite.T(), recurrentIssues[0].PersistedIssue)
	if originalFirstSeen != "" {
		assert.NotEqual(suite.T(), originalFirstSeen, recurrentIssues[0].PersistedIssue.FirstSeen,
			"first_seen must be reset for a recurrent issue, not carried over from the previous occurrence")
	}
}
