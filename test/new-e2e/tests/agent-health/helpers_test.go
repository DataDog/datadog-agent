// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const (
	// defaultIssueTimeout is the default timeout for issue detection.
	defaultIssueTimeout = 2 * time.Minute
	// defaultIssuePollInterval is the poll cadence for EventuallyWithT / Never.
	defaultIssuePollInterval = 10 * time.Second
	// defaultIssueAbsenceWindow is how long we verify an issue stays absent/resolved.
	defaultIssueAbsenceWindow = 45 * time.Second
)

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
// Useful for issue types where the ID includes a runtime-generated hash suffix.
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

// waitForIssueInState polls fakeintake until at least one issue matching issueID
// is found with the specified state, then returns all matching issues.
// If issueID ends with "*" it is treated as a prefix match.
func waitForIssueInState(t *testing.T, fi *fakeintakeclient.Client, issueID string, state healthplatform.IssueState) []*healthplatform.Issue {
	t.Helper()
	prefix, usePrefix := strings.CutSuffix(issueID, "*")
	var found []*healthplatform.Issue
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		payloads, err := fi.GetAgentHealth()
		assert.NoError(ct, err)
		found = nil
		for _, p := range payloads {
			var candidates []*healthplatform.Issue
			if usePrefix {
				candidates = findIssuesByPrefix(p, prefix)
			} else {
				candidates = findIssuesByID(t, p, issueID)
			}
			for _, iss := range candidates {
				if iss.PersistedIssue != nil && iss.PersistedIssue.State == state {
					found = append(found, iss)
				}
			}
		}
		assert.NotEmptyf(ct, found, "issue %q with state %v not found in fakeintake", issueID, state)
	}, defaultIssueTimeout, defaultIssuePollInterval,
		"issue %q with state %v not found in fakeintake within timeout", issueID, state)
	return found
}

// assertIssueResolvedOrAbsent verifies that after a fix and agent restart the
// issue either stops being reported to fakeintake, or is only reported with
// RESOLVED state. Fails if any non-resolved payload for the issue arrives
// within defaultIssueAbsenceWindow.
func assertIssueResolvedOrAbsent(t *testing.T, fi *fakeintakeclient.Client, issueID string) {
	t.Helper()
	prefix, usePrefix := strings.CutSuffix(issueID, "*")
	require.Never(t, func() bool {
		payloads, err := fi.GetAgentHealth()
		if err != nil {
			return false
		}
		for _, p := range payloads {
			var candidates []*healthplatform.Issue
			if usePrefix {
				candidates = findIssuesByPrefix(p, prefix)
			} else {
				candidates = findIssuesByID(t, p, issueID)
			}
			for _, iss := range candidates {
				if iss.PersistedIssue == nil || iss.PersistedIssue.State != healthplatform.IssueState_ISSUE_STATE_RESOLVED {
					return true
				}
			}
		}
		return false
	}, defaultIssueAbsenceWindow, defaultIssuePollInterval,
		"issue %q still reported as non-resolved after fix", issueID)
}

// ============================================================================
// Shared test utilities
// ============================================================================

// writeCheckFile writes a Python custom check to the agent's checks.d directory.
// It writes to a world-writable temp path via SFTP, then uses sudo to move the
// file into the protected directory and set ownership.
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
