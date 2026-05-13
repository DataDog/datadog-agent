// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidconfig

import (
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
	assert.Contains(t, issue.GetTitle(), "3 schema violations")
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

// A vanilla mock has only defaults, which round-trip through YAML cleanly and
// pass the schema. Confirms Run() is a no-op on a healthy config.
func TestCheck_HealthyConfigReturnsNil(t *testing.T) {
	report, err := newChecker(config.NewMock(t)).Run()
	require.NoError(t, err)
	assert.Nil(t, report)
}

// Inject a string into an integer-typed field Confirms the validator surfaces the violation and the
// checker wraps it into an IssueReport.
func TestCheck_SchemaViolationProducesReport(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetWithoutSource("agent_ipc.port", "not-a-number")

	report, err := newChecker(cfg).Run()
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, healthplatformdef.InvalidConfigIssueID, report.GetIssueId())
	assert.Equal(t, string(lite.ErrorKindSchemaValidation),
		report.GetContext()[lite.ContextKeyErrorKind])
	assert.Contains(t, report.GetContext()[lite.ContextKeyErrors], "agent_ipc/port")
}

func TestCheck_VerdictIsCachedAtStartup(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetWithoutSource("agent_ipc.port", "not-a-number")

	c := newChecker(cfg)
	first, _ := c.Run()
	require.NotNil(t, first)

	// Mutate the config after the first run. The cached verdict must still win.
	cfg.SetWithoutSource("agent_ipc.port", 5001)
	second, _ := c.Run()
	assert.Same(t, first, second, "Run must return the same cached IssueReport on subsequent calls")
}
