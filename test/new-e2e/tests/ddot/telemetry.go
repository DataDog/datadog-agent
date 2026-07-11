// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ddot

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

const (
	metricProcmgrDaemonReachable        = "runtime__procmgr_daemon_reachable"
	metricProcmgrDaemonReady            = "runtime__procmgr_daemon_ready"
	metricProcmgrProcessRunning         = "runtime__procmgr_process_running"
	metricAgentServiceInstalled         = "runtime__agent_service_installed"
	metricAgentServiceProcmgrConfigured = "runtime__agent_service_procmgr_configured"
	metricAgentServiceManagementMode    = "runtime__agent_service_management_mode"
	ddotServiceID                       = "ddot"
	ddotManagementModeProcmgr           = "procmgr"
)

// AssertProcmgrDDOTTelemetry verifies COAT gauges report DDOT under dd-procmgrd.
// Call AssertDDOTManagedByProcmgr first so procmgr is reachable and DDOT is running.
func AssertProcmgrDDOTTelemetry(t *testing.T, host *components.RemoteHost) {
	t.Helper()

	// The procmgr reporter refreshes every 5 minutes; poll until gauges reflect the
	// state already confirmed by AssertDDOTManagedByProcmgr.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, err := host.Execute("sudo datadog-agent diagnose show-metadata agent-full-telemetry")
		require.NoError(c, err)

		assertTelemetryGaugeTrue(c, out, metricProcmgrDaemonReachable, nil)
		assertTelemetryGaugeTrue(c, out, metricProcmgrDaemonReady, nil)
		assertTelemetryGaugeTrue(c, out, metricProcmgrProcessRunning, map[string]string{
			"process": procmgrProcessName,
		})
		assertTelemetryGaugeTrue(c, out, metricAgentServiceInstalled, map[string]string{
			"service": ddotServiceID,
		})
		assertTelemetryGaugeTrue(c, out, metricAgentServiceProcmgrConfigured, map[string]string{
			"service": ddotServiceID,
		})
		assertTelemetryGaugeTrue(c, out, metricAgentServiceManagementMode, map[string]string{
			"service": ddotServiceID,
			"mode":    ddotManagementModeProcmgr,
		})
	}, 7*time.Minute, 10*time.Second, "procmgr DDOT telemetry gauges should be emitted")
}

func assertTelemetryGaugeTrue(c *assert.CollectT, output, metric string, labels map[string]string) {
	c.Helper()

	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(trimmed, metric) {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		value := fields[len(fields)-1]
		if value != "1" && value != "1.0" {
			continue
		}

		missingLabel := false
		for key, val := range labels {
			if !strings.Contains(trimmed, key+`="`+val+`"`) {
				missingLabel = true
				break
			}
		}
		if missingLabel {
			continue
		}

		return
	}

	if len(labels) == 0 {
		assert.Failf(c, "telemetry gauge not found", "expected %s with value 1", metric)
		return
	}
	assert.Failf(c, "telemetry gauge not found", "expected %s with labels %v and value 1", metric, labels)
}
