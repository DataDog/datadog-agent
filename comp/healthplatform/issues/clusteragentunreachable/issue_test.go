// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package clusteragentunreachable

import (
	"testing"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
)

func TestBuildIssue(t *testing.T) {
	tests := []struct {
		name            string
		context         map[string]string
		wantTitle       string
		wantDescription []string
		wantSteps       int
		wantTLSStep     bool
	}{
		{
			name: "connection refused",
			context: map[string]string{
				contextKeyError:        "dial tcp 10.0.0.1:5005: connect: connection refused",
				contextKeyFailingSince: "2026-10-20T12:00:00Z",
				contextKeyHostname:     "node-1",
			},
			wantTitle:       "Agent on 'node-1' cannot reach the Cluster Agent",
			wantDescription: []string{"node-1", "2026-10-20T12:00:00Z", "connection refused", "cluster checks"},
			wantSteps:       4,
		},
		{
			name: "tls failure adds tls step",
			context: map[string]string{
				contextKeyError:    "x509: certificate signed by unknown authority",
				contextKeyHostname: "node-1",
			},
			wantTitle:       "Agent on 'node-1' cannot reach the Cluster Agent",
			wantDescription: []string{"x509"},
			wantSteps:       5,
			wantTLSStep:     true,
		},
		{
			name:            "empty context",
			context:         map[string]string{},
			wantTitle:       "Agent on 'unknown' cannot reach the Cluster Agent",
			wantDescription: []string{"unknown error", "since unknown"},
			wantSteps:       4,
		},
		{
			name:      "nil context",
			context:   nil,
			wantTitle: "Agent on 'unknown' cannot reach the Cluster Agent",
			wantSteps: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue, err := ClusterAgentUnreachableIssue{}.BuildIssue(tt.context)
			require.NoError(t, err)

			assert.Empty(t, issue.Id)
			assert.Equal(t, IssueName, issue.IssueName)
			assert.Equal(t, IssueType, issue.IssueType)
			assert.Equal(t, tt.wantTitle, issue.Title)
			for _, sub := range tt.wantDescription {
				assert.Contains(t, issue.Description, sub)
			}
			assert.Equal(t, "connectivity", issue.Category)
			assert.Equal(t, "cluster-agent-client", issue.Location)
			assert.Equal(t, "agent", issue.Source)
			assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, issue.Severity)
			assert.ElementsMatch(t, []string{"cluster-agent", "connectivity"}, issue.Tags)

			require.NotNil(t, issue.Remediation)
			assert.NotEmpty(t, issue.Remediation.Summary)
			require.Len(t, issue.Remediation.Steps, tt.wantSteps)
			for i, step := range issue.Remediation.Steps {
				assert.Equal(t, int32(i+1), step.Order)
			}
			assert.Contains(t, issue.Remediation.Steps[0].Text, "agent status")
			assert.Contains(t, issue.Remediation.Steps[len(issue.Remediation.Steps)-1].Text, docsURL)
			if tt.wantTLSStep {
				assert.Contains(t, issue.Remediation.Steps[2].Text, "TLS settings")
			}

			require.NotNil(t, issue.Extra)
			fields := issue.Extra.GetFields()
			for _, key := range []string{contextKeyError, contextKeyFailingSince, contextKeyHostname, "impact"} {
				assert.NotNil(t, fields[key], key)
			}
		})
	}
}

func TestNewModule(t *testing.T) {
	m := NewModule(issues.ModuleDeps{})
	assert.Equal(t, IssueName, m.IssueName())
	assert.Equal(t, IssueType, m.IssueType())
	assert.Nil(t, m.BuiltInStartupHealthCheck())

	check := m.BuiltInPeriodicHealthCheck()
	require.NotNil(t, check)
	assert.Equal(t, checkSource, check.Source)
	assert.Equal(t, checkInterval, check.Interval)
	require.NotNil(t, check.Fn)
}
