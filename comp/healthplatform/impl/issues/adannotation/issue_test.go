// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package adannotation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildIssue(t *testing.T) {
	tests := []struct {
		name            string
		context         map[string]string
		expectedTitle   string
		expectedDescSub string
		expectErr       bool
	}{
		{
			name: "valid context",
			context: map[string]string{
				"entityName":   "default/my-pod (abc123)",
				"errorMessage": "annotation ad.datadoghq.com/nonmatching.check_names is invalid: nonmatching doesn't match a container identifier",
			},
			expectedTitle:   "AD Annotation Error on 'default/my-pod (abc123)'",
			expectedDescSub: "nonmatching doesn't match a container identifier",
		},
		{
			name:            "empty context uses defaults",
			context:         map[string]string{},
			expectedTitle:   "AD Annotation Error on 'unknown'",
			expectedDescSub: failedMsg,
		},
		{
			name:            "nil context uses defaults",
			context:         nil,
			expectedTitle:   "AD Annotation Error on 'unknown'",
			expectedDescSub: failedMsg,
		},
		{
			name: "malformed JSON error message",
			context: map[string]string{
				"entityName":   "kube-system/nginx-pod (def456)",
				"errorMessage": "could not extract checks config: in checks: failed to unmarshal JSON",
			},
			expectedTitle:   "AD Annotation Error on 'kube-system/nginx-pod (def456)'",
			expectedDescSub: "failed to unmarshal JSON",
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

			assert.Equal(t, IssueID, issue.Id)
			assert.Equal(t, issueName, issue.IssueName)
			assert.Equal(t, tt.expectedTitle, issue.Title)
			assert.Contains(t, issue.Description, tt.expectedDescSub)
			assert.Equal(t, category, issue.Category)
			assert.Equal(t, severity, issue.Severity)
			assert.Equal(t, source, issue.Source)
			assert.Equal(t, location, issue.Location)

			// Verify remediation
			require.NotNil(t, issue.Remediation)
			assert.NotEmpty(t, issue.Remediation.Steps)
			assert.Equal(t, 5, len(issue.Remediation.Steps))

			// Verify extra fields
			require.NotNil(t, issue.Extra)
			fields := issue.Extra.GetFields()
			assert.NotNil(t, fields["entity_name"])
			assert.NotNil(t, fields["error_message"])
			assert.NotNil(t, fields["impact"])

			// Verify tags
			assert.Contains(t, issue.Tags, "ad-annotation")
			assert.Contains(t, issue.Tags, "autodiscovery")
		})
	}
}
