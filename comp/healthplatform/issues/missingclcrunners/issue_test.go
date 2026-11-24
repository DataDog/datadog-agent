// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package missingclcrunners

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthplatform "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
)

func TestBuildIssue_BasicFields(t *testing.T) {
	template := &MissingCLCRunnersIssue{}
	issue, err := template.BuildIssue(map[string]string{"pending_checks": "7"})

	require.NoError(t, err)
	assert.Equal(t, IssueID, issue.Id)
	assert.Equal(t, issueName, issue.IssueName)
	assert.Equal(t, issueType, issue.IssueType)
	assert.Equal(t, "Missing Cluster Check Runners", issue.Title)
	assert.Contains(t, issue.Description, "no eligible Cluster Check Runner")
	assert.Equal(t, "availability", issue.Category)
	assert.Equal(t, "cluster-checks", issue.Location)
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, issue.Severity)
	assert.Equal(t, "cluster-agent", issue.Source)
	assert.Contains(t, issue.Tags, "cluster-check-runner")
	assert.Contains(t, issue.Tags, "cluster-agent")
}

func TestIssueTypeMatchesName(t *testing.T) {
	// IssueType must equal IssueName lowercased with spaces replaced by underscores.
	assert.Equal(t, "missing_cluster_check_runners", IssueType)
}

func TestBuildIssue_Remediation(t *testing.T) {
	template := &MissingCLCRunnersIssue{}
	issue, err := template.BuildIssue(map[string]string{})

	require.NoError(t, err)
	require.NotNil(t, issue.Remediation)
	assert.NotEmpty(t, issue.Remediation.Summary)
	assert.Len(t, issue.Remediation.Steps, 5)

	lastStep := issue.Remediation.Steps[len(issue.Remediation.Steps)-1]
	assert.Contains(t, lastStep.Text, "docs.datadoghq.com")
}

func TestBuildIssue_Extra(t *testing.T) {
	template := &MissingCLCRunnersIssue{}

	issue, err := template.BuildIssue(map[string]string{"pending_checks": "3"})
	require.NoError(t, err)
	require.NotNil(t, issue.Extra)
	assert.NotEmpty(t, issue.Extra.Fields["impact"].GetStringValue())
	assert.Equal(t, "3", issue.Extra.Fields["pending_checks"].GetStringValue())

	// pending_checks is optional and omitted when not provided.
	issue, err = template.BuildIssue(map[string]string{})
	require.NoError(t, err)
	require.NotNil(t, issue.Extra)
	_, hasPending := issue.Extra.Fields["pending_checks"]
	assert.False(t, hasPending)
}

func TestNewModule(t *testing.T) {
	m := NewModule(issues.ModuleDeps{})
	assert.Equal(t, IssueName, m.IssueName())
	assert.Equal(t, IssueType, m.IssueType())
	issue, err := m.BuildIssue(map[string]string{})
	require.NoError(t, err)
	assert.NotNil(t, issue)
	assert.Nil(t, m.BuiltInPeriodicHealthCheck())
	assert.Nil(t, m.BuiltInStartupHealthCheck())
}
