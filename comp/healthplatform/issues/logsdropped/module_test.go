// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsdropped

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
	t.Cleanup(logsmetrics.ResetDroppedLogsForTest)

	hn, _ := hostnamemock.NewMock(hostnamemock.MockHostname("host-a"))
	m := NewModule(issues.ModuleDeps{
		Config:   config.NewMockFromYAML(t, yaml),
		Hostname: hn,
	})
	assert.Equal(t, IssueName, m.IssueName())
	assert.Equal(t, IssueType, m.IssueType())
	assert.Nil(t, m.BuiltInStartupHealthCheck())

	check := m.BuiltInPeriodicHealthCheck()
	require.NotNil(t, check, "the periodic check must always be registered")
	assert.Equal(t, checkSource, check.Source)

	reports, err := check.Fn()
	return len(reports), err
}

// health_platform defaults on while logs_enabled defaults off.
func TestModule_LogsDisabledIsQuietAndResolves(t *testing.T) {
	logsmetrics.ResetDroppedLogsForTest()

	n, err := runModule(t, "logs_enabled: false")

	assert.NoError(t, err, "logs being disabled is a known state, not a failure to determine one")
	assert.Zero(t, n, "no reports, so a stale issue from a previous logs-enabled run resolves")
}

func TestModule_LogsEnabledWithDropsReports(t *testing.T) {
	for _, yaml := range []string{"logs_enabled: true", "log_enabled: true"} {
		t.Run(yaml, func(t *testing.T) {
			logsmetrics.ResetDroppedLogsForTest()
			logsmetrics.MarkLogsAgentRunning()
			record(logsmetrics.DropReasonAuthRejected, primary, "nginx", "web", 3)

			n, err := runModule(t, yaml)

			require.NoError(t, err)
			assert.Equal(t, 1, n)
		})
	}
}
