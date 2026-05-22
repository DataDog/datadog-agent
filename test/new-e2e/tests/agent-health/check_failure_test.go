// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	_ "embed"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

//go:embed fixtures/health_platform_agent_config.yaml
var healthPlatformAgentConfig string

// brokenCheckConf is the check configuration placed in conf.d to activate the check.
const brokenCheckConf = `
init_config:
instances:
  - {}
`

// brokenCheckPy is a Python check that always raises — deployed by the provisioner.
const brokenCheckPy = `
from datadog_checks.base import AgentCheck

class BrokenCheck(AgentCheck):
    def check(self, instance):
        raise Exception("synthetic failure for e2e health platform test")
`

// fixedCheckPy replaces brokenCheckPy in the Resolution phase.
const fixedCheckPy = `
from datadog_checks.base import AgentCheck

class BrokenCheck(AgentCheck):
    def check(self, instance):
        self.gauge("e2e.healthplatform.check_ok", 1)
`

// ============================================================================
// Environment definition
// ============================================================================

type checkFailureEnv struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Fakeintake *components.FakeIntake
}

func checkFailureEnvProvisioner() provisioners.PulumiEnvRunFunc[checkFailureEnv] {
	return func(ctx *pulumi.Context, env *checkFailureEnv) error {
		base := &baseEC2Env{
			RemoteHost: env.RemoteHost,
			Agent:      env.Agent,
			Fakeintake: env.Fakeintake,
		}
		return newBaseEC2Env(ctx, base, "checkfailurevm",
			agentparams.WithAgentConfig(healthPlatformAgentConfig),
			agentparams.WithIntegration("broken_check.d", brokenCheckConf),
			agentparams.WithFile(
				"/etc/datadog-agent/checks.d/broken_check.py",
				brokenCheckPy,
				true,
			),
		)
	}
}

var _ common.Diagnosable = (*checkFailureEnv)(nil)

func (e *checkFailureEnv) Diagnose(_ string) (string, error) {
	if e.Agent == nil {
		return "", errors.New("agent not initialized")
	}
	out := e.Agent.Client.Diagnose(agentclient.WithArgs([]string{"--include", healthIssueSuite}))
	return "==== agent diagnose health-issues ====\n" + out, nil
}

// ============================================================================
// Test suite
// ============================================================================

type checkFailureSuite struct {
	e2e.BaseSuite[checkFailureEnv]
}

// TestCheckFailureSuite runs the check failure health issue lifecycle test.
func TestCheckFailureSuite(t *testing.T) {
	e2e.Run(t, &checkFailureSuite{},
		e2e.WithPulumiProvisioner(checkFailureEnvProvisioner(), nil),
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
	fi := suite.Env().Fakeintake.Client()

	suite.T().Run("PreCondition", func(t *testing.T) {
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady())
		}, 2*time.Minute, 10*time.Second, "agent not ready")
	})

	RunHealthIssueLifecycle(suite.T(),
		HealthIssueTestCase{
			// IssueName is matched (substring) against the diagnose entry Name field.
			IssueName: "broken_check",
			// IssueID ends with "*" for prefix matching: the real ID includes a
			// runtime-generated hash suffix ("check-execution-failure:broken_check:<hash>").
			IssueID: "check-execution-failure:broken_check*",
			// TriggerIssue re-deploys the broken check after the Resolution cleanup.
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

// writeCheckFile writes content to /etc/datadog-agent/checks.d/broken_check.py.
// It writes to a temp path first (SFTP user, no sudo), then sudo-moves it into
// the protected directory with correct ownership.
func writeCheckFile(t *testing.T, h *components.RemoteHost, content string) {
	t.Helper()
	const (
		tmpPath   = "/tmp/broken_check_e2e.py"
		checkPath = "/etc/datadog-agent/checks.d/broken_check.py"
	)
	_, err := h.WriteFile(tmpPath, []byte(content))
	require.NoError(t, err, "failed to write check file to temp path")
	h.MustExecute(fmt.Sprintf("sudo mv %s %s && sudo chown dd-agent:dd-agent %s && sudo chmod 644 %s",
		tmpPath, checkPath, checkPath, checkPath))
}
