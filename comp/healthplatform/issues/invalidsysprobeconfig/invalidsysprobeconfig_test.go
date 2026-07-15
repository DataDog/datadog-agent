// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidsysprobeconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func testHostname(name string) hostnameinterface.Component {
	hn, _ := hostnamemock.NewMock(hostnamemock.MockHostname(name))
	return hn
}

// A valid system-probe setting passes the schema → no report.
func TestCheck_HealthyConfigReturnsNil(t *testing.T) {
	reports, err := newChecker(sysprobeconfigmock.NewMock(t), testHostname("h")).Run()
	require.NoError(t, err)
	assert.Empty(t, reports)
}

// A string in an integer-typed field violates the schema → one report.
func TestCheck_SchemaViolationProducesReport(t *testing.T) {
	cfg := sysprobeconfigmock.NewMockWithOverrides(t, map[string]interface{}{
		"system_probe_config.health_port": "not-an-integer",
	})
	reports, err := newChecker(cfg, testHostname("h")).Run()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Regexp(t, `^invalid-system-probe-config:[0-9a-f]{16}$`, reports[0].IssueID)
	assert.Equal(t, IssueName, reports[0].IssueName)
	assert.Equal(t, "system-probe", reports[0].Source)
}

// The reported IssueID is scoped per host so the recommendations service keeps each host's
// violation distinct instead of collapsing them into one case.
func TestInstanceIssueID_UniquePerHost(t *testing.T) {
	cfg := sysprobeconfigmock.NewMockWithOverrides(t, map[string]interface{}{
		"system_probe_config.health_port": "not-an-integer",
	})
	a, err := newChecker(cfg, testHostname("host-a")).Run()
	require.NoError(t, err)
	b, err := newChecker(cfg, testHostname("host-b")).Run()
	require.NoError(t, err)
	require.Len(t, a, 1)
	require.Len(t, b, 1)
	assert.NotEqual(t, a[0].IssueID, b[0].IssueID, "issue id must differ per host")
}

// The core guarantee: we validate the customer's config, not values that Adjust() rewrites
// at the agent-runtime layer. A bad file value must survive the merge and be seen by the check.
func TestCustomerConfig_SkipsAgentRuntime(t *testing.T) {
	cfg := sysprobeconfigmock.NewMock(t)
	cfg.Set("system_probe_config.health_port", "not-an-integer", model.SourceFile) // customer's value
	cfg.Set("system_probe_config.health_port", 5558, model.SourceAgentRuntime)     // Adjust's repair

	got := customerConfig(cfg)
	spc, _ := got["system_probe_config"].(map[string]any)
	require.NotNil(t, spc)
	assert.Equal(t, "not-an-integer", spc["health_port"], "the customer's file value must survive, not Adjust's runtime repair")
}

// Locks the dedup contract that distinguishes this issue from invalid-config.
func TestBuildIssue_LocksContract(t *testing.T) {
	issue, err := InvalidSysprobeConfigIssue{}.BuildIssue(map[string]string{
		contextKeyConfigPath: "/etc/datadog-agent/system-probe.yaml",
		contextKeyErrorCount: "1",
		contextErrorKey(0):   "at '/system_probe_config/health_port': got string, want integer",
	})
	require.NoError(t, err)
	assert.Equal(t, IssueName, issue.GetIssueName())
	assert.Equal(t, IssueType, issue.GetIssueType())
	assert.Equal(t, "system-probe", issue.GetLocation())
	assert.Equal(t, []string{"config", "schema", "system-probe"}, issue.GetTags())
	assert.Contains(t, issue.GetTitle(), "Datadog System-Probe Configuration")
	assert.Contains(t, issue.GetTitle(), "system-probe.yaml")
}

// Without system-probe config the startup check must NOT register, or the bundle would
// resolve a real persisted issue without ever validating.
func TestBuiltInStartupHealthCheck_SkippedWhenSysprobeAbsent(t *testing.T) {
	m := &invalidSysprobeConfigModule{datadog: config.NewMock(t), checker: newChecker(nil, nil)}
	assert.Nil(t, m.BuiltInStartupHealthCheck())
}

// The check is gated behind health_platform.invalidsysprobeconfig_check.enabled.
func TestBuiltInStartupHealthCheck_GatedByFlag(t *testing.T) {
	sp := sysprobeconfigmock.NewMockWithOverrides(t, map[string]interface{}{
		"system_probe_config.health_port": "not-an-integer",
	})
	dd := config.NewMock(t)
	m := &invalidSysprobeConfigModule{datadog: dd, checker: newChecker(sp, testHostname("h"))}

	// Enabled (default) → violation surfaces.
	reports, err := m.BuiltInStartupHealthCheck().Fn()
	require.NoError(t, err)
	require.Len(t, reports, 1)

	// Disabled → suppressed.
	dd.Set("health_platform.invalidsysprobeconfig_check.enabled", false, model.SourceAgentRuntime)
	reports, err = m.BuiltInStartupHealthCheck().Fn()
	require.NoError(t, err)
	assert.Empty(t, reports)
}
