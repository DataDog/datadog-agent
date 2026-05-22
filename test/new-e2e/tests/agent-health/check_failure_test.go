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

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

// fixedCheckPy is the replacement check that succeeds, used in the Resolution phase.
// The failing version (brokenCheckPy) is defined in provisioner.go alongside the provisioner.
const fixedCheckPy = `
from datadog_checks.base import AgentCheck

class BrokenCheck(AgentCheck):
    def check(self, instance):
        self.gauge("e2e.healthplatform.check_ok", 1)
`

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
			// IssueName is matched (substring) against the diagnose entry Name field,
			// which the checkfailure template populates with the check name.
			IssueName: "broken_check",
			IssueID:   "check-execution-failure:broken_check",

			// TriggerIssue re-deploys the broken check after the Resolution cleanup
			// restores the working version, so the environment is ready for a re-run.
			TriggerIssue: func(t *testing.T, h *components.RemoteHost) {
				writeCheckFile(t, h, brokenCheckPy)
			},

			// FixIssue replaces the failing check with one that succeeds.
			FixIssue: func(t *testing.T, h *components.RemoteHost) {
				writeCheckFile(t, h, fixedCheckPy)
			},

			// AssertMetadata validates the fakeintake payload metadata fields.
			AssertMetadata: func(t *testing.T, issue *healthplatform.Issue) {
				assert.Equal(t, "check-execution-failure", issue.IssueName)
				assert.Equal(t, "checks", issue.Category)
				assert.Equal(t, "agent", issue.Source)
				assert.Contains(t, issue.Tags, "check:broken_check")
				require.NotNil(t, issue.Remediation, "remediation should be provided")
				assert.NotEmpty(t, issue.Remediation.Summary)
			},
		},
		agent,
		host,
		fi,
	)
}

// writeCheckFile writes content to the broken_check.py location using WriteFile,
// then fixes ownership so dd-agent can read it.
func writeCheckFile(t *testing.T, h *components.RemoteHost, content string) {
	t.Helper()
	const checkPath = "/etc/datadog-agent/checks.d/broken_check.py"
	_, err := h.WriteFile(checkPath, []byte(content))
	require.NoError(t, err, "failed to write check file")
	h.MustExecute("sudo chown dd-agent:dd-agent " + checkPath)
}
