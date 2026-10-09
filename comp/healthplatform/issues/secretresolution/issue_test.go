// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package secretresolution

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
)

func TestSecretResolutionIssue(t *testing.T) {
	failure := secrets.ResolutionFailure{
		Handle: "redis-password", Origin: "config-id", OriginName: "redis", Path: []string{"password"},
	}
	for _, tc := range []struct{ reason, description, correction string }{
		{"missing", "did not return a value", "returns an entry"},
		{"empty", "returned an empty value", "non-empty value"},
		{"invalid_response", "returned an unreadable response", "valid JSON"},
		{"backend_error", "could not resolve", "access permissions"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			failure.Reason = tc.reason
			issue, err := buildIssue("qa-host", failure)
			require.NoError(t, err)
			assert.Equal(t, IssueType, issue.IssueType)
			assert.Equal(t, "Agent could not load secret \"redis-password\" for redis", issue.Title)
			assert.Contains(t, issue.Description, tc.description)
			assert.Contains(t, issue.Description, "ENC[redis-password]")
			assert.Contains(t, issue.Description, "/password")
			assert.Len(t, issue.Remediation.Steps, 3)
			assert.Contains(t, issue.Remediation.Steps[1].Text, tc.correction)
			assert.Equal(t, "/password", issue.Extra.AsMap()["setting_path"])
		})
	}
	failure.HasCachedValue = true
	issue, err := buildIssue("qa-host", failure)
	require.NoError(t, err)
	assert.Contains(t, issue.Description, "previously resolved value")
	id := issue.Id
	failure.Reason = "empty"
	issue, err = buildIssue("qa-host", failure)
	require.NoError(t, err)
	assert.Equal(t, id, issue.Id, "changes in reason or cached value preserve the issue ID")
	otherIssue, err := buildIssue("other-host", failure)
	require.NoError(t, err)
	assert.NotEqual(t, id, otherIssue.Id)
}

func TestSecretResolutionScrubsReferences(t *testing.T) {
	failure := secrets.ResolutionFailure{
		Handle: "https://user:private-password@example.com/secret",
		Origin: "config-id", OriginName: "redis", Reason: "backend_error",
		ConfigSource: "https://user:private-password@example.com/config",
		Path:         []string{"https://user:private-password@example.com", "token"},
	}
	issue, err := buildIssue("qa-host", failure)
	require.NoError(t, err)
	encoded, err := json.Marshal(issue)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private-password")
	assert.Contains(t, string(encoded), "example.com")
	assert.Equal(t, "https://user:private-password@example.com", failure.Path[0], "reporting must not modify resolver references")
}

func TestSecretResolutionConfigurationSource(t *testing.T) {
	issue, err := (&SecretResolutionIssue{}).BuildIssue(map[string]string{
		"reason": "missing", "handle": "redis-password", "configuration": "redis",
		"setting_path": "/password", "configuration_source": "file:/etc/datadog-agent/conf.d/redisdb.d/conf.yaml",
	})
	require.NoError(t, err)
	assert.Equal(t, "file:/etc/datadog-agent/conf.d/redisdb.d/conf.yaml", issue.Extra.AsMap()["configuration_source"])
}
