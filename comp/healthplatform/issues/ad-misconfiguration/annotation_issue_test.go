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

func TestBuildAnnotationIssue(t *testing.T) {
	tests := []struct {
		name              string
		context           map[string]string
		expectedTitle     string
		expectedDescSub   string
		expectedStepCount int
		expectErr         bool
	}{
		{
			name: "pod annotation error",
			context: map[string]string{
				"entityName":   "default/my-pod (abc123)",
				"errorMessage": "annotation ad.datadoghq.com/nonmatching.check_names is invalid: nonmatching doesn't match a container identifier",
				"errorSource":  "pod_annotation",
			},
			expectedTitle:     "Autodiscovery Pod Annotation Misconfiguration on 'default/my-pod (abc123)'",
			expectedDescSub:   "pod annotation error",
			expectedStepCount: 4,
		},
		{
			name: "container label error",
			context: map[string]string{
				"entityName":   "docker://abc123",
				"errorMessage": "could not extract checks config: in checks: failed to unmarshal JSON",
				"errorSource":  "container_label",
			},
			expectedTitle:     "Autodiscovery Container Label Misconfiguration on 'docker://abc123'",
			expectedDescSub:   "container label error",
			expectedStepCount: 3,
		},
		{
			name:              "empty context defaults to pod annotation remediation",
			context:           map[string]string{},
			expectedTitle:     "Autodiscovery Pod Annotation Misconfiguration on 'unknown'",
			expectedDescSub:   failedMsg,
			expectedStepCount: 4,
		},
		{
			name:              "nil context defaults to pod annotation remediation",
			context:           nil,
			expectedTitle:     "Autodiscovery Pod Annotation Misconfiguration on 'unknown'",
			expectedDescSub:   failedMsg,
			expectedStepCount: 4,
		},
		{
			name: "malformed JSON error message with pod annotation source",
			context: map[string]string{
				"entityName":   "kube-system/nginx-pod (def456)",
				"errorMessage": "could not extract checks config: in checks: failed to unmarshal JSON",
				"errorSource":  "pod_annotation",
			},
			expectedTitle:     "Autodiscovery Pod Annotation Misconfiguration on 'kube-system/nginx-pod (def456)'",
			expectedDescSub:   "failed to unmarshal JSON",
			expectedStepCount: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template := NewADAnnotationIssue()
			issue, err := template.BuildIssue(tt.context)

			if tt.expectErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, issue)

			assert.Empty(t, issue.Id, "Id is set by the caller (ReportIssue), not by the template")
			assert.Equal(t, annotationIssueName, issue.IssueName)
			assert.Equal(t, annotationIssueType, issue.IssueType)
			assert.Equal(t, tt.expectedTitle, issue.Title)
			assert.Contains(t, issue.Description, tt.expectedDescSub)
			assert.Equal(t, category, issue.Category)
			assert.Equal(t, severity, issue.Severity)
			assert.Equal(t, source, issue.Source)
			assert.Equal(t, location, issue.Location)

			require.NotNil(t, issue.Remediation)
			assert.NotEmpty(t, issue.Remediation.Steps)
			assert.Equal(t, tt.expectedStepCount, len(issue.Remediation.Steps))

			require.NotNil(t, issue.Extra)
			fields := issue.Extra.GetFields()
			assert.NotNil(t, fields["entity_name"])
			assert.NotNil(t, fields["error_message"])
			assert.NotNil(t, fields["error_source"])
			assert.NotNil(t, fields["impact"])

			assert.Contains(t, issue.Tags, "autodiscovery")
		})
	}
}
