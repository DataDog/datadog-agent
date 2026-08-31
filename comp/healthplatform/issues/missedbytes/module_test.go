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

// runModule runs the check function the scheduler would call. The tracker is a
// singleton, so these tests must not run in parallel.
func runModule(t *testing.T, yaml string) (int, error) {
	t.Helper()
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

// health_platform defaults on while logs_enabled defaults off, and an error here is
// warn-logged by both the runner and the scheduler on every tick.
func TestModule_LogsDisabledIsQuietAndResolves(t *testing.T) {
	logsmetrics.ResetMissedBytesForTest()

	n, err := runModule(t, "logs_enabled: false")

	assert.NoError(t, err, "logs being disabled is a known state, not a failure to determine one")
	assert.Zero(t, n, "no reports, so a stale issue from a previous logs-enabled run resolves")
}

// The logs_enabled gate must not swallow a real report.
func TestModule_LogsEnabledWithLossReports(t *testing.T) {
	logsmetrics.ResetMissedBytesForTest()
	logsmetrics.MarkLogsAgentRunning()
	logsmetrics.RecordMissedBytes("nginx", "web", 4096)

	n, err := runModule(t, "logs_enabled: true")

	require.NoError(t, err)
	assert.Equal(t, 1, n)
}
