// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const (
	// healthIssueSuite is the diagnose suite name for health platform issues.
	healthIssueSuite = "health-issues"

	// defaultIssueTimeout is the default timeout for issue detection / resolution.
	defaultIssueTimeout = 2 * time.Minute
	// defaultIssuePollInterval is the poll cadence for EventuallyWithT.
	defaultIssuePollInterval = 10 * time.Second
	// defaultIssueAbsenceWindow is how long we verify an issue stays absent.
	defaultIssueAbsenceWindow = 45 * time.Second
)

// ============================================================================
// diagnose JSON types
// ============================================================================

// diagnosisEntry is a single entry in the JSON output of `agent diagnose`.
type diagnosisEntry struct {
	Name     string `json:"Name"`
	Status   string `json:"Status"`
	Result   string `json:"Result"`
	Category string `json:"Category"`
}

// diagnoseOutput is the top-level structure returned by `agent diagnose --json`.
type diagnoseOutput struct {
	Diagnoses []diagnosisEntry `json:"diagnoses"`
}

// ============================================================================
// diagnose helpers
// ============================================================================

// runHealthDiagnose calls `agent diagnose --include health-issues --json` and returns
// the parsed output. It fails the test on JSON parse errors.
func runHealthDiagnose(t testing.TB, agent *components.RemoteHostAgent) diagnoseOutput {
	t.Helper()
	raw := agent.Client.Diagnose(agentclient.WithArgs([]string{"--include", healthIssueSuite, "--json"}))
	var out diagnoseOutput
	// The command may include a non-JSON preamble; find the first '{' to be safe.
	if start := strings.Index(raw, "{"); start >= 0 {
		raw = raw[start:]
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Logf("diagnose raw output: %s", raw)
		require.NoError(t, err, "failed to parse diagnose JSON output")
	}
	return out
}

// findDiagnosis returns the first entry whose Name contains issueName, or nil.
func findDiagnosis(out diagnoseOutput, issueName string) *diagnosisEntry {
	for i := range out.Diagnoses {
		if strings.Contains(out.Diagnoses[i].Name, issueName) {
			return &out.Diagnoses[i]
		}
	}
	return nil
}

// AssertIssueDetectedViaDiagnose polls `agent diagnose --include health-issues` until
// an entry matching issueName appears with a non-passing status (Fail or Warning).
func AssertIssueDetectedViaDiagnose(t *testing.T, agent *components.RemoteHostAgent, issueName string) {
	t.Helper()
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		out := runHealthDiagnose(t, agent)
		d := findDiagnosis(out, issueName)
		if !assert.NotNilf(ct, d, "health issue %q not found in diagnose (have %d entries)", issueName, len(out.Diagnoses)) {
			t.Logf("diagnose entries: %+v", out.Diagnoses)
			return
		}
		assert.NotEqualf(ct, "Pass", d.Status, "health issue %q should not be passing", issueName)
	}, defaultIssueTimeout, defaultIssuePollInterval, "health issue %q not detected via diagnose within timeout", issueName)
}

// AssertIssueAbsentViaDiagnose verifies that no health diagnose entry matching issueName
// appears within defaultIssueAbsenceWindow.
func AssertIssueAbsentViaDiagnose(t *testing.T, agent *components.RemoteHostAgent, issueName string) {
	t.Helper()
	require.Never(t, func() bool {
		out := runHealthDiagnose(t, agent)
		return findDiagnosis(out, issueName) != nil
	}, defaultIssueAbsenceWindow, defaultIssuePollInterval,
		"health issue %q appeared in diagnose output after fix", issueName)
}

// ============================================================================
// fakeintake helpers
// ============================================================================

// findIssue searches for an issue by ID in a fakeintake health report payload.
func findIssue(t testing.TB, report *aggregator.AgentHealthPayload, issueID string) *healthplatform.Issue {
	t.Helper()
	if report == nil || report.HealthReport == nil {
		return nil
	}
	for id, issue := range report.Issues {
		if id == issueID {
			return issue
		}
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("issue %q not found; have %d issues:", issueID, len(report.Issues)))
	for id, iss := range report.Issues {
		sb.WriteString(fmt.Sprintf("\n  id=%q title=%q", id, iss.GetTitle()))
	}
	t.Log(sb.String())
	return nil
}

// waitForIssueInFakeintake polls fakeintake until an issue with the given issueID is found,
// then returns it. Fails the test on timeout.
func waitForIssueInFakeintake(t *testing.T, fi *fakeintakeclient.Client, issueID string) *healthplatform.Issue {
	t.Helper()
	var found *healthplatform.Issue
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		payloads, err := fi.GetAgentHealth()
		assert.NoError(ct, err)
		for _, p := range payloads {
			if iss := findIssue(t, p, issueID); iss != nil {
				found = iss
			}
		}
		assert.NotNil(ct, found, "issue %q not found in fakeintake", issueID)
	}, defaultIssueTimeout, defaultIssuePollInterval, "issue %q not found in fakeintake within timeout", issueID)
	return found
}

// ============================================================================
// HealthIssueTestCase — standard lifecycle driver
// ============================================================================

// HealthIssueTestCase describes the lifecycle of a single health platform issue.
// Fill in the fields and call RunHealthIssueLifecycle to execute the standard
// 3-phase test: detect → persist across restart → resolve.
type HealthIssueTestCase struct {
	// IssueName is matched against diagnose output entry names (substring match).
	IssueName string
	// IssueID is the exact issue ID used to search fakeintake payloads.
	IssueID string
	// TriggerIssue puts the host into a state where the issue fires.
	// Called once before the agent is expected to detect the issue.
	// May be nil if the environment is pre-configured to trigger the issue.
	TriggerIssue func(t *testing.T, host *components.RemoteHost)
	// FixIssue reverses TriggerIssue so the issue resolves on the next agent start.
	FixIssue func(t *testing.T, host *components.RemoteHost)
	// AssertMetadata optionally validates issue metadata from the fakeintake payload
	// once the issue is first detected. May be nil.
	AssertMetadata func(t *testing.T, issue *healthplatform.Issue)
}

// RunHealthIssueLifecycle executes the 3-phase health issue lifecycle test.
//
// Phase 1 – IssueDetection: verify the issue appears in `agent diagnose` and optionally fakeintake.
// Phase 2 – RestartResilience: restart the agent; verify the issue persists.
// Phase 3 – Resolution: apply the fix, restart, verify the issue disappears from diagnose.
func RunHealthIssueLifecycle(
	t *testing.T,
	tc HealthIssueTestCase,
	agent *components.RemoteHostAgent,
	host *components.RemoteHost,
	fi *fakeintakeclient.Client, // may be nil if fakeintake is not available
) {
	t.Helper()

	var initialFirstSeen string

	// =========================================================================
	// Phase 1: Issue Detection
	// =========================================================================
	t.Run("IssueDetection", func(t *testing.T) {
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady())
		}, 2*time.Minute, 10*time.Second, "agent not ready")

		if tc.TriggerIssue != nil {
			tc.TriggerIssue(t, host)
		}

		// Primary check: diagnose command
		AssertIssueDetectedViaDiagnose(t, agent, tc.IssueName)
		t.Logf("Phase 1: %q detected via diagnose ✓", tc.IssueName)

		// Secondary check: fakeintake payload metadata
		if fi != nil && (tc.IssueID != "" || tc.AssertMetadata != nil) {
			issue := waitForIssueInFakeintake(t, fi, tc.IssueID)
			if tc.AssertMetadata != nil {
				tc.AssertMetadata(t, issue)
			}
			if issue.PersistedIssue != nil {
				initialFirstSeen = issue.PersistedIssue.FirstSeen
			}
		}
	})

	// =========================================================================
	// Phase 2: Restart Resilience
	// =========================================================================
	t.Run("RestartResilience", func(t *testing.T) {
		if fi != nil {
			require.NoError(t, fi.FlushServerAndResetAggregators())
		}
		require.NoError(t, agent.Client.Restart())
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady())
		}, 2*time.Minute, 10*time.Second, "agent not ready after restart")

		AssertIssueDetectedViaDiagnose(t, agent, tc.IssueName)
		t.Logf("Phase 2: %q persists after restart ✓", tc.IssueName)

		// Verify ONGOING state in fakeintake if we captured first_seen earlier
		if fi != nil && initialFirstSeen != "" {
			issue := waitForIssueInFakeintake(t, fi, tc.IssueID)
			require.NotNil(t, issue.PersistedIssue)
			assert.Equal(t, healthplatform.IssueState_ISSUE_STATE_ONGOING, issue.PersistedIssue.State)
			assert.Equal(t, initialFirstSeen, issue.PersistedIssue.FirstSeen, "first_seen should be preserved across restart")
		}
	})

	// =========================================================================
	// Phase 3: Resolution
	// =========================================================================
	t.Run("Resolution", func(t *testing.T) {
		// Re-trigger on cleanup so dev-mode infra is left in its original broken
		// state and the test can be re-run without manual intervention.
		t.Cleanup(func() {
			if tc.TriggerIssue != nil {
				tc.TriggerIssue(t, host)
			}
			_ = agent.Client.Restart()
		})

		require.NotNilf(t, tc.FixIssue, "FixIssue must be provided to test resolution")
		tc.FixIssue(t, host)

		if fi != nil {
			require.NoError(t, fi.FlushServerAndResetAggregators())
		}
		require.NoError(t, agent.Client.Restart())
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			assert.True(ct, agent.Client.IsReady())
		}, 2*time.Minute, 10*time.Second, "agent not ready after fix restart")

		AssertIssueAbsentViaDiagnose(t, agent, tc.IssueName)
		t.Logf("Phase 3: %q resolved — absent from diagnose ✓", tc.IssueName)
	})
}
