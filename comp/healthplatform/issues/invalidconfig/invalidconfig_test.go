// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
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

// cfgForPath wraps the standard config mock so ConfigFileUsed() returns a
// specific path. The rest of the Component surface forwards to the real mock.
type cfgForPath struct {
	config.Component
	path string
}

func (c cfgForPath) ConfigFileUsed() string { return c.path }

func mockCfg(t *testing.T, path string) config.Component {
	t.Helper()
	return cfgForPath{Component: config.NewMock(t), path: path}
}

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "datadog.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestCheck_ParseFailureProducesReport(t *testing.T) {
	c := newChecker(mockCfg(t, writeYAML(t, "{ this is not yaml\n")))
	report, err := c.Run()
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, healthplatformdef.InvalidConfigIssueID, report.GetIssueId())
	assert.Equal(t, string(lite.ErrorKindYAMLParse), report.GetContext()[lite.ContextKeyErrorKind])
	assert.NotEmpty(t, report.GetContext()[lite.ContextKeyErrorMessage])
}

func TestCheck_SchemaViolationProducesReport(t *testing.T) {
	c := newChecker(mockCfg(t, writeYAML(t,
		"api_key: abc\nsite: datadoghq.com\nagent_ipc:\n  port: \"not-a-number\"\n")))
	report, err := c.Run()
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, string(lite.ErrorKindSchemaValidation), report.GetContext()[lite.ContextKeyErrorKind])
	assert.Contains(t, report.GetContext()[lite.ContextKeyErrors], "agent_ipc/port")
}

func TestCheck_HealthyConfigReturnsNil(t *testing.T) {
	c := newChecker(mockCfg(t, writeYAML(t, "api_key: abc\nsite: datadoghq.com\n")))
	report, err := c.Run()
	require.NoError(t, err)
	assert.Nil(t, report)
}

func TestCheck_FileMissingReturnsNil(t *testing.T) {
	c := newChecker(mockCfg(t, "/does/not/exist/datadog.yaml"))
	report, err := c.Run()
	require.NoError(t, err)
	assert.Nil(t, report)
}

func TestCheck_EmptyPathReturnsNil(t *testing.T) {
	c := newChecker(mockCfg(t, ""))
	report, err := c.Run()
	require.NoError(t, err)
	assert.Nil(t, report)
}
