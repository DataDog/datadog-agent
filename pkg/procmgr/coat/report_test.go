// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeProcmgrConfigFixture(t *testing.T, root string, service MigratableService) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Join(root, processesDirRel), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, processesDirRel, service.ProcmgrConfigFile),
		[]byte("cfg"),
		0o644,
	))
}

func reportProcessByName(t *testing.T, report SupportReport, name string) ProcessSnapshot {
	t.Helper()

	for _, process := range report.Processes {
		if process.Name == name {
			return process
		}
	}
	require.Failf(t, "missing process", "process %q was not reported", name)
	return ProcessSnapshot{}
}

func TestReportIncludesEveryProcessNotJustCatalogServices(t *testing.T) {
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true, RunningProcesses: 1, Version: "1.2.3"},
		processes: map[string]ProcessSnapshot{
			"datadog-agent-process": {Name: "datadog-agent-process", State: ProcessStateRunning, PID: 42},
			"some-other-process":    {Name: "some-other-process", State: ProcessStateRunning, PID: 43},
		},
	})

	report := collector.Report(context.Background())

	require.Len(t, report.Processes, 2,
		"the report must cover every supervised process, not only the migratable catalog")
	assert.Equal(t, "datadog-agent-process", report.Processes[0].Name, "processes must be sorted by name")
	assert.Equal(t, "some-other-process", report.Processes[1].Name)
	assert.Equal(t, "1.2.3", report.Daemon.Version)
	assert.Empty(t, report.DaemonError)
	assert.NotEmpty(t, report.Notes, "the report must explain how to read a process that is down")
}

func TestReportUnreachableDaemonIsRecordedNotDropped(t *testing.T) {
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		connectErr: errors.New("open \\\\.\\pipe\\datadog-procmgrd: file does not exist"),
	})

	report := collector.Report(context.Background())

	assert.False(t, report.Daemon.Reachable)
	assert.Contains(t, report.DaemonError, "connect to dd-procmgrd",
		"a non-answering daemon must produce a report saying so, not an empty file")
	assert.NotEmpty(t, report.SocketPath, "support needs to know which endpoint was tried")
	assert.Empty(t, report.Processes)
	assert.NotEmpty(t, report.Services, "service supervision mapping does not need the daemon")
}

func TestReportStatusFailureIsRecorded(t *testing.T) {
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemonErr: errors.New("deadline exceeded"),
	})

	report := collector.Report(context.Background())

	assert.Contains(t, report.DaemonError, "dd-procmgrd status")
	assert.Contains(t, report.DaemonError, "deadline exceeded")
}

func TestReportListFailureKeepsDaemonState(t *testing.T) {
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemon:  DaemonSnapshot{Reachable: true, Ready: true},
		listErr: errors.New("list failed"),
	})

	report := collector.Report(context.Background())

	assert.True(t, report.Daemon.Reachable, "a failed list must not discard the status we did get")
	assert.Empty(t, report.DaemonError)
	assert.Contains(t, report.ProcessesError, "dd-procmgrd list")
}

func TestReportDescribeFailureFallsBackToListData(t *testing.T) {
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true},
		processes: map[string]ProcessSnapshot{
			"datadog-agent-process": {Name: "datadog-agent-process", State: ProcessStateRunning, PID: 42},
		},
		describeErr: errors.New("describe failed"),
	})

	report := collector.Report(context.Background())

	process := reportProcessByName(t, report, "datadog-agent-process")
	assert.Equal(t, uint32(42), process.PID, "a failed describe must keep the data List already gave us")
	require.Len(t, report.Warnings, 1)
	assert.Contains(t, report.Warnings[0], "describe datadog-agent-process")
}

// A gate-blocked process and an inert catalog entry are both Created, and auto_start is the only
// field that separates them. It comes from Describe, so losing that call loses the distinction.
func TestReportDistinguishesGatedProcessFromInertCatalogEntry(t *testing.T) {
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true},
		processes: map[string]ProcessSnapshot{
			"datadog-agent-process":  {Name: "datadog-agent-process", State: ProcessStateCreated},
			"datadog-agent-sysprobe": {Name: "datadog-agent-sysprobe", State: ProcessStateCreated},
		},
		details: map[string]ProcessSnapshot{
			"datadog-agent-process": {
				Name:      "datadog-agent-process",
				State:     ProcessStateCreated,
				AutoStart: true,
			},
			"datadog-agent-sysprobe": {
				Name:      "datadog-agent-sysprobe",
				State:     ProcessStateCreated,
				AutoStart: false,
			},
		},
	})

	report := collector.Report(context.Background())

	gated := reportProcessByName(t, report, "datadog-agent-process")
	assert.Equal(t, ProcessStateCreated, gated.State)
	assert.True(t, gated.AutoStart, "auto_start is what marks this as blocked rather than inert")

	inert := reportProcessByName(t, report, "datadog-agent-sysprobe")
	assert.Equal(t, ProcessStateCreated, inert.State)
	assert.False(t, inert.AutoStart)
}

func TestReportSeparatesCrashLoopFromFailedSpawn(t *testing.T) {
	exitCode := int32(2)
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true},
		processes: map[string]ProcessSnapshot{
			"crash-looper": {
				Name:         "crash-looper",
				State:        ProcessStateCrashed,
				RestartCount: 5,
				LastExitCode: &exitCode,
			},
			"failed-spawn": {Name: "failed-spawn", State: ProcessStateFailed, RestartCount: 0},
		},
	})

	report := collector.Report(context.Background())

	looper := reportProcessByName(t, report, "crash-looper")
	assert.Equal(t, uint32(5), looper.RestartCount)
	require.NotNil(t, looper.LastExitCode, "a crash loop must carry the exit code that support needs")
	assert.Equal(t, int32(2), *looper.LastExitCode)

	spawn := reportProcessByName(t, report, "failed-spawn")
	assert.Zero(t, spawn.RestartCount)
	assert.Nil(t, spawn.LastExitCode, "never having exited must stay distinct from exiting with code 0")
}

// The report is only useful to support if a Stopped legacy service can be explained, which is
// what the per-service management mode does.
func TestReportServicesExplainStoppedLegacyService(t *testing.T) {
	sysprobe, ok := serviceByID("sysprobe")
	require.True(t, ok, "system-probe must be in the migratable catalog or its flare row is silent")
	assert.Equal(t, "datadog-system-probe", sysprobe.LegacyWindowsService)

	root := t.TempDir()
	writeProcmgrConfigFixture(t, root, sysprobe)

	collector := NewCollectorWithClient(root, &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true, RunningProcesses: 1},
		processes: map[string]ProcessSnapshot{
			sysprobe.ProcmgrProcessName: {Name: sysprobe.ProcmgrProcessName, State: ProcessStateRunning},
		},
	})

	report := collector.Report(context.Background())

	var found bool
	for _, service := range report.Services {
		if service.ID != "sysprobe" {
			continue
		}
		found = true
		assert.Equal(t, ManagementModeProcmgr, service.ManagementMode)
		assert.Equal(t, ProcessStateRunning, service.ProcmgrState)
	}
	assert.True(t, found, "the report must carry a row for every migratable service")
}

func TestReportMarshalsToReadableJSON(t *testing.T) {
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true, Version: "1.2.3", UptimeSeconds: 90},
		processes: map[string]ProcessSnapshot{
			"datadog-agent-process": {Name: "datadog-agent-process", State: ProcessStateRunning, PID: 42},
		},
	})

	raw, err := json.Marshal(collector.Report(context.Background()))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Contains(t, decoded, "daemon")
	assert.Contains(t, decoded, "processes")
	assert.Contains(t, decoded, "services")
	assert.Contains(t, string(raw), `"uptime_seconds":90`, "keys must be snake_case for support readability")
	assert.Contains(t, string(raw), `"management_mode"`)
}

func TestReportRedactsSecretArguments(t *testing.T) {
	windowsCommand := `C:\Program Files\Datadog\Datadog Agent\bin\agent\process-agent.exe`
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true},
		processes: map[string]ProcessSnapshot{
			"datadog-agent-process": {Name: "datadog-agent-process", State: ProcessStateRunning},
		},
		details: map[string]ProcessSnapshot{
			"datadog-agent-process": {
				Name:    "datadog-agent-process",
				State:   ProcessStateRunning,
				Command: windowsCommand,
				Args: []string{
					"--password", "separate-token-secret",
					"--api_key=inline-secret",
					"--config", `C:\Program Files\Datadog\datadog.yaml`,
				},
			},
		},
	})

	proc := reportProcessByName(t, collector.Report(context.Background()), "datadog-agent-process")

	assert.NotContains(t, proc.Args, "separate-token-secret",
		"a value in its own argv token must be redacted, the flare's line-based scrubber cannot pair it")
	assert.NotContains(t, proc.Args, "--api_key=inline-secret")
	assert.Contains(t, proc.Args, "--api_key="+redactedValue)

	// Not mangling these is why the pairing is done here rather than through
	// redact.ScrubSimpleCommand, which would re-tokenize both on their spaces.
	assert.Equal(t, windowsCommand, proc.Command, "the executable path must survive intact")
	assert.Contains(t, proc.Args, `C:\Program Files\Datadog\datadog.yaml`,
		"a non-sensitive value containing spaces must survive intact")
}

func TestScrubProcessArgsIsIdempotent(t *testing.T) {
	processes := []ProcessSnapshot{{Args: []string{"--password", "s3cret", "--verbose"}}}

	scrubProcessArgs(processes)
	once := append([]string{}, processes[0].Args...)
	scrubProcessArgs(processes)

	assert.Equal(t, once, processes[0].Args)
	assert.Equal(t, []string{"--password", redactedValue, "--verbose"}, processes[0].Args)
}
