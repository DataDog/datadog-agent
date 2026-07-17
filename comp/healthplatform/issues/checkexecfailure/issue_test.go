// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package checkexecfailure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"
)

func TestBuildIssue_BasicFields(t *testing.T) {
	template := NewCheckExecFailureIssue()
	issue, err := template.BuildIssue(map[string]string{
		contextKeyCheckName:   "mysql",
		contextKeyErrors:      "connection refused",
		contextKeyOccurrences: "3",
	})

	require.NoError(t, err)
	assert.Empty(t, issue.GetId(), "Id is set by the reporter, not by the template")
	assert.Equal(t, IssueName, issue.GetIssueName())
	assert.Equal(t, IssueType, issue.GetIssueType())
	assert.Equal(t, Source, issue.GetSource())
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, issue.GetSeverity())
	assert.Equal(t, "Check 'mysql' Is Failing to Run", issue.GetTitle())
	assert.Contains(t, issue.GetDescription(), "mysql")
	assert.Contains(t, issue.GetDescription(), "3 consecutive runs")
	assert.Contains(t, issue.GetDescription(), "connection refused")
	assert.Contains(t, issue.GetTags(), "collector")
	assert.Contains(t, issue.GetTags(), "check_execution")

	extra := issue.GetExtra().GetFields()
	assert.Equal(t, "mysql", extra[contextKeyCheckName].GetStringValue())
	assert.Equal(t, "connection refused", extra[contextKeyErrors].GetStringValue())
	assert.Equal(t, "3", extra[contextKeyOccurrences].GetStringValue())
	assert.NotEmpty(t, extra[contextKeyImpact].GetStringValue())
}

func TestBuildIssue_Remediation(t *testing.T) {
	template := NewCheckExecFailureIssue()
	issue, err := template.BuildIssue(map[string]string{contextKeyCheckName: "mysql"})

	require.NoError(t, err)
	require.NotNil(t, issue.GetRemediation())
	assert.NotEmpty(t, issue.GetRemediation().GetSummary())
	require.Len(t, issue.GetRemediation().GetSteps(), 3)
	assert.Contains(t, issue.GetRemediation().GetSteps()[1].GetText(), "datadog-agent check mysql")
}

func TestBuildIssue_MissingCheckNameDefaultsToUnknown(t *testing.T) {
	template := NewCheckExecFailureIssue()
	issue, err := template.BuildIssue(map[string]string{})

	require.NoError(t, err)
	assert.Contains(t, issue.GetTitle(), "unknown")
}
