// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gpuenvironment

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDRACDIIssue(t *testing.T) {
	tests := []struct {
		name          string
		context       map[string]string
		expectedTitle string
		expectedDesc  string
		expectedExtra map[string]string
	}{
		{
			name: "full context",
			context: map[string]string{
				DRACDIContextKeySearchedPaths:  "/host/var/run/cdi,/var/run/cdi",
				DRACDIContextKeyError:          "no such file or directory",
				DRACDIContextKeyContainerCount: "3",
			},
			expectedTitle: "GPU monitoring cannot find the CDI specs of NVIDIA DRA claims (searched /host/var/run/cdi,/var/run/cdi)",
			expectedDesc:  "3 container(s) use NVIDIA DRA GPU claims",
			expectedExtra: map[string]string{
				DRACDIContextKeySearchedPaths:  "/host/var/run/cdi,/var/run/cdi",
				DRACDIContextKeyError:          "no such file or directory",
				DRACDIContextKeyContainerCount: "3",
			},
		},
		{
			name:          "nil context uses defaults",
			context:       nil,
			expectedTitle: "GPU monitoring cannot find the CDI specs of NVIDIA DRA claims (searched unknown)",
			expectedDesc:  "unknown container(s)",
			expectedExtra: map[string]string{
				DRACDIContextKeySearchedPaths:  "unknown",
				DRACDIContextKeyError:          "unknown error",
				DRACDIContextKeyContainerCount: "unknown",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue, err := NewDRACDIIssue().BuildIssue(tt.context)
			require.NoError(t, err)
			require.NotNil(t, issue)

			assert.Empty(t, issue.Id)
			assert.Equal(t, DRACDIIssueName, issue.IssueName)
			assert.Equal(t, DRACDIIssueType, issue.IssueType)
			assert.Equal(t, tt.expectedTitle, issue.Title)
			assert.Contains(t, issue.Description, tt.expectedDesc)
			assert.Equal(t, category, issue.Category)
			assert.Equal(t, location, issue.Location)
			assert.Equal(t, severity, issue.Severity)
			assert.Equal(t, source, issue.Source)

			require.NotNil(t, issue.Remediation)
			assert.NotEmpty(t, issue.Remediation.Summary)
			require.Len(t, issue.Remediation.Steps, 6)
			assert.True(t, strings.HasPrefix(issue.Remediation.Steps[0].Text, "Helm chart:"))
			assert.True(t, strings.HasPrefix(issue.Remediation.Steps[1].Text, "Datadog Operator:"))

			require.NotNil(t, issue.Extra)
			fields := issue.Extra.GetFields()
			for key, value := range tt.expectedExtra {
				assert.Equal(t, value, fields[key].GetStringValue(), "extra key %s", key)
			}
			assert.NotNil(t, fields["impact"])

			assert.Contains(t, issue.Tags, "gpu")
		})
	}
}
