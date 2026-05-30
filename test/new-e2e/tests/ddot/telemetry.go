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
	metricProcmgrDaemonReachable        = "runtime_procmgr_daemon_reachable"
	metricProcmgrDaemonReady            = "runtime_procmgr_daemon_ready"
	metricProcmgrProcessRunning         = "runtime_procmgr_process_running"
	metricAgentServiceInstalled         = "runtime_agent_service_installed"
	metricAgentServiceProcmgrConfigured = "runtime_agent_service_procmgr_configured"
	metricAgentServiceManagementMode    = "runtime_agent_service_management_mode"
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
		t.Logf("procmgr telemetry gauges:\n%s", filterProcmgrTelemetryOutput(out))
		logProcmgrDiagnostics(t, host)

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

func filterProcmgrTelemetryOutput(output string) string {
	const prefix = "runtime__"
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) &&
			(strings.Contains(trimmed, "procmgr") || strings.Contains(trimmed, "agent_service")) {
			lines = append(lines, trimmed)
		}
	}
	if len(lines) == 0 {
		return "(no procmgr telemetry gauges in agent-full-telemetry output)"
	}
	return strings.Join(lines, "\n")
}

func logProcmgrDiagnostics(t *testing.T, host *components.RemoteHost) {
	t.Helper()

	installRoot := resolveAgentInstallRoot(host)
	for _, cmd := range []string{"status", "list"} {
		out, err := host.Execute(procmgrCLICmd(installRoot, cmd))
		t.Logf("dd-procmgr %s (err=%v):\n%s", cmd, err, strings.TrimSpace(out))
	}

	if out, err := host.Execute("test -S " + procmgrSocket + " && ls -la " + procmgrSocket + " 2>&1 || true"); err == nil {
		t.Logf("procmgr socket:\n%s", strings.TrimSpace(out))
	}

	for _, unit := range []string{"datadog-agent-procmgr.service", "datadog-agent-procmgr-exp.service"} {
		out, err := host.Execute("sudo journalctl -u " + unit + " -n 200 --no-pager 2>/dev/null || true")
		if err != nil {
			t.Logf("%s journal unavailable: %v", unit, err)
			continue
		}
		if strings.TrimSpace(out) == "" {
			continue
		}
		t.Logf("%s journal:\n%s", unit, strings.TrimSpace(out))
	}

	if out, err := host.Execute(`sudo journalctl -u datadog-agent.service -n 500 --no-pager 2>/dev/null | grep -iE 'procmgr|telemetry' || true`); err == nil {
		if trimmed := strings.TrimSpace(out); trimmed != "" {
			t.Logf("datadog-agent procmgr/telemetry logs:\n%s", trimmed)
		}
	}

	if out, err := host.Execute(`test -f /var/log/datadog/dd-procmgr.log && sudo tail -n 200 /var/log/datadog/dd-procmgr.log || true`); err == nil {
		if trimmed := strings.TrimSpace(out); trimmed != "" {
			t.Logf("dd-procmgr.log:\n%s", trimmed)
		}
	}
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
