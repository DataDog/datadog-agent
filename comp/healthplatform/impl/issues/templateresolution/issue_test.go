// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package templateresolution

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildIssueWithFullContext(t *testing.T) {
	template := NewTemplateResolutionIssue()

	issue, err := template.BuildIssue(map[string]string{
		"templateName":  "postgres",
		"serviceID":     "docker://abc123",
		"errorMessage":  "failed to get extra info for service docker://abc123, skipping config - AD: variable not supported by listener",
		"adIdentifiers": "_dbm_postgres_aurora",
		"source":        "file:/etc/datadog-agent/conf.d/postgres.d/conf.yaml",
		"provider":      "file",
	})

	require.NoError(t, err)
	assert.Equal(t, IssueID, issue.Id)
	assert.Equal(t, issueName, issue.IssueName)
	assert.Equal(t, "high", issue.Severity)
	assert.Equal(t, "configuration", issue.Category)
	assert.Equal(t, "autodiscovery", issue.Location)
	assert.Contains(t, issue.Title, "postgres")
	assert.Contains(t, issue.Description, "postgres")
	assert.Contains(t, issue.Description, "docker://abc123")
	assert.Contains(t, issue.Description, "variable not supported by listener")
	assert.NotNil(t, issue.Remediation)
	assert.NotEmpty(t, issue.Remediation.Steps)
	assert.NotNil(t, issue.Extra)
	assert.Equal(t, "postgres", issue.Extra.Fields["template_name"].GetStringValue())
	assert.Equal(t, "_dbm_postgres_aurora", issue.Extra.Fields["ad_identifiers"].GetStringValue())
	assert.Equal(t, "file", issue.Extra.Fields["provider"].GetStringValue())
	assert.Contains(t, issue.Tags, "autodiscovery")
	assert.Contains(t, issue.Tags, "postgres")
}

func TestBuildIssueWithMinimalContext(t *testing.T) {
	template := NewTemplateResolutionIssue()

	issue, err := template.BuildIssue(map[string]string{})

	require.NoError(t, err)
	assert.Equal(t, IssueID, issue.Id)
	assert.Contains(t, issue.Title, "unknown")
	assert.Contains(t, issue.Description, "unknown")
	assert.NotNil(t, issue.Remediation)
	// Optional fields should not appear in extra when empty
	assert.Nil(t, issue.Extra.Fields["ad_identifiers"])
	assert.Nil(t, issue.Extra.Fields["config_source"])
	assert.Nil(t, issue.Extra.Fields["provider"])
}

func TestBuildIssueRemediationSteps(t *testing.T) {
	template := NewTemplateResolutionIssue()

	issue, err := template.BuildIssue(map[string]string{
		"templateName": "redis",
		"serviceID":    "container://xyz",
		"errorMessage": "unsupported variable",
	})

	require.NoError(t, err)
	assert.Len(t, issue.Remediation.Steps, 5)
	assert.Contains(t, issue.Remediation.Steps[0].Text, "template variables")
	assert.Contains(t, issue.Remediation.Steps[1].Text, "configcheck")
	assert.Contains(t, issue.Remediation.Steps[3].Text, "docs.datadoghq.com")
}
