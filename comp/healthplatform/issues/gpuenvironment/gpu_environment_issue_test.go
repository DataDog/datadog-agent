// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gpuenvironment

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildGPUEnvironmentIssue(t *testing.T) {
	context := map[string]string{
		"reason": ReasonNvmlUnavailable,
	}

	issue, err := NewGPUEnvironmentIssue().BuildIssue(context)

	require.NoError(t, err)
	require.NotNil(t, issue)

	assert.Empty(t, issue.Id, "Id is set by the caller (ReportIssue), not by the template")
	assert.Equal(t, issueName, issue.IssueName)
	assert.Equal(t, issueType, issue.IssueType)
	assert.Equal(t, "GPU monitoring cannot initialize NVML", issue.Title)
	assert.Contains(t, issue.Description, "NVIDIA Management Library")
	assert.Equal(t, category, issue.Category)
	assert.Equal(t, severity, issue.Severity)
	assert.Equal(t, source, issue.Source)
	assert.Equal(t, location, issue.Location)

	require.NotNil(t, issue.Remediation)
	assert.Len(t, issue.Remediation.Steps, 4)

	require.NotNil(t, issue.Extra)
	fields := issue.Extra.GetFields()
	assert.NotNil(t, fields["reason"])
	assert.NotNil(t, fields["impact"])

	assert.Contains(t, issue.Tags, "gpu")
}
