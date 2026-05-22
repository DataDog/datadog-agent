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

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
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

	RunHealthIssueLifecycle(suite.T(),
		HealthIssueTestCase{
			IssueName: "broken_check",
			// IssueID ends with "*" for prefix matching: the real ID includes a
			// runtime-generated hash suffix ("check-execution-failure:broken_check:<hash>").
			IssueID: "check-execution-failure:broken_check*",
			TriggerIssue: func(t *testing.T, h *components.RemoteHost) {
				writeCheckFile(t, h, brokenCheckPy)
			},
			FixIssue: func(t *testing.T, h *components.RemoteHost) {
				writeCheckFile(t, h, fixedCheckPy)
			},
			AssertMetadata: func(t *testing.T, issue *healthplatform.Issue) {
				assert.Equal(t, "check_execution_failure", issue.IssueName)
				assert.Equal(t, "check-execution", issue.Category)
				assert.Equal(t, "collector", issue.Source)
				assert.Contains(t, issue.Tags, "broken_check")
				require.NotNil(t, issue.Remediation, "remediation should be provided")
				assert.NotEmpty(t, issue.Remediation.Summary)
			},
		},
		agent, host, fi,
	)
}
