// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package schemacheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubReader struct {
	settings   map[string]any
	configFile string
}

func (s stubReader) AllSettingsWithoutDefaultOrSecrets() map[string]any { return s.settings }
func (s stubReader) ConfigFileUsed() string                             { return s.configFile }

func badValidator(any) ([]string, error) { return []string{"/k: bad"}, nil }

func TestRun_EmptySettingsNoReport(t *testing.T) {
	reports, err := Check{IssueID: "id", Validator: badValidator}.Run(stubReader{})
	require.NoError(t, err)
	assert.Empty(t, reports, "no customer-set values means nothing to validate")
}

func TestRun_ViolationProducesOneReport(t *testing.T) {
	r := stubReader{settings: map[string]any{"k": "v"}, configFile: "/etc/x.yaml"}
	reports, err := Check{IssueID: "my-id", Validator: badValidator}.Run(r)
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, "my-id", reports[0].IssueID)
	assert.Equal(t, "my-id", reports[0].IssueName)
	assert.Equal(t, "/etc/x.yaml", reports[0].Context[ContextKeyConfigPath])
	assert.Equal(t, "1", reports[0].Context[ContextKeyErrorCount])
	assert.Contains(t, reports[0].Context[ContextKeyErrors], "/k: bad")
}

// normalizeForSchema scrubs secret-like values
func TestNormalizeForSchema_ScrubsSecrets(t *testing.T) {
	out, err := normalizeForSchema(map[string]any{
		"api_key": "0123456789abcdef0123456789abcdef",
		"nested":  map[string]any{"port": 5558},
	})
	require.NoError(t, err)
	assert.NotEqual(t, "0123456789abcdef0123456789abcdef", out["api_key"], "secret-like values are scrubbed")
	assert.Equal(t, map[string]any{"port": 5558}, out["nested"], "structure and types survive the round-trip")
}

// Singular count drops the plural suffix
func TestBuildIssue_SingularSuffix(t *testing.T) {
	issue, err := Check{Subject: "X", ViolationNoun: "schema"}.BuildIssue(map[string]string{
		ContextKeyErrorCount: "1",
		ContextKeyErrors:     "/a: bad",
	})
	require.NoError(t, err)
	assert.Equal(t, "X has 1 schema violation", issue.GetTitle())
}

// An unparseable count must not understate a non-empty error list as 0
func TestBuildIssue_MalformedCountFallsBackToBlob(t *testing.T) {
	issue, err := Check{Subject: "X", ViolationNoun: "schema"}.BuildIssue(map[string]string{
		ContextKeyErrorCount: "not-a-number",
		ContextKeyErrors:     "/a: bad\n/b: bad\n/c: bad",
	})
	require.NoError(t, err)
	assert.Equal(t, "X has 3 schema violations", issue.GetTitle())
}
