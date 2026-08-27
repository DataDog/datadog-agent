// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package missedbytes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

// runModule builds the module from the given agent config over a clean
// process-wide tracker and runs the check function the scheduler would call.
// The tracker is a singleton, so these tests must not run in parallel.
func runModule(t *testing.T, yaml string) (int, error) {
	t.Helper()
	logsmetrics.ResetMissedBytesForTest()
	t.Cleanup(logsmetrics.ResetMissedBytesForTest)

	hn, _ := hostnamemock.NewMock(hostnamemock.MockHostname("host-a"))
	m := NewModule(issues.ModuleDeps{
		Config:   config.NewMockFromYAML(t, yaml),
		Hostname: hn,
	})

	check := m.BuiltInPeriodicHealthCheck()
	require.NotNil(t, check, "the periodic check must always be registered")

	reports, err := check.Fn()
	return len(reports), err
}

// The check is registered on every agent but health_platform defaults on while
// logs_enabled defaults off, so the disabled case must stay silent: an error
// here is logged at warn by both the runner and the scheduler on every tick.
func TestModule_LogsDisabledIsQuietAndResolves(t *testing.T) {
	n, err := runModule(t, "logs_enabled: false")

	assert.NoError(t, err, "logs being disabled is a known state, not a failure to determine one")
	assert.Zero(t, n, "no reports, so a stale issue from a previous logs-enabled run resolves")
}

// With logs on but no file launcher, the process cannot establish state. It must
// report an error rather than nil so the scheduler leaves the running agent's
// issue alone — a flare shares the agent's on-disk issue store.
func TestModule_LogsEnabledWithoutFileLauncherReportsUnknown(t *testing.T) {
	n, err := runModule(t, "logs_enabled: true")

	assert.ErrorIs(t, err, errFileTailingInactive)
	assert.Zero(t, n)
}

// The logs_enabled gate must not swallow a real report.
func TestModule_LogsEnabledWithLossReports(t *testing.T) {
	logsmetrics.ResetMissedBytesForTest()
	t.Cleanup(logsmetrics.ResetMissedBytesForTest)

	logsmetrics.MarkFileTailingActive()
	logsmetrics.RecordMissedBytes("nginx", "web", 4096)

	hn, _ := hostnamemock.NewMock(hostnamemock.MockHostname("host-a"))
	m := NewModule(issues.ModuleDeps{
		Config:   config.NewMockFromYAML(t, "logs_enabled: true"),
		Hostname: hn,
	})

	reports, err := m.BuiltInPeriodicHealthCheck().Fn()
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, hostIssueID("host-a"), reports[0].IssueID)
}
