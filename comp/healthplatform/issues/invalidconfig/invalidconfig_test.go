// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !jetson

package invalidconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

func TestBuildIssue_SchemaViolationProducesMediumSeverity(t *testing.T) {
	issue, err := InvalidConfigIssue{}.BuildIssue(map[string]string{
		contextKeyConfigPath: "/etc/datadog-agent/datadog.yaml",
		contextKeyErrorCount: "3",
		contextKeyErrors:     "/agent_ipc/port: expected integer\n/tags: expected array",
	})
	require.NoError(t, err)
	assert.Equal(t, IssueID, issue.GetId())
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, issue.GetSeverity())
	assert.Contains(t, issue.GetTitle(), "3 schema violations")
	assert.Equal(t, float64(3),
		issue.GetExtra().GetFields()[contextKeyErrorCount].GetNumberValue())
	assert.Contains(t, issue.GetDescription(), "agent_ipc/port")
	assert.Contains(t, issue.GetDescription(), "/tags")
	assert.Contains(t, issue.GetDescription(), "; ", "description must use a visible delimiter between violations so the UI renders them legibly")

	errorsBlob := issue.GetExtra().GetFields()[contextKeyErrors].GetStringValue()
	assert.Contains(t, errorsBlob, "agent_ipc/port")
	assert.Contains(t, errorsBlob, "/tags")
	assert.Contains(t, errorsBlob, " • ", "extra.errors must use a visible delimiter so the UI renders multi-violation blobs legibly")
}

// A vanilla mock has only defaults, which round-trip through YAML cleanly and
// pass the schema. Confirms Run() is a no-op on a healthy config.
func TestCheck_HealthyConfigReturnsNil(t *testing.T) {
	reports, err := newChecker(config.NewMock(t)).Run()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

// Inject a string into an integer-typed field. Confirms the validator surfaces
// the violation and the checker wraps it into an IssueReport.
func TestCheck_SchemaViolationProducesReport(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetWithoutSource("agent_ipc.port", "not-a-number")

	reports, err := newChecker(cfg).Run()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, IssueID, reports[0].IssueName)
	assert.Equal(t, IssueID, reports[0].IssueID)
	assert.Contains(t, reports[0].Context[contextKeyErrors], "agent_ipc/port")
}
