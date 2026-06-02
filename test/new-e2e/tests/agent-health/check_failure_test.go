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

//go:embed fixtures/health_platform_agent_config.yaml
var healthPlatformAgentConfig string

//go:embed fixtures/broken_check.yaml
var brokenCheckConf string

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

// TestCheckFailureIssueLifecycle verifies the full lifecycle of a check execution
// failure health issue:
//
//  1. IssueDetection  – a custom Python check that always raises is detected via
//     `agent diagnose --include health-issues` and confirmed in fakeintake.
//  2. RestartResilience – the issue persists as ONGOING after an agent restart.
//  3. Resolution – replacing the failing check with a working version and
//     restarting the agent makes the issue disappear from diagnose.
func (suite *checkFailureSuite) TestCheckFailureIssueLifecycle() {
	host := suite.Env().RemoteHost
	agent := suite.Env().Agent
	fi := suite.Env().FakeIntake.Client()

	const (
		issueName = "broken_check"
		issueID   = "check-execution-failure:broken_check*"
	)

	var initialFirstSeen string

	// =========================================================================
	// Phase 1: Issue Detection
	// =========================================================================
	suite.T().Run("IssueDetection", func(t *testing.T) {
		// broken_check.py is deployed by the provisioner; agent is ready at suite start.
		AssertIssueDetectedViaDiagnose(t, agent, issueName)

		issues := waitForIssuesInFakeintake(t, fi, issueID)
		require.NotEmpty(t, issues)
		issue := issues[0]
		assert.Equal(t, "check_execution_failure", issue.IssueName)
		assert.Equal(t, "check-execution", issue.Category)
		assert.Equal(t, "collector", issue.Source)
		assert.Contains(t, issue.Tags, "broken_check")
		require.NotNil(t, issue.Remediation, "remediation should be provided")
		assert.NotEmpty(t, issue.Remediation.Summary)

		for _, iss := range issues {
			if iss.PersistedIssue != nil && initialFirstSeen == "" {
				initialFirstSeen = iss.PersistedIssue.FirstSeen
			}
		}
	})

	// =========================================================================
	// Phase 2: Restart Resilience
	// =========================================================================
	suite.T().Run("RestartResilience", func(t *testing.T) {
		require.NoError(t, fi.FlushServerAndResetAggregators())
		require.NoError(t, agent.Client.Restart())
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady())
		}, 2*time.Minute, 10*time.Second, "agent not ready after restart")

		AssertIssueDetectedViaDiagnose(t, agent, issueName)

		if initialFirstSeen != "" {
			issues := waitForIssuesInFakeintake(t, fi, issueID)
			require.NotEmpty(t, issues)
			require.NotNil(t, issues[0].PersistedIssue)
			assert.Equal(t, healthplatform.IssueState_ISSUE_STATE_ONGOING, issues[0].PersistedIssue.State)
			assert.Equal(t, initialFirstSeen, issues[0].PersistedIssue.FirstSeen, "first_seen should be preserved across restart")
		}
	})

	// =========================================================================
	// Phase 3: Resolution
	// =========================================================================
	suite.T().Run("Resolution", func(t *testing.T) {
		writeCheckFile(t, host, fixedCheckPy)
		require.NoError(t, fi.FlushServerAndResetAggregators())
		require.NoError(t, agent.Client.Restart())
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady())
		}, 2*time.Minute, 10*time.Second, "agent not ready after fix restart")

		AssertIssueAbsentViaDiagnose(t, agent, issueName)
	})
}
