// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidconfig

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/config/lite"
)

func TestBuildIssue_YAMLParseHasHighSeverity(t *testing.T) {
	issue, err := InvalidConfigIssue{}.BuildIssue(map[string]string{
		lite.ContextKeyErrorKind:    string(lite.ErrorKindYAMLParse),
		lite.ContextKeyConfigPath:   "/etc/datadog-agent/datadog.yaml",
		lite.ContextKeyErrorMessage: "yaml: line 12: did not find expected ',' or ']'",
	})
	require.NoError(t, err)
	assert.Equal(t, healthplatformdef.InvalidConfigIssueID, issue.GetId())
	assert.Equal(t, "high", issue.GetSeverity())
	assert.Contains(t, issue.GetTitle(), "not valid YAML")
	assert.Contains(t, issue.GetDescription(), "/etc/datadog-agent/datadog.yaml")
	assert.Equal(t, string(lite.ErrorKindYAMLParse),
		issue.GetExtra().GetFields()[lite.ContextKeyErrorKind].GetStringValue())
	require.NotEmpty(t, issue.GetRemediation().GetSteps())
}

func TestBuildIssue_SchemaValidationHasMediumSeverity(t *testing.T) {
	issue, err := InvalidConfigIssue{}.BuildIssue(map[string]string{
		lite.ContextKeyErrorKind:  string(lite.ErrorKindSchemaValidation),
		lite.ContextKeyConfigPath: "/etc/datadog-agent/datadog.yaml",
		lite.ContextKeyErrorCount: "3",
		lite.ContextKeyErrors:     "/agent_ipc/port: expected integer\n/tags: expected array",
	})
	require.NoError(t, err)
	assert.Equal(t, "medium", issue.GetSeverity())
	assert.Contains(t, issue.GetTitle(), "3 schema violation(s)")
	assert.Equal(t, string(lite.ErrorKindSchemaValidation),
		issue.GetExtra().GetFields()[lite.ContextKeyErrorKind].GetStringValue())
	assert.Equal(t, float64(3),
		issue.GetExtra().GetFields()[lite.ContextKeyErrorCount].GetNumberValue())
	assert.Contains(t,
		issue.GetExtra().GetFields()[lite.ContextKeyErrors].GetStringValue(),
		"agent_ipc/port")
}

// Backend dedupe depends on both code paths emitting the same Issue ID.
func TestBuildIssue_SharesIssueIDWithRescue(t *testing.T) {
	assert.Equal(t, lite.IssueID, healthplatformdef.InvalidConfigIssueID)
}

func newTestChecker(read func(string) ([]byte, error)) *checker {
	return &checker{readFile: read}
}

func TestCheck_ParseFailureProducesReport(t *testing.T) {
	c := newTestChecker(func(string) ([]byte, error) {
		return []byte("{ this is not yaml\n"), nil
	})
	report, err := c.Run()
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, healthplatformdef.InvalidConfigIssueID, report.GetIssueId())
	assert.Equal(t, string(lite.ErrorKindYAMLParse), report.GetContext()[lite.ContextKeyErrorKind])
	assert.NotEmpty(t, report.GetContext()[lite.ContextKeyErrorMessage])
}

// Healthy YAML hits VerdictSchemaUnavailable in tests (no embedded schema)
// which the checker swallows — the report should be nil either way.
func TestCheck_HealthyConfigReturnsNil(t *testing.T) {
	c := newTestChecker(func(string) ([]byte, error) {
		return []byte("api_key: abc\nsite: dd.eu\n"), nil
	})
	report, err := c.Run()
	require.NoError(t, err)
	assert.Nil(t, report)
}

func TestCheck_FileMissingReturnsNil(t *testing.T) {
	c := newTestChecker(func(string) ([]byte, error) {
		return nil, errors.New("no such file or directory")
	})
	report, err := c.Run()
	require.NoError(t, err)
	assert.Nil(t, report)
}

// configFilePath must always return a non-empty path so Run can decide to
// readFile or skip based on a single check.
func TestConfigFilePath_FallsBackToDefault(t *testing.T) {
	assert.NotEmpty(t, (&checker{cfg: nil}).configFilePath())
}
