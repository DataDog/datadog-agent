// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
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

// resilienceSuite tests the health platform's cross-restart persistence and
// recurrence behaviours. These are framework-level concerns (not issue-specific),
// so they are covered once here rather than duplicated in every lifecycle test.
type resilienceSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestResilienceSuite runs the health platform resilience tests.
// It reuses the broken_check fixtures from check_failure_test.go (same package).
func TestResilienceSuite(t *testing.T) {
	e2e.Run(t, &resilienceSuite{},
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(healthPlatformAgentConfig),
					agentparams.WithIntegration("broken_check.d", brokenCheckConf),
					agentparams.WithFile(
						"/etc/datadog-agent/checks.d/broken_check.py",
						brokenCheckPy,
						true,
					),
				),
			),
		)),
	)
}

// TestHealthPlatformResilience verifies that a health issue persists across a
// graceful agent restart: after restart the issue must be re-reported to
// fakeintake as ONGOING with the same first_seen timestamp as before.
func (suite *resilienceSuite) TestHealthPlatformResilience() {
	agent := suite.Env().Agent
	fakeIntake := suite.Env().FakeIntake.Client()

	const issuePrefix = "check-execution-failure:broken_check"

	// Wait for the initial detection (NEW or ONGOING) so we have a first_seen to compare.
	// The check may fail multiple times before the first egress tick, so the state may
	// already be ONGOING by the time the report arrives in fakeintake.
	var initialIssues []*healthplatform.Issue
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		payloads, err := fakeIntake.GetAgentHealth()
		assert.NoError(ct, err)
		initialIssues = nil
		for _, p := range payloads {
			for _, iss := range findIssuesByPrefix(p, issuePrefix) {
				if iss.PersistedIssue != nil &&
					(iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_NEW ||
						iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_ONGOING) {
					initialIssues = append(initialIssues, iss)
				}
			}
		}
		assert.NotEmpty(ct, initialIssues, "issue not found as NEW or ONGOING in fakeintake")
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

	// After restart the issue must be re-reported as ONGOING (loaded from on-disk store).
	var reloadedIssues []*healthplatform.Issue
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		payloads, err := fakeIntake.GetAgentHealth()
		assert.NoError(ct, err)
		reloadedIssues = nil
		for _, p := range payloads {
			for _, iss := range findIssuesByPrefix(p, issuePrefix) {
				if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_ONGOING {
					reloadedIssues = append(reloadedIssues, iss)
				}
			}
		}
		assert.NotEmpty(ct, reloadedIssues, "issue not found as ONGOING in fakeintake after restart")
	}, defaultIssueTimeout, defaultIssuePollInterval, "issue not re-reported as ONGOING after restart")

	require.NotEmpty(suite.T(), reloadedIssues)
	require.NotNil(suite.T(), reloadedIssues[0].PersistedIssue)
	if firstSeen != "" {
		assert.Equal(suite.T(), firstSeen, reloadedIssues[0].PersistedIssue.FirstSeen,
			"first_seen must be preserved across restart")
	}
}

// TestHealthPlatformIssueRecurrence verifies that a previously resolved issue
// re-appears as NEW (not ONGOING) when the underlying problem recurs, and that
// first_seen is reset to the new detection time.
//
// This tests store.go's state-transition logic: a resolved issue ID that is
// re-reported reverts to NEW and clears its original first_seen/last_seen.
func (suite *resilienceSuite) TestHealthPlatformIssueRecurrence() {
	fakeIntake := suite.Env().FakeIntake.Client()

	const issuePrefix = "check-execution-failure:broken_check"

	// Capture first_seen from the initial detection (issue may be NEW or ONGOING
	// depending on test execution order within the suite).
	var originalFirstSeen string
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		payloads, err := fakeIntake.GetAgentHealth()
		assert.NoError(ct, err)
		for _, p := range payloads {
			for _, iss := range findIssuesByPrefix(p, issuePrefix) {
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
				agentparams.WithAgentConfig(healthPlatformAgentConfig),
				agentparams.WithIntegration("broken_check.d", brokenCheckConf),
				agentparams.WithFile("/etc/datadog-agent/checks.d/broken_check.py", fixedCheckPy, true),
			),
		),
	))
	require.NoError(suite.T(), fakeIntake.FlushServerAndResetAggregators())

	// Verify the issue is resolved or no longer reported.
	require.Never(suite.T(), func() bool {
		payloads, _ := fakeIntake.GetAgentHealth()
		for _, p := range payloads {
			for _, iss := range findIssuesByPrefix(p, issuePrefix) {
				if iss.PersistedIssue == nil || iss.PersistedIssue.State != healthplatform.IssueState_ISSUE_STATE_RESOLVED {
					return true
				}
			}
		}
		return false
	}, defaultIssueAbsenceWindow, defaultIssuePollInterval, "issue not resolved after fix")

	// Re-break: deploy the broken check again.
	suite.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(healthPlatformAgentConfig),
				agentparams.WithIntegration("broken_check.d", brokenCheckConf),
				agentparams.WithFile("/etc/datadog-agent/checks.d/broken_check.py", brokenCheckPy, true),
			),
		),
	))
	require.NoError(suite.T(), fakeIntake.FlushServerAndResetAggregators())

	// Issue must reappear with a reset first_seen. Accept NEW or ONGOING: the check may
	// fail multiple times before the first egress tick, so the state may already be
	// ONGOING in fakeintake. The first_seen assertion below is the authoritative check.
	var recurrentIssues []*healthplatform.Issue
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		payloads, err := fakeIntake.GetAgentHealth()
		assert.NoError(ct, err)
		recurrentIssues = nil
		for _, p := range payloads {
			for _, iss := range findIssuesByPrefix(p, issuePrefix) {
				if iss.PersistedIssue != nil &&
					(iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_NEW ||
						iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_ONGOING) {
					recurrentIssues = append(recurrentIssues, iss)
				}
			}
		}
		assert.NotEmpty(ct, recurrentIssues, "re-broken issue not found in fakeintake after recurrence")
	}, defaultIssueTimeout, defaultIssuePollInterval, "issue not re-detected after recurrence")

	require.NotEmpty(suite.T(), recurrentIssues)
	require.NotNil(suite.T(), recurrentIssues[0].PersistedIssue)
	if originalFirstSeen != "" {
		assert.NotEqual(suite.T(), originalFirstSeen, recurrentIssues[0].PersistedIssue.FirstSeen,
			"first_seen must be reset for a recurrent issue, not carried over from the previous occurrence")
	}
}
