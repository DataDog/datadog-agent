// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

const healthPlatformAgentConfig = `health_platform:
  enabled: true
  forwarder:
    interval: 30s
`

const brokenCheckConf = `init_config:
instances:
  - {}
`

//go:embed fixtures/broken_check.py
var brokenCheckPy string

//go:embed fixtures/fixed_check.py
var fixedCheckPy string

// ============================================================================
// Test suite
// ============================================================================

type checkFailureSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestCheckFailureSuite runs the check failure health issue lifecycle test.
func TestCheckFailureSuite(t *testing.T) {
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

// TestCheckFailureIssueLifecycle verifies that a check execution failure is
// detected in fakeintake as NEW and that replacing the failing check with a
// working version causes the issue to stop being reported (or be reported as
// RESOLVED).
//
// Cross-restart persistence is tested separately in TestResilienceSuite.
func (suite *checkFailureSuite) TestCheckFailureIssueLifecycle() {
	fakeIntake := suite.Env().FakeIntake.Client()

	const issuePrefix = "check-execution-failure:broken_check"

	suite.T().Run("IssueDetection", func(t *testing.T) {
		// broken_check.py is deployed by the provisioner; agent is ready at suite start.
		var issues []*healthplatform.Issue
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			issues = nil
			for _, p := range payloads {
				for _, iss := range findIssuesByPrefix(p, issuePrefix) {
					if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_NEW {
						issues = append(issues, iss)
					}
				}
			}
			assert.NotEmpty(ct, issues, "check execution failure not found as NEW in fakeintake")
		}, defaultIssueTimeout, defaultIssuePollInterval, "check execution failure not detected as NEW in fakeintake")

		require.NotEmpty(t, issues)
		issue := issues[0]
		assert.Equal(t, "check_execution_failure", issue.IssueName)
		assert.Equal(t, "check-execution", issue.Category)
		assert.Equal(t, "collector", issue.Source)
		assert.Contains(t, issue.Tags, "broken_check")
		require.NotNil(t, issue.Remediation, "remediation should be provided")
		assert.NotEmpty(t, issue.Remediation.Summary)
	})

	suite.T().Run("Resolution", func(t *testing.T) {
		require.NoError(t, fakeIntake.FlushServerAndResetAggregators())
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

		require.Never(t, func() bool {
			payloads, _ := fakeIntake.GetAgentHealth()
			for _, p := range payloads {
				for _, iss := range findIssuesByPrefix(p, issuePrefix) {
					if iss.PersistedIssue == nil || iss.PersistedIssue.State != healthplatform.IssueState_ISSUE_STATE_RESOLVED {
						return true
					}
				}
			}
			return false
		}, defaultIssueAbsenceWindow, defaultIssuePollInterval, "issue still reported as non-resolved after fix")
	})
}
