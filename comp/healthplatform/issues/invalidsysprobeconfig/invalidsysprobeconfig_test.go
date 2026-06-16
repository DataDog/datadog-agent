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

	"github.com/DataDog/datadog-agent/comp/core/config"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/schemacheck"
)

// Locks this module's slice of the dedup contract — what makes it distinct from
// invalid-config. The shared proto fields are locked in schemacheck_test.
func TestBuildIssue_LocksDedupContract(t *testing.T) {
	issue, err := (&invalidSysprobeConfigModule{}).BuildIssue(map[string]string{
		schemacheck.ContextKeyErrorCount: "1",
		schemacheck.ContextKeyErrors:     "/system_probe_config/health_port: got string, want integer",
	})
	require.NoError(t, err)
	assert.Equal(t, IssueID, issue.GetIssueName())
	assert.Equal(t, "system-probe", issue.GetLocation())
	assert.Equal(t, []string{"config", "schema", "system-probe"}, issue.GetTags())
	assert.Contains(t, issue.GetTitle(), "Datadog system-probe configuration")
}

// A valid system-probe setting passes the schema → no report.
func TestCheck_HealthyConfigReturnsNil(t *testing.T) {
	reports, err := check.Run(sysprobeconfigmock.NewMock(t))
	require.NoError(t, err)
	assert.Empty(t, reports)
}

// A string in an integer-typed field violates the schema → one report.
func TestCheck_SchemaViolationProducesReport(t *testing.T) {
	cfg := sysprobeconfigmock.NewMockWithOverrides(t, map[string]interface{}{
		"system_probe_config.health_port": "not-an-integer",
	})
	reports, err := check.Run(cfg)
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, IssueID, reports[0].IssueName)
	assert.Contains(t, reports[0].Context[schemacheck.ContextKeyErrors], "health_port")
}

// Without sysprobeconfig the module must NOT register a startup check, or the bundle
// would resolve a real persisted system-probe issue without ever validating.
func TestBuiltInStartupHealthCheck_SkippedWhenSysprobeAbsent(t *testing.T) {
	assert.Nil(t, (&invalidSysprobeConfigModule{sysprobe: nil}).BuiltInStartupHealthCheck())
}

// With sysprobeconfig present the startup check is registered with the system-probe source.
func TestBuiltInStartupHealthCheck_RegisteredWhenSysprobePresent(t *testing.T) {
	chk := (&invalidSysprobeConfigModule{sysprobe: sysprobeconfigmock.NewMock(t)}).BuiltInStartupHealthCheck()
	require.NotNil(t, chk)
	assert.Equal(t, "system-probe", chk.Source)
}

// The check is gated behind health_platform.invalidsysprobeconfig_check.enabled: off (the default)
// suppresses the report even with a violation; on lets it through.
func TestBuiltInStartupHealthCheck_GatedByFlag(t *testing.T) {
	sp := sysprobeconfigmock.NewMockWithOverrides(t, map[string]interface{}{
		"system_probe_config.health_port": "not-an-integer",
	})

	off := (&invalidSysprobeConfigModule{datadog: config.NewMock(t), sysprobe: sp}).BuiltInStartupHealthCheck()
	reports, err := off.Fn()
	require.NoError(t, err)
	assert.Empty(t, reports, "flag off suppresses the check")

	dd := config.NewMock(t)
	dd.SetInTest("health_platform.invalidsysprobeconfig_check.enabled", true)
	on := (&invalidSysprobeConfigModule{datadog: dd, sysprobe: sp}).BuiltInStartupHealthCheck()
	reports, err = on.Fn()
	require.NoError(t, err)
	require.Len(t, reports, 1, "flag on surfaces the violation")
}
