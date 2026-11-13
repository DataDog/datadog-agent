// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package untailedfiles

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fullContext(t *testing.T) map[string]string {
	t.Helper()
	sources, err := json.Marshal([]sourceFiles{
		{Source: "nginx", Service: "web", Path: "/var/log/nginx/*.log", Matched: 320, Untailed: 300},
		{Source: "redis", Service: "cache", Path: "/var/log/redis/*.log", Matched: 60, Untailed: 40},
	})
	require.NoError(t, err)
	return map[string]string{
		contextKeyLimit:          "500",
		contextKeyTailed:         "500",
		contextKeyUntailed:       "340",
		contextKeySourceCount:    "2",
		contextKeySourcesOmitted: "0",
		contextKeyLastSeenAt:     "2026-11-12T10:00:00Z",
		contextKeySources:        string(sources),
	}
}

func assertCommon(t *testing.T, issue *healthplatform.Issue) {
	t.Helper()
	assert.Empty(t, issue.Id, "the runner sets Id, never the template")
	assert.Equal(t, IssueName, issue.IssueName)
	assert.Equal(t, IssueType, issue.IssueType)
	assert.Equal(t, "logs_pipeline", issue.Category)
	assert.Equal(t, "logs-agent", issue.Location)
	assert.Equal(t, "logs", issue.Source)
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, issue.Severity)
	assert.NotContains(t, issue.Description, "\n")
	require.NotNil(t, issue.Remediation)
	require.Len(t, issue.Remediation.Steps, 5)
	for i, s := range issue.Remediation.Steps {
		assert.Equal(t, int32(i+1), s.Order)
	}
	assert.Contains(t, issue.Remediation.Steps[0].Text, "datadog-agent status")

	require.NotNil(t, issue.Extra)
	fields := issue.Extra.GetFields()
	for _, key := range []string{contextKeyLimit, contextKeyTailed, contextKeyUntailed, contextKeySourceCount,
		contextKeySourcesOmitted, contextKeyLastSeenAt, contextKeySources} {
		assert.Contains(t, fields, key)
	}
}

func TestBuildIssue_FullContext(t *testing.T) {
	issue, err := UntailedFilesIssue{}.BuildIssue(fullContext(t))
	require.NoError(t, err)
	assertCommon(t, issue)

	assert.Equal(t, "Not tailing 340 of 840 matching log files: logs_config.open_files_limit (500) reached", issue.Title)
	assert.Contains(t, issue.Description, "at most 500 log files at once")
	assert.Contains(t, issue.Description, "340 files from 2 sources got no tailer")
	assert.Contains(t, issue.Description, "Most affected: nginx/web (/var/log/nginx/*.log) 300 of 320 files, redis/cache")
	assert.Contains(t, issue.Remediation.Steps[1].Text, "from 500 to at least 840")

	sources := issue.Extra.GetFields()[contextKeySources].GetListValue().GetValues()
	require.Len(t, sources, 2)
	assert.Equal(t, float64(300), sources[0].GetStructValue().GetFields()["untailed"].GetNumberValue())
	assert.Equal(t, float64(500), issue.Extra.GetFields()[contextKeyLimit].GetNumberValue())
}

func TestBuildIssue_OmittedSources(t *testing.T) {
	ctx := fullContext(t)
	ctx[contextKeySourceCount] = "14"
	ctx[contextKeySourcesOmitted] = "12"

	issue, err := UntailedFilesIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	assert.Contains(t, issue.Description, "and 12 other sources.")
	assert.Contains(t, issue.Description, "from 14 sources")
}

func TestBuildIssue_SanitizesNames(t *testing.T) {
	ctx := fullContext(t)
	sources, _ := json.Marshal([]sourceFiles{{Source: "ngi\nnx", Service: "web", Path: "/var/log/\r" + strings.Repeat("a", 400), Matched: 2, Untailed: 1}})
	ctx[contextKeySources] = string(sources)

	issue, err := UntailedFilesIssue{}.BuildIssue(ctx)
	require.NoError(t, err)
	assert.NotContains(t, issue.Description, "\n")
	assert.NotContains(t, issue.Description, "\r")
	assert.Contains(t, issue.Description, "nginx/web")
	assert.Contains(t, issue.Description, "...")
}

func TestBuildIssue_EmptyAndMalformedContext(t *testing.T) {
	for name, ctx := range map[string]map[string]string{
		"nil":       nil,
		"empty":     {},
		"malformed": {contextKeySources: "{not json", contextKeyLimit: "x"},
	} {
		t.Run(name, func(t *testing.T) {
			issue, err := UntailedFilesIssue{}.BuildIssue(ctx)
			require.NoError(t, err)
			assertCommon(t, issue)
			assert.Equal(t, "Not tailing 0 of 0 matching log files: logs_config.open_files_limit (0) reached", issue.Title)
			assert.NotContains(t, issue.Description, "Most affected")
			assert.Contains(t, issue.Remediation.Steps[1].Text, "the number of files you need to tail")
			assert.Equal(t, "unknown", issue.Extra.GetFields()[contextKeyLastSeenAt].GetStringValue())
		})
	}
}
