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

// resilienceSuite tests the health platform's cross-restart persistence behaviour:
// an active issue must be reloaded from the on-disk store on agent restart, reported
// back to fakeintake as ONGOING, and retain its original first_seen timestamp.
//
// This concern is framework-level (not issue-specific), so it is tested once here
// rather than duplicated in every issue lifecycle test.
type resilienceSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestResilienceSuite runs the health platform resilience test.
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

// TestHealthPlatformResilience verifies that a health issue persists across an
// agent restart: the issue must re-appear in fakeintake with ONGOING state and
// the same first_seen timestamp as before the restart.
func (suite *resilienceSuite) TestHealthPlatformResilience() {
	agent := suite.Env().Agent
	fi := suite.Env().FakeIntake.Client()

	const (
		issueName = "broken_check"
		issueID   = "check-execution-failure:broken_check*"
	)

	// Wait for the issue to appear before the restart so we have a first_seen to compare.
	initialIssues := waitForIssuesInFakeintake(suite.T(), fi, issueID)
	require.NotEmpty(suite.T(), initialIssues)

	var firstSeen string
	for _, iss := range initialIssues {
		if iss.PersistedIssue != nil && firstSeen == "" {
			firstSeen = iss.PersistedIssue.FirstSeen
		}
	}

	require.NoError(suite.T(), fi.FlushServerAndResetAggregators())
	require.NoError(suite.T(), agent.Client.Restart())
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		assert.True(ct, agent.Client.IsReady())
	}, 2*time.Minute, 10*time.Second, "agent not ready after restart")

	// Issue must still appear in diagnose after restart (loaded from on-disk store).
	AssertIssueDetectedViaDiagnose(suite.T(), agent, issueName)

	// Issue must be re-reported to fakeintake as ONGOING with the same first_seen.
	reloadedIssues := waitForIssuesInFakeintake(suite.T(), fi, issueID)
	require.NotEmpty(suite.T(), reloadedIssues)
	require.NotNil(suite.T(), reloadedIssues[0].PersistedIssue)
	assert.Equal(suite.T(), healthplatform.IssueState_ISSUE_STATE_ONGOING, reloadedIssues[0].PersistedIssue.State)
	if firstSeen != "" {
		assert.Equal(suite.T(), firstSeen, reloadedIssues[0].PersistedIssue.FirstSeen,
			"first_seen must be preserved across restart")
	}
}
