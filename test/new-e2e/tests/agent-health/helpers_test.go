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
// diagnose helpers
// ============================================================================

// runHealthDiagnose calls `agent diagnose --include health-issues --json` and
// returns the parsed agentclient.DiagnoseResult. The types (DiagnoseResult,
// DiagnoseRun, DiagnoseEntry) live in the agentclient package so they can be
// shared across all e2e test packages.
func runHealthDiagnose(t testing.TB, agent *components.RemoteHostAgent) agentclient.DiagnoseResult {
	t.Helper()
	raw := agent.Client.Diagnose(agentclient.WithArgs([]string{"--include", healthIssueSuite, "--json"}))
	var out agentclient.DiagnoseResult
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

// findDiagnosesByName searches all runs and returns all entries whose Name
// contains issueName (case-sensitive substring match).
func findDiagnosesByName(out agentclient.DiagnoseResult, issueName string) []*agentclient.DiagnoseEntry {
	var results []*agentclient.DiagnoseEntry
	for r := range out.Runs {
		for i := range out.Runs[r].Diagnoses {
			if strings.Contains(out.Runs[r].Diagnoses[i].Name, issueName) {
				results = append(results, &out.Runs[r].Diagnoses[i])
			}
		}
	}
	return results
}

// AssertIssueDetectedViaDiagnose polls `agent diagnose --include health-issues` until
// an entry matching issueName appears with a non-passing status (Fail or Warning).
func AssertIssueDetectedViaDiagnose(t *testing.T, agent *components.RemoteHostAgent, issueName string) {
	t.Helper()
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		out := runHealthDiagnose(t, agent)
		totalEntries := 0
		for _, r := range out.Runs {
			totalEntries += len(r.Diagnoses)
		}
		ds := findDiagnosesByName(out, issueName)
		if !assert.NotEmptyf(ct, ds, "health issue %q not found in diagnose (have %d entries across %d runs)",
			issueName, totalEntries, len(out.Runs)) {
			t.Logf("diagnose runs: %+v", out.Runs)
			return
		}
		for _, d := range ds {
			assert.NotEqualf(ct, "Pass", d.Status, "health issue %q should not be passing", issueName)
		}
	}, defaultIssueTimeout, defaultIssuePollInterval, "health issue %q not detected via diagnose within timeout", issueName)
}

// AssertIssueAbsentViaDiagnose verifies that no health diagnose entry matching issueName
// appears within defaultIssueAbsenceWindow.
func AssertIssueAbsentViaDiagnose(t *testing.T, agent *components.RemoteHostAgent, issueName string) {
	t.Helper()
	require.Never(t, func() bool {
		out := runHealthDiagnose(t, agent)
		return len(findDiagnosesByName(out, issueName)) > 0
	}, defaultIssueAbsenceWindow, defaultIssuePollInterval,
		"health issue %q appeared in diagnose output after fix", issueName)
}

// ============================================================================
// fakeintake helpers
// ============================================================================

// findIssuesByID searches for all issues with the given exact ID in a fakeintake health report payload.
func findIssuesByID(t testing.TB, report *aggregator.AgentHealthPayload, issueID string) []*healthplatform.Issue {
	t.Helper()
	if report == nil || report.HealthReport == nil {
		return nil
	}
	var results []*healthplatform.Issue
	for id, issue := range report.Issues {
		if id == issueID {
			results = append(results, issue)
		}
	}
	if len(results) == 0 {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("issue %q not found; have %d issues:", issueID, len(report.Issues)))
		for id, iss := range report.Issues {
			sb.WriteString(fmt.Sprintf("\n  id=%q title=%q", id, iss.GetTitle()))
		}
		t.Log(sb.String())
	}
	return results
}

// findIssuesByPrefix searches for all issues whose ID starts with prefix.
// Useful for issue types where the ID includes a runtime-generated hash suffix
// (e.g. "check-execution-failure:broken_check:a1b2c3d4").
func findIssuesByPrefix(report *aggregator.AgentHealthPayload, prefix string) []*healthplatform.Issue {
	if report == nil || report.HealthReport == nil {
		return nil
	}
	var results []*healthplatform.Issue
	for id, issue := range report.Issues {
		if strings.HasPrefix(id, prefix) {
			results = append(results, issue)
		}
	}
	return results
}

// waitForIssuesInFakeintake polls fakeintake until at least one issue matching issueID is found,
// then returns all matching issues. Fails the test on timeout.
// If issueID ends with "*" it is treated as a prefix match (useful for check-failure IDs
// that include a runtime hash suffix).
func waitForIssuesInFakeintake(t *testing.T, fi *fakeintakeclient.Client, issueID string) []*healthplatform.Issue {
	t.Helper()
	prefix, usePrefix := strings.CutSuffix(issueID, "*")
	var found []*healthplatform.Issue
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		payloads, err := fi.GetAgentHealth()
		assert.NoError(ct, err)
		for _, p := range payloads {
			if usePrefix {
				found = append(found, findIssuesByPrefix(p, prefix)...)
			} else {
				found = append(found, findIssuesByID(t, p, issueID)...)
			}
		}
		assert.NotEmpty(ct, found, "issue with id/prefix %q not found in fakeintake", issueID)
	}, defaultIssueTimeout, defaultIssuePollInterval, "issue %q not found in fakeintake within timeout", issueID)
	return found
}

// ============================================================================
// Shared test utilities
// ============================================================================

// writeCheckFile writes a Python custom check to the agent's checks.d directory.
// It writes to a world-writable temp path via SFTP, then uses sudo to move the
// file into the protected directory and set ownership. This helper is shared
// across all health platform e2e tests that exercise check-failure scenarios.
func writeCheckFile(t *testing.T, h *components.RemoteHost, content string) {
	t.Helper()
	const (
		tmpPath   = "/tmp/hp_e2e_check.py"
		checkPath = "/etc/datadog-agent/checks.d/broken_check.py"
	)
	_, err := h.WriteFile(tmpPath, []byte(content))
	require.NoError(t, err, "failed to write check file to temp path")
	h.MustExecute(fmt.Sprintf("sudo mv %s %s && sudo chown dd-agent:dd-agent %s && sudo chmod 644 %s",
		tmpPath, checkPath, checkPath, checkPath))
}
