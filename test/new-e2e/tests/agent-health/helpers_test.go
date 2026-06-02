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

	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

const (
	// defaultIssueTimeout is the default timeout for issue detection.
	defaultIssueTimeout = 2 * time.Minute
	// defaultIssuePollInterval is the poll cadence for EventuallyWithT / Never.
	defaultIssuePollInterval = 10 * time.Second
	// defaultIssueAbsenceWindow is how long we verify an issue stays absent/resolved.
	defaultIssueAbsenceWindow = 45 * time.Second
)

// findIssuesByID returns all issues with the given exact ID from a fakeintake payload.
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

// findIssuesByPrefix returns all issues whose ID starts with prefix from a fakeintake payload.
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
