// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ntp

import (
	"testing"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnreachableBuildIssue(t *testing.T) {
	tests := []struct {
		name            string
		context         map[string]string
		expectedDescSub string
	}{
		{
			name: "fully populated context",
			context: map[string]string{
				"servers": "0.datadog.pool.ntp.org:123",
				"error":   "failed to get clock offset from any ntp host",
			},
			expectedDescSub: "could not reach any of the configured NTP servers (0.datadog.pool.ntp.org:123)",
		},
		{
			name:            "empty context defaults are applied",
			context:         map[string]string{},
			expectedDescSub: "could not reach any of the configured NTP servers (unknown)",
		},
		{
			name:            "nil context does not panic",
			context:         nil,
			expectedDescSub: "could not reach any of the configured NTP servers (unknown)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template := NewUnreachableIssue()
			issue, err := template.BuildIssue(tt.context)

			require.NoError(t, err)
			require.NotNil(t, issue)

			assert.Empty(t, issue.Id, "Id is set by the caller (ReportIssue), not by the template")
			assert.Equal(t, UnreachableIssueName, issue.IssueName)
			assert.Equal(t, "Datadog Agent Cannot Reach Any NTP Server", issue.Title)
			assert.Contains(t, issue.Description, tt.expectedDescSub)
			assert.Equal(t, "connectivity", issue.Category)
			assert.Equal(t, "system", issue.Location)
			assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, issue.Severity)
			assert.Equal(t, "ntp", issue.Source)

			require.NotNil(t, issue.Remediation)
			assert.NotEmpty(t, issue.Remediation.Summary)
			assert.NotEmpty(t, issue.Remediation.Steps)

			require.NotNil(t, issue.Extra)
			fields := issue.Extra.GetFields()
			assert.NotNil(t, fields["servers"])
			assert.NotNil(t, fields["error"])

			assert.Contains(t, issue.Tags, "ntp")
			assert.Contains(t, issue.Tags, "connectivity")
		})
	}
}
