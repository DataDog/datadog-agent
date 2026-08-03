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

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

const (
	// defaultIssueTimeout is the default timeout for issue detection and resolution.
	defaultIssueTimeout = 2 * time.Minute
	// defaultIssuePollInterval is the poll cadence for EventuallyWithT.
	defaultIssuePollInterval = 10 * time.Second
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
