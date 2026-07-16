// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ntp

import (
	"strings"
	"testing"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDriftBuildIssue(t *testing.T) {
	tests := []struct {
		name            string
		context         map[string]string
		expectedDescSub string
	}{
		{
			name: "fully populated context",
			context: map[string]string{
				"drift":     "+2m30s",
				"ntpServer": "0.datadog.pool.ntp.org:123",
				"threshold": "1m0s",
			},
			expectedDescSub: "drifting from NTP reference time by +2m30s, which exceeds the 1m0s threshold",
		},
		{
			name:            "empty context defaults are applied",
			context:         map[string]string{},
			expectedDescSub: "drifting from NTP reference time by unknown, which exceeds the unknown threshold",
		},
		{
			name:            "nil context does not panic",
			context:         nil,
			expectedDescSub: "drifting from NTP reference time by unknown, which exceeds the unknown threshold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template := NewDriftIssue()
			issue, err := template.BuildIssue(tt.context)

			require.NoError(t, err)
			require.NotNil(t, issue)

			assert.Empty(t, issue.Id, "Id is set by the caller (ReportIssue), not by the template")
			assert.Equal(t, DriftIssueName, issue.IssueName)
			assert.Equal(t, "System Clock Drift Detected", issue.Title)
			assert.Contains(t, issue.Description, tt.expectedDescSub)
			assert.Equal(t, "integration", issue.Category)
			assert.Equal(t, "system", issue.Location)
			assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, issue.Severity)
			assert.Equal(t, "ntp", issue.Source)

			require.NotNil(t, issue.Remediation)
			assert.NotEmpty(t, issue.Remediation.Summary)
			assert.NotEmpty(t, issue.Remediation.Steps)

			require.NotNil(t, issue.Extra)
			fields := issue.Extra.GetFields()
			assert.NotNil(t, fields["drift"])
			assert.NotNil(t, fields["ntp_server"])
			assert.NotNil(t, fields["threshold"])

			assert.Contains(t, issue.Tags, "ntp")
			assert.Contains(t, issue.Tags, "clock-drift")
		})
	}
}

func TestDriftRemediationSteps(t *testing.T) {
	t.Run("windows", func(t *testing.T) {
		steps := driftRemediationSteps("windows")
		require.NotEmpty(t, steps)
		assert.True(t, containsSubstring(steps, "w32tm"), "expected a Windows-specific remediation step")
	})

	t.Run("linux", func(t *testing.T) {
		steps := driftRemediationSteps("linux")
		require.NotEmpty(t, steps)
		assert.True(t, containsSubstring(steps, "chronyc"), "expected a chrony remediation step")
	})

	t.Run("darwin falls back to the default branch", func(t *testing.T) {
		steps := driftRemediationSteps("darwin")
		assert.Equal(t, driftRemediationSteps("linux"), steps)
	})
}

func containsSubstring(steps []*healthplatform.RemediationStep, substr string) bool {
	for _, s := range steps {
		if strings.Contains(s.Text, substr) {
			return true
		}
	}
	return false
}
