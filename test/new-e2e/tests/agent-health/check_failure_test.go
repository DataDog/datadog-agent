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

// healthPlatformAgentConfig is the shared base agent config; short forwarder interval reduces test latency.
//
//go:embed fixtures/agent_config.yaml
var healthPlatformAgentConfig string

const brokenCheckConf = `init_config:
instances:
  - {}
`

//go:embed fixtures/broken_check.py
var brokenCheckPy string

//go:embed fixtures/fixed_check.py
var fixedCheckPy string

type checkFailureSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestCheckFailureSuite runs the check failure health issue lifecycle test.
func TestCheckFailureSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &checkFailureSuite{},
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

// TestCheckFailureIssueLifecycle verifies that a check execution failure is detected as NEW
// and transitions to RESOLVED after the check is fixed. Cross-restart persistence is in TestResilienceSuite.
func (suite *checkFailureSuite) TestCheckFailureIssueLifecycle() {
	fakeIntake := suite.Env().FakeIntake.Client()

	const issuePrefix = "check-execution-failure:broken_check"

	suite.T().Run("IssueDetection", func(t *testing.T) {
		// Accept NEW or ONGOING: check may fail multiple times before the first egress tick.
		var issues []*healthplatform.Issue
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			issues = nil
			for _, p := range payloads {
				for _, iss := range findIssuesByPrefix(p, issuePrefix) {
					if iss.PersistedIssue != nil &&
						(iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_ACTIVE) {
						issues = append(issues, iss)
					}
				}
			}
			assert.NotEmpty(ct, issues, "check execution failure not found as ACTIVE in fakeintake")
		}, defaultIssueTimeout, defaultIssuePollInterval, "check execution failure not detected in fakeintake")

		require.NotEmpty(t, issues)
		issue := issues[0]
		assert.Equal(t, "Check Execution Failure", issue.IssueName)
		assert.Equal(t, "check-execution", issue.Category)
		assert.Equal(t, "collector", issue.Source)
		assert.Contains(t, issue.Tags, "broken_check")
		require.NotNil(t, issue.Remediation, "remediation should be provided")
		assert.NotEmpty(t, issue.Remediation.Summary)
	})

	suite.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(healthPlatformAgentConfig),
				agentparams.WithIntegration("broken_check.d", brokenCheckConf),
				agentparams.WithFile(
					"/etc/datadog-agent/checks.d/broken_check.py",
					fixedCheckPy,
					true,
				),
			),
		),
	))
	// Restart so the agent re-imports fixed_check.py (WithFile doesn't reload cached Python modules).
	agent := suite.Env().Agent
	require.NoError(suite.T(), agent.Client.Restart())
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		assert.True(ct, agent.Client.IsReady())
	}, 2*time.Minute, 10*time.Second, "agent not ready after fix")
	require.NoError(suite.T(), fakeIntake.FlushServerAndResetAggregators())

	suite.T().Run("Resolution", func(t *testing.T) {
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			for _, p := range payloads {
				for _, iss := range findIssuesByPrefix(p, issuePrefix) {
					if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_RESOLVED {
						return
					}
				}
			}
			assert.Fail(ct, "no payload found with the issue in RESOLVED state")
		}, defaultIssueTimeout, defaultIssuePollInterval, "issue never transitioned to RESOLVED after fix")
	})
}
