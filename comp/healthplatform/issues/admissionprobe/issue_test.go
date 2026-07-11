// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package admissionprobe

import (
	"testing"

	"github.com/stretchr/testify/assert"

	healthplatform "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	"github.com/stretchr/testify/require"
)

func TestBuildIssue_BasicFields(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue":       "webhook not reachable",
		"remediation": "check firewall rules",
	})

	require.NoError(t, err)
	assert.Equal(t, IssueID, issue.Id)
	assert.Equal(t, issueName, issue.IssueName)
	assert.Equal(t, "Admission Controller Unreachable", issue.Title)
	assert.Contains(t, issue.Description, "webhook not reachable")
	assert.Equal(t, "availability", issue.Category)
	assert.Equal(t, "admission-controller", issue.Location)
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, issue.Severity)
	assert.Equal(t, "cluster-agent", issue.Source)
	assert.Contains(t, issue.Tags, "admission-controller")
	assert.Contains(t, issue.Tags, "connectivity")
	assert.Contains(t, issue.Tags, "cluster-agent")
}

func TestBuildIssue_Remediation(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue":       "timeout",
		"remediation": "check network",
	})

	require.NoError(t, err)
	require.NotNil(t, issue.Remediation)
	assert.NotEmpty(t, issue.Remediation.Summary)
	assert.Len(t, issue.Remediation.Steps, 5)

	lastStep := issue.Remediation.Steps[len(issue.Remediation.Steps)-1]
	assert.Contains(t, lastStep.Text, "dtdg.co")

	assert.Equal(t, "check network", issue.Remediation.Steps[3].Text)
}

func TestBuildIssue_Defaults(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{})

	require.NoError(t, err)
	assert.Contains(t, issue.Description, "unreachable from the Kubernetes API server")
	assert.Contains(t, issue.Remediation.Steps[3].Text, "port 8000")
}

func TestBuildIssue_Extra(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue":       "connection refused",
		"remediation": "allow port 8000",
	})

	require.NoError(t, err)
	require.NotNil(t, issue.Extra)
	assert.NotEmpty(t, issue.Extra.Fields["impact"].GetStringValue())
}

func TestNewModule(t *testing.T) {
	m := NewModule(issues.ModuleDeps{})
	assert.Equal(t, IssueName, m.IssueName())
	issue, err := m.BuildIssue(map[string]string{})
	require.NoError(t, err)
	assert.NotNil(t, issue)
	assert.Nil(t, m.BuiltInPeriodicHealthCheck())
	assert.Nil(t, m.BuiltInStartupHealthCheck())
}
