// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !jetson

package invalidsysprobeconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
)

// stubSysprobeConfig is a minimal sysprobeconfig.Component for testing the checker
// pipeline directly
type stubSysprobeConfig struct {
	sysprobeconfig.Component
	settings   map[string]any
	configFile string
}

func (s *stubSysprobeConfig) AllSettingsWithoutDefaultOrSecrets() map[string]any { return s.settings }
func (s *stubSysprobeConfig) ConfigFileUsed() string                             { return s.configFile }

// Locks the dedup contract: the backend keys on these fields, and they must stay
// distinct from invalid-config (different IssueName, Location, and an extra tag).
func TestBuildIssue_LocksDedupContract(t *testing.T) {
	issue, err := InvalidSysprobeConfigIssue{}.BuildIssue(map[string]string{
		contextKeyErrorCount: "1",
		contextKeyErrors:     "/system_probe_config/health_port: got string, want integer",
	})
	require.NoError(t, err)
	assert.Empty(t, issue.GetId(), "Id is set by the runner, not the template")
	assert.Equal(t, IssueID, issue.GetIssueName())
	assert.Equal(t, "configuration", issue.GetCategory())
	assert.Equal(t, "system-probe", issue.GetLocation())
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, issue.GetSeverity())
	assert.Equal(t, "config", issue.GetSource())
	assert.Equal(t, []string{"config", "schema", "system-probe"}, issue.GetTags())
}

// A valid system-probe setting passes the schema, so Run() reports nothing.
func TestCheck_HealthyConfigReturnsNil(t *testing.T) {
	cfg := &stubSysprobeConfig{settings: map[string]any{
		"system_probe_config": map[string]any{"health_port": 5558},
	}}
	reports, err := newChecker(cfg).Run()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

// A string in an integer-typed field violates the schema; the checker wraps the
// violation into a single IssueReport keyed by the system-probe issue id.
func TestCheck_SchemaViolationProducesReport(t *testing.T) {
	cfg := &stubSysprobeConfig{settings: map[string]any{
		"system_probe_config": map[string]any{"health_port": "not-an-integer"},
	}}
	reports, err := newChecker(cfg).Run()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, IssueID, reports[0].IssueName)
	assert.Equal(t, IssueID, reports[0].IssueID)
	assert.Equal(t, "system-probe", reports[0].Source)
	assert.Contains(t, reports[0].Context[contextKeyErrors], "health_port")
}

// When sysprobeconfig is absent
func TestCheck_NilSysprobeConfigNoOps(t *testing.T) {
	reports, err := newChecker(nil).Run()
	require.NoError(t, err)
	assert.Empty(t, reports)
}
