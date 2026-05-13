// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildInvalidConfigIssue_YAMLParse(t *testing.T) {
	issue := BuildInvalidConfigIssue(IssueInfo{
		Kind:         ErrorKindYAMLParse,
		ConfigPath:   "/etc/datadog-agent/datadog.yaml",
		ErrorMessage: "yaml: line 12: did not find expected ',' or ']'",
	})
	require.NotNil(t, issue)
	assert.Equal(t, IssueID, issue.GetId())
	assert.Equal(t, "invalid_config", issue.GetIssueName())
	assert.Equal(t, "high", issue.GetSeverity())
	assert.Equal(t, "config", issue.GetCategory())
	assert.Equal(t, "config", issue.GetSource())
	assert.Contains(t, issue.GetTitle(), "not valid YAML")
	assert.Contains(t, issue.GetDescription(), "/etc/datadog-agent/datadog.yaml")
	assert.Contains(t, issue.GetDescription(), "line 12")
	require.NotNil(t, issue.GetRemediation())
	require.NotEmpty(t, issue.GetRemediation().GetSteps())
	assert.ElementsMatch(t, []string{"config", "yaml_parse"}, issue.GetTags())
}

func TestBuildInvalidConfigIssue_YAMLParse_MissingPath(t *testing.T) {
	issue := BuildInvalidConfigIssue(IssueInfo{
		Kind:         ErrorKindYAMLParse,
		ErrorMessage: "yaml error",
	})
	require.NotNil(t, issue)
	assert.Contains(t, issue.GetDescription(), "(no datadog.yaml found)")
}

func TestBuildInvalidConfigIssue_SchemaValidation(t *testing.T) {
	errs := "/agent_ipc/port: expected integer\n/tags: expected array"
	issue := BuildInvalidConfigIssue(IssueInfo{
		Kind:       ErrorKindSchemaValidation,
		ConfigPath: "/etc/datadog-agent/datadog.yaml",
		Errors:     errs,
		ErrorCount: 2,
	})
	require.NotNil(t, issue)
	assert.Equal(t, "medium", issue.GetSeverity())
	assert.Contains(t, issue.GetTitle(), "2 schema violations")
	assert.Contains(t, issue.GetDescription(), "Found 2 schema violations")
	assert.Contains(t, issue.GetDescription(), "/agent_ipc/port")
	assert.ElementsMatch(t, []string{"config", "schema"}, issue.GetTags())
}

func TestBuildInvalidConfigIssue_SchemaValidation_PluralizesSingular(t *testing.T) {
	issue := BuildInvalidConfigIssue(IssueInfo{
		Kind:       ErrorKindSchemaValidation,
		ConfigPath: "/etc/datadog-agent/datadog.yaml",
		Errors:     "/agent_ipc/port: expected integer",
		ErrorCount: 1,
	})
	assert.Contains(t, issue.GetTitle(), "1 schema violation")
	assert.NotContains(t, issue.GetTitle(), "violations")
	assert.NotContains(t, issue.GetTitle(), "violation(s)")
}

func TestBuildInvalidConfigIssue_SchemaValidation_MissingPath(t *testing.T) {
	issue := BuildInvalidConfigIssue(IssueInfo{
		Kind:       ErrorKindSchemaValidation,
		Errors:     "/a: bad",
		ErrorCount: 1,
	})
	assert.Contains(t, issue.GetDescription(), "(unknown path)")
}

func TestBuildInvalidConfigIssue_StartupFailure(t *testing.T) {
	issue := BuildInvalidConfigIssue(IssueInfo{
		Kind:         ErrorKindStartupFailure,
		ConfigPath:   "/etc/datadog-agent/datadog.yaml",
		ErrorMessage: "port 8125 already in use",
	})
	require.NotNil(t, issue)
	assert.Equal(t, "high", issue.GetSeverity())
	assert.Equal(t, "config", issue.GetSource())
	assert.Contains(t, issue.GetTitle(), "failed to start")
	assert.Contains(t, issue.GetDescription(), "port 8125 already in use")
	assert.ElementsMatch(t, []string{"agent", "startup_failure"}, issue.GetTags())
}

func TestIssueInfo_Tags(t *testing.T) {
	cases := map[ErrorKind][]string{
		ErrorKindYAMLParse:        {"config", "yaml_parse"},
		ErrorKindSchemaValidation: {"config", "schema"},
		ErrorKindStartupFailure:   {"agent", "startup_failure"},
	}
	for kind, want := range cases {
		assert.ElementsMatch(t, want, IssueInfo{Kind: kind}.Tags(), "kind=%s", kind)
	}
}

func TestIssueInfo_ContextRoundTrip_Schema(t *testing.T) {
	original := IssueInfo{
		Kind:       ErrorKindSchemaValidation,
		ConfigPath: "/etc/datadog-agent/datadog.yaml",
		Errors:     "/a/b: bad",
		ErrorCount: 3,
	}
	assert.Equal(t, original, IssueInfoFromContext(original.ToContext()))
}

func TestIssueInfo_ContextRoundTrip_YAMLParse(t *testing.T) {
	original := IssueInfo{
		Kind:         ErrorKindYAMLParse,
		ConfigPath:   "/etc/datadog-agent/datadog.yaml",
		ErrorMessage: "broken",
	}
	assert.Equal(t, original, IssueInfoFromContext(original.ToContext()))
}

func TestIssueInfo_ContextRoundTrip_StartupFailure(t *testing.T) {
	original := IssueInfo{
		Kind:         ErrorKindStartupFailure,
		ConfigPath:   "/etc/datadog-agent/datadog.yaml",
		ErrorMessage: "boom",
	}
	assert.Equal(t, original, IssueInfoFromContext(original.ToContext()))
}
