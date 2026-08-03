// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package host contains host-level test helpers for fleet tests.
package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

// linuxStableInstallDir is the OCI install root for the stable datadog-agent package on Linux.
const linuxStableInstallDir = "/opt/datadog-packages/datadog-agent/stable"

// Host wraps an environments.Host with helper methods for fleet tests.
type Host struct {
	*environments.Host
}

// New creates a new Host wrapper.
func New(host *environments.Host) *Host {
	return &Host{Host: host}
}

// FilePermissions represents the permissions of a file on Unix systems.
type FilePermissions struct {
	Mode  string
	Owner string
	Group string
}

// GetFilePermissions returns the permissions of a file on Unix systems.
// Returns an error on Windows as POSIX permissions don't apply.
func (h *Host) GetFilePermissions(filePath string) (*FilePermissions, error) {
	switch h.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		// Use stat to get file permissions, owner, and group
		output, err := h.RemoteHost.Execute("stat -c '%a %U %G' " + filePath)
		if err != nil {
			return nil, err
		}
		parts := strings.Fields(strings.TrimSpace(output))
		if len(parts) != 3 {
			return nil, fmt.Errorf("unexpected stat output: %s", output)
		}
		return &FilePermissions{
			Mode:  parts[0],
			Owner: parts[1],
			Group: parts[2],
		}, nil
	case e2eos.WindowsFamily:
		// Windows doesn't use POSIX permissions
		return nil, errors.New("file permissions check not supported on Windows")
	default:
		return nil, fmt.Errorf("unsupported OS family: %v", h.RemoteHost.OSFamily)
	}
}

// DirExists checks if a directory exists on the remote host.
// Returns true if the path exists and is a directory, false if it doesn't exist or isn't a directory.
// Only returns an error for actual failures (e.g., permission issues).
func (h *Host) DirExists(path string) (bool, error) {
	info, err := h.RemoteHost.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

// procmgrCLI returns the path to the dd-procmgr executable for the current host's OS.
func (h *Host) procmgrCLI() (string, error) {
	switch h.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		return filepath.Join(linuxStableInstallDir, "embedded/bin/dd-procmgr"), nil
	case e2eos.WindowsFamily:
		installRoot, err := windowsagent.GetInstallPathFromRegistry(h.RemoteHost)
		if err != nil {
			return "", err
		}
		return filepath.Join(installRoot, "bin", "agent", "dd-procmgr.exe"), nil
	default:
		return "", fmt.Errorf("unsupported OS family: %v", h.RemoteHost.OSFamily)
	}
}

// procmgrExec invokes the dd-procmgr CLI with the given arguments, using the invocation mechanism
// appropriate for the host's OS (the daemon runs as dd-agent on Linux; no equivalent restriction
// applies on Windows).
func (h *Host) procmgrExec(cli, args string) (string, error) {
	switch h.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		return h.RemoteHost.Execute("sudo -u dd-agent " + cli + " " + args)
	case e2eos.WindowsFamily:
		escaped := strings.ReplaceAll(cli, "'", "''")
		return h.RemoteHost.Execute(fmt.Sprintf(`powershell -NoProfile -Command "$ErrorActionPreference='Stop'; & '%s' %s"`, escaped, args))
	default:
		return "", fmt.Errorf("unsupported OS family: %v", h.RemoteHost.OSFamily)
	}
}

// ProcmgrEnabled reports whether dd-procmgr is the active service manager on the host, i.e.
// whether the dd-procmgr CLI is present under the current datadog-agent install. This is a
// presence check, not an opt-out check: hosts running an agent version or install method that
// predates procmgr correctly report false here.
func (h *Host) ProcmgrEnabled() bool {
	cli, err := h.procmgrCLI()
	if err != nil {
		return false
	}
	switch h.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		_, err = h.RemoteHost.Execute("test -x " + cli)
	case e2eos.WindowsFamily:
		escaped := strings.ReplaceAll(cli, "'", "''")
		_, err = h.RemoteHost.Execute(fmt.Sprintf(`powershell -NoProfile -Command "if (-not (Test-Path -LiteralPath '%s')) { exit 1 }"`, escaped))
	default:
		return false
	}
	return err == nil
}

// ProcmgrProcess is the state of a single process supervised by dd-procmgrd, as reported by
// `dd-procmgr describe --json`.
type ProcmgrProcess struct {
	Name         string
	UUID         string
	State        string
	PID          int
	Command      string
	Args         []string
	RestartCount int
	AutoStart    bool
}

// DescribeProcess returns the dd-procmgrd-reported state of a supervised process. ok is false if
// the process is not registered with dd-procmgrd (including when dd-procmgr itself isn't present).
func (h *Host) DescribeProcess(name string) (info ProcmgrProcess, ok bool, err error) {
	cli, err := h.procmgrCLI()
	if err != nil {
		return ProcmgrProcess{}, false, nil
	}
	out, err := h.procmgrExec(cli, "describe --json "+name)
	if err != nil {
		return ProcmgrProcess{}, false, nil
	}
	var detail struct {
		Name         string   `json:"name"`
		UUID         string   `json:"uuid"`
		State        string   `json:"state"`
		PID          int      `json:"pid"`
		Command      string   `json:"command"`
		Args         []string `json:"args"`
		RestartCount int      `json:"restart_count"`
		AutoStart    bool     `json:"auto_start"`
	}
	if err := json.Unmarshal([]byte(out), &detail); err != nil {
		return ProcmgrProcess{}, false, fmt.Errorf("failed to parse dd-procmgr describe output: %w", err)
	}
	return ProcmgrProcess{
		Name:         detail.Name,
		UUID:         detail.UUID,
		State:        detail.State,
		PID:          detail.PID,
		Command:      detail.Command,
		Args:         detail.Args,
		RestartCount: detail.RestartCount,
		AutoStart:    detail.AutoStart,
	}, true, nil
}

// AssertProcessRunning fails the test unless dd-procmgrd reports name as stably Running.
func (h *Host) AssertProcessRunning(t *testing.T, name string) {
	t.Helper()
	var runningSince time.Time
	const minRunningDuration = 5 * time.Second
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		info, ok, err := h.DescribeProcess(name)
		require.NoError(c, err)
		if !assert.True(c, ok, "process %s is not loaded by dd-procmgrd", name) {
			runningSince = time.Time{}
			return
		}
		if !assert.Equal(c, "Running", info.State, "process %s is not running", name) {
			runningSince = time.Time{}
			return
		}
		if runningSince.IsZero() {
			runningSince = time.Now()
		}
		// EventuallyWithT treats a tick with no recorded failures as an immediate success, so the
		// stability window must be enforced via an assertion rather than a silent early return —
		// otherwise the very first tick that observes Running would pass right away.
		assert.GreaterOrEqual(c, time.Since(runningSince), minRunningDuration,
			"process %s has not been running long enough yet", name)
	}, 2*time.Minute, 5*time.Second)
}

// AssertProcessNotLoaded fails the test unless dd-procmgrd no longer has name registered.
func (h *Host) AssertProcessNotLoaded(t *testing.T, name string) {
	t.Helper()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, ok, err := h.DescribeProcess(name)
		require.NoError(c, err)
		assert.False(c, ok, "process %s is still loaded by dd-procmgrd", name)
	}, 2*time.Minute, 5*time.Second)
}

// AssertSystemdUnitNotActive fails the test unless every given systemd unit is not active. Linux only.
func (h *Host) AssertSystemdUnitNotActive(t *testing.T, units ...string) {
	t.Helper()
	for _, unit := range units {
		out, err := h.RemoteHost.Execute("systemctl is-active " + unit + " 2>/dev/null || true")
		require.NoError(t, err)
		assert.NotEqual(t, "active", strings.TrimSpace(out), "%s should not be active", unit)
	}
}

// AssertWindowsServiceNotRunning fails the test if the given Windows SCM service is Running.
// A service that doesn't exist counts as not running.
func (h *Host) AssertWindowsServiceNotRunning(t *testing.T, serviceName string) {
	t.Helper()
	_, err := h.RemoteHost.Execute(
		`powershell -NoProfile -Command "$s = Get-Service -Name '` + serviceName + `' -ErrorAction SilentlyContinue; if ($null -eq $s) { exit 0 }; if ($s.Status -eq 'Running') { exit 1 }; exit 0"`)
	require.NoError(t, err, "%s Windows service must not be Running", serviceName)
}

// procmgr COAT telemetry gauge names, reported via `datadog-agent diagnose show-metadata agent-full-telemetry`.
const (
	metricProcmgrDaemonReachable        = "runtime__procmgr_daemon_reachable"
	metricProcmgrDaemonReady            = "runtime__procmgr_daemon_ready"
	metricProcmgrProcessRunning         = "runtime__procmgr_process_running"
	metricAgentServiceInstalled         = "runtime__agent_service_installed"
	metricAgentServiceProcmgrConfigured = "runtime__agent_service_procmgr_configured"
	metricAgentServiceManagementMode    = "runtime__agent_service_management_mode"
	procmgrManagementModeProcmgr        = "procmgr"
)

// AssertProcmgrTelemetry verifies the agent's COAT gauges report serviceID/processName as managed
// by dd-procmgrd. Linux only. Call AssertProcessRunning first so procmgr is reachable and the
// process is running.
func (h *Host) AssertProcmgrTelemetry(t *testing.T, serviceID, processName string) {
	t.Helper()

	// The procmgr reporter refreshes every 5 minutes; poll until gauges reflect the state already
	// confirmed by AssertProcessRunning.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, err := h.RemoteHost.Execute("sudo datadog-agent diagnose show-metadata agent-full-telemetry")
		require.NoError(c, err)

		assertTelemetryGaugeTrue(c, out, metricProcmgrDaemonReachable, nil)
		assertTelemetryGaugeTrue(c, out, metricProcmgrDaemonReady, nil)
		assertTelemetryGaugeTrue(c, out, metricProcmgrProcessRunning, map[string]string{
			"process": processName,
		})
		assertTelemetryGaugeTrue(c, out, metricAgentServiceInstalled, map[string]string{
			"service": serviceID,
		})
		assertTelemetryGaugeTrue(c, out, metricAgentServiceProcmgrConfigured, map[string]string{
			"service": serviceID,
		})
		assertTelemetryGaugeTrue(c, out, metricAgentServiceManagementMode, map[string]string{
			"service": serviceID,
			"mode":    procmgrManagementModeProcmgr,
		})
	}, 7*time.Minute, 10*time.Second, "procmgr telemetry gauges should be emitted")
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
