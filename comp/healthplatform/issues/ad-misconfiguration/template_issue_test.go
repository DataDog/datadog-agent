// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package admisconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTemplateIssue(t *testing.T) {
	tests := []struct {
		name              string
		context           map[string]string
		expectedTitle     string
		expectedDescSub   string
		expectedStepCount int
	}{
		{
			name: "template resolution error",
			context: map[string]string{
				"entityName":   "postgres (docker://abc123)",
				"errorMessage": "failed to get extra info for service docker://abc123, skipping config - extra config \"dbinstanceidentifier\" is not supported",
				"errorSource":  "template_resolution",
			},
			expectedTitle:     "Autodiscovery Template Resolution Error on 'postgres (docker://abc123)'",
			expectedDescSub:   "template resolution error",
			expectedStepCount: 2,
		},
		{
			name:              "empty context uses defaults",
			context:           map[string]string{},
			expectedTitle:     "Autodiscovery Template Resolution Error on 'unknown'",
			expectedDescSub:   failedMsg,
			expectedStepCount: 2,
		},
		{
			name:              "nil context uses defaults",
			context:           nil,
			expectedTitle:     "Autodiscovery Template Resolution Error on 'unknown'",
			expectedDescSub:   failedMsg,
			expectedStepCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template := NewADTemplateIssue()
			issue, err := template.BuildIssue(tt.context)

			require.NoError(t, err)
			require.NotNil(t, issue)

			assert.Empty(t, issue.Id, "Id is set by the caller (ReportIssue), not by the template")
			assert.Equal(t, templateIssueName, issue.IssueName)
			assert.Equal(t, templateIssueType, issue.IssueType)
			assert.Equal(t, tt.expectedTitle, issue.Title)
			assert.Contains(t, issue.Description, tt.expectedDescSub)
			assert.Equal(t, category, issue.Category)
			assert.Equal(t, severity, issue.Severity)
			assert.Equal(t, source, issue.Source)
			assert.Equal(t, location, issue.Location)

			require.NotNil(t, issue.Remediation)
			assert.Equal(t, tt.expectedStepCount, len(issue.Remediation.Steps))

			require.NotNil(t, issue.Extra)
			fields := issue.Extra.GetFields()
			assert.NotNil(t, fields["entity_name"])
			assert.NotNil(t, fields["error_message"])
			assert.NotNil(t, fields["impact"])

			assert.Contains(t, issue.Tags, "autodiscovery")
		})
	}
}
