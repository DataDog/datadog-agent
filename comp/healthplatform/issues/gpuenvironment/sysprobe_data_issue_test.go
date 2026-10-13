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

func TestBuildSystemProbeDataIssue(t *testing.T) {
	tests := []struct {
		name          string
		failures      []SystemProbeSourceFailure
		expectedTitle string
		expectedSteps int
		// expectedDeploymentSteps is the number of leading Helm chart and Datadog Operator steps.
		expectedDeploymentSteps int
		expectedDesc            []string
	}{
		{
			name: "single source unreachable",
			failures: []SystemProbeSourceFailure{
				{Source: SystemProbeSourceEBPF, ErrorClass: SystemProbeErrorUnreachable, Error: "dial unix: no such file"},
			},
			expectedTitle:           "GPU monitoring is not receiving data from system-probe for: ebpf",
			expectedSteps:           4,
			expectedDeploymentSteps: 2,
			expectedDesc:            []string{"gpu.process.*"},
		},
		{
			name: "multiple sources sorted, module not loaded and timeout",
			failures: []SystemProbeSourceFailure{
				{Source: SystemProbeSourcePRM, ErrorClass: SystemProbeErrorTimeout, Error: "context deadline exceeded"},
				{Source: SystemProbeSourceDriverEvents, ErrorClass: SystemProbeErrorModuleNotLoaded, Error: "status_code: 404"},
			},
			expectedTitle:           "GPU monitoring is not receiving data from system-probe for: driver_events, prm",
			expectedSteps:           6,
			expectedDeploymentSteps: 2,
			expectedDesc:            []string{"XID", "NVLink"},
		},
		{
			name: "source disabled in system-probe",
			failures: []SystemProbeSourceFailure{
				{Source: SystemProbeSourceDriverEvents, ErrorClass: SystemProbeErrorSourceDisabled, Error: "status_code: 503"},
			},
			expectedTitle:           "GPU monitoring is not receiving data from system-probe for: driver_events",
			expectedSteps:           5,
			expectedDeploymentSteps: 2,
			expectedDesc:            []string{"XID"},
		},
		{
			name: "unreachable and source disabled",
			failures: []SystemProbeSourceFailure{
				{Source: SystemProbeSourceEBPF, ErrorClass: SystemProbeErrorUnreachable},
				{Source: SystemProbeSourceDriverEvents, ErrorClass: SystemProbeErrorSourceDisabled},
			},
			expectedTitle:           "GPU monitoring is not receiving data from system-probe for: driver_events, ebpf",
			expectedSteps:           8,
			expectedDeploymentSteps: 4,
			expectedDesc:            []string{"XID", "gpu.process.*"},
		},
		{
			name: "empty error class defaults to other",
			failures: []SystemProbeSourceFailure{
				{Source: SystemProbeSourceEBPF},
			},
			expectedTitle: "GPU monitoring is not receiving data from system-probe for: ebpf",
			expectedSteps: 2,
			expectedDesc:  []string{"gpu.process.*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue, err := NewSystemProbeDataIssue().BuildIssue(tt.failures)
			require.NoError(t, err)
			require.NotNil(t, issue)

			assert.Empty(t, issue.Id)
			assert.Equal(t, SystemProbeDataIssueName, issue.IssueName)
			assert.Equal(t, SystemProbeDataIssueType, issue.IssueType)
			assert.Equal(t, tt.expectedTitle, issue.Title)
			for _, desc := range tt.expectedDesc {
				assert.Contains(t, issue.Description, desc)
			}
			assert.Equal(t, category, issue.Category)
			assert.Equal(t, location, issue.Location)
			assert.Equal(t, severity, issue.Severity)
			assert.Equal(t, source, issue.Source)

			require.NotNil(t, issue.Remediation)
			assert.NotEmpty(t, issue.Remediation.Summary)
			require.Len(t, issue.Remediation.Steps, tt.expectedSteps)
			for i, step := range issue.Remediation.Steps {
				assert.Equal(t, int32(i+1), step.Order)
				isDeploymentStep := strings.HasPrefix(step.Text, "Helm chart:") || strings.HasPrefix(step.Text, "Datadog Operator:")
				assert.Equal(t, i < tt.expectedDeploymentSteps, isDeploymentStep, "step %d: %s", step.Order, step.Text)
			}

			require.NotNil(t, issue.Extra)
			fields := issue.Extra.GetFields()
			assert.NotNil(t, fields["impact"])
			affected := fields["affected_sources"].GetListValue().GetValues()
			require.Len(t, affected, len(tt.failures))
			for _, v := range affected {
				sourceFields := v.GetStructValue().GetFields()
				for _, key := range []string{"source", "description", "config_key", "error_class", "error"} {
					assert.NotNil(t, sourceFields[key], "missing key %s", key)
				}
				assert.NotEmpty(t, sourceFields["error_class"].GetStringValue())
			}

			assert.Contains(t, issue.Tags, "gpu")
		})
	}
}

func TestBuildSystemProbeDataIssueInvalidInput(t *testing.T) {
	_, err := NewSystemProbeDataIssue().BuildIssue(nil)
	require.Error(t, err)

	_, err = NewSystemProbeDataIssue().BuildIssue([]SystemProbeSourceFailure{{Source: "unknown"}})
	require.Error(t, err)
}

func TestBuildSystemProbeDataIssueTruncatesError(t *testing.T) {
	issue, err := NewSystemProbeDataIssue().BuildIssue([]SystemProbeSourceFailure{
		{Source: SystemProbeSourceEBPF, ErrorClass: SystemProbeErrorOther, Error: strings.Repeat("x", 2*maxSystemProbeErrorLength)},
	})
	require.NoError(t, err)

	affected := issue.Extra.GetFields()["affected_sources"].GetListValue().GetValues()
	require.Len(t, affected, 1)
	errMsg := affected[0].GetStructValue().GetFields()["error"].GetStringValue()
	assert.Len(t, errMsg, maxSystemProbeErrorLength+len("..."))
}
