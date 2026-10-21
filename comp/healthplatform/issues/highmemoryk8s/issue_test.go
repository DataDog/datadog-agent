// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package highmemoryk8s

import (
	"testing"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildIssue(t *testing.T) {
	for _, tc := range []struct {
		name            string
		context         map[string]string
		wantTitle       string
		wantDescription string
		wantUsage       string
	}{
		{
			name: "full context",
			context: map[string]string{
				contextKeyUsagePercent:     "93",
				contextKeyThresholdPercent: "90",
				contextKeyWindow:           "5 minutes",
			},
			wantTitle:       "Datadog Agent memory usage is at 93% of its container limit",
			wantDescription: "used 93% of its container memory limit on average over the last 5 minutes, above the 90% threshold",
			wantUsage:       "93",
		},
		{
			name:            "empty context keys",
			context:         map[string]string{contextKeyUsagePercent: ""},
			wantTitle:       "Datadog Agent memory usage is at unknown% of its container limit",
			wantDescription: "used unknown% of its container memory limit on average over the last unknown, above the unknown% threshold",
			wantUsage:       unknownValue,
		},
		{
			name:            "nil context",
			context:         nil,
			wantTitle:       "Datadog Agent memory usage is at unknown% of its container limit",
			wantDescription: "above the unknown% threshold",
			wantUsage:       unknownValue,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issue, err := HighMemoryK8sIssue{}.BuildIssue(tc.context)
			require.NoError(t, err)

			assert.Empty(t, issue.Id)
			assert.Equal(t, IssueName, issue.IssueName)
			assert.Equal(t, IssueType, issue.IssueType)
			assert.Equal(t, tc.wantTitle, issue.Title)
			assert.Contains(t, issue.Description, tc.wantDescription)
			assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, issue.Severity)
			assert.NotEmpty(t, issue.Category)
			assert.NotEmpty(t, issue.Location)
			assert.NotEmpty(t, issue.Source)
			assert.Contains(t, issue.Tags, "memory")

			require.NotNil(t, issue.Remediation)
			assert.NotEmpty(t, issue.Remediation.Summary)
			assert.Equal(t, 4, len(issue.Remediation.Steps))
			for i, step := range issue.Remediation.Steps {
				assert.Equal(t, int32(i+1), step.Order)
			}

			require.NotNil(t, issue.Extra)
			fields := issue.Extra.GetFields()
			assert.Equal(t, tc.wantUsage, fields[contextKeyUsagePercent].GetStringValue())
			assert.NotNil(t, fields[contextKeyThresholdPercent])
			assert.NotNil(t, fields[contextKeyWindow])
			assert.NotNil(t, fields["impact"])
		})
	}
}
