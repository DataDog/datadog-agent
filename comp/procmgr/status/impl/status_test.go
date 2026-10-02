// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package statusimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/procmgr/coat"
)

type fakeReporter struct {
	report coat.SupportReport
}

func (f fakeReporter) Report(context.Context, coat.ScrubOptions) coat.SupportReport {
	return f.report
}

func providerWith(report coat.SupportReport, scrub coat.ScrubOptions) statusProvider {
	return statusProvider{
		reporter: fakeReporter{report: report},
		scrub:    scrub,
	}
}

func exitCode(code int32) *int32 { return &code }

func TestStatusShowsReachableDaemonAndProcess(t *testing.T) {
	provider := providerWith(coat.SupportReport{
		SocketPath: "/var/run/datadog-procmgrd/dd-procmgrd.sock",
		Daemon: coat.DaemonSnapshot{
			Reachable:        true,
			Ready:            true,
			Version:          "1.2.3",
			UptimeSeconds:    90,
			RunningProcesses: 1,
			TotalProcesses:   1,
			ServiceState:     "running",
		},
		Processes: []coat.ProcessSnapshot{
			{
				Name:         "datadog-agent-process",
				State:        coat.ProcessStateRunning,
				PID:          42,
				RestartCount: 0,
				Profile:      "default",
				User:         "dd-agent",
			},
		},
		Services: []coat.ServiceSnapshot{
			{
				ID:                "process",
				Installed:         true,
				ProcmgrConfigured: true,
				ManagementMode:    coat.ManagementModeProcmgr,
				ProcmgrState:      coat.ProcessStateRunning,
			},
		},
		Notes: []string{"state=running: the process is supervised and up."},
	}, coat.ScrubOptions{})

	stats := make(map[string]interface{})
	require.NoError(t, provider.JSON(false, stats))
	require.Contains(t, stats, "processManager")

	report, ok := stats["processManager"].(coat.SupportReport)
	require.True(t, ok)
	assert.True(t, report.Daemon.Reachable)
	assert.True(t, report.Daemon.Ready)
	require.Len(t, report.Processes, 1)
	assert.Equal(t, "datadog-agent-process", report.Processes[0].Name)

	var text bytes.Buffer
	require.NoError(t, provider.Text(false, &text))
	out := text.String()
	assert.Contains(t, out, "Reachable: true")
	assert.Contains(t, out, "Ready: true")
	assert.Contains(t, out, "Service State: running")
	assert.Contains(t, out, "datadog-agent-process: state=running pid=42 restarts=0")
	assert.Contains(t, out, "process: installed=true configured=true mode=procmgr procmgr_state=running")
	assert.Contains(t, out, "procmgr/state.json")
}

func TestStatusKeepsSectionWhenDaemonUnreachable(t *testing.T) {
	provider := providerWith(coat.SupportReport{
		SocketPath:  `\\.\pipe\datadog-procmgrd`,
		DaemonError: "connect to dd-procmgrd: file does not exist",
		Processes:   []coat.ProcessSnapshot{},
		Services:    []coat.ServiceSnapshot{},
	}, coat.ScrubOptions{})

	stats := make(map[string]interface{})
	require.NoError(t, provider.JSON(false, stats))
	require.Contains(t, stats, "processManager")

	report, ok := stats["processManager"].(coat.SupportReport)
	require.True(t, ok)
	assert.Equal(t, "connect to dd-procmgrd: file does not exist", report.DaemonError)
	assert.False(t, report.Daemon.Reachable)

	var text bytes.Buffer
	require.NoError(t, provider.Text(false, &text))
	out := text.String()
	assert.Contains(t, out, "Daemon Error: connect to dd-procmgrd: file does not exist")
	assert.Contains(t, out, "Reachable: false")
}

func TestStatusShowsProcmgrAndSystemdManagementModes(t *testing.T) {
	provider := providerWith(coat.SupportReport{
		Daemon: coat.DaemonSnapshot{Reachable: true, Ready: true},
		Services: []coat.ServiceSnapshot{
			{
				ID:                "process",
				Installed:         true,
				ProcmgrConfigured: true,
				ManagementMode:    coat.ManagementModeProcmgr,
				ProcmgrState:      coat.ProcessStateStopped,
			},
			{
				ID:                "trace",
				Installed:         true,
				ProcmgrConfigured: false,
				ManagementMode:    coat.ManagementModeSystemd,
				ProcmgrState:      coat.ProcessStateCreated,
			},
		},
	}, coat.ScrubOptions{})

	var text bytes.Buffer
	require.NoError(t, provider.Text(false, &text))
	out := text.String()
	assert.Contains(t, out, "process: installed=true configured=true mode=procmgr procmgr_state=stopped")
	assert.Contains(t, out, "trace: installed=true configured=false mode=systemd procmgr_state=created")
}

func TestStatusShowsExitAndSignalWhenSet(t *testing.T) {
	code := exitCode(1)
	sig := exitCode(9)
	provider := providerWith(coat.SupportReport{
		Daemon: coat.DaemonSnapshot{Reachable: true, Ready: true},
		Processes: []coat.ProcessSnapshot{{
			Name:         "datadog-agent-process",
			State:        coat.ProcessStateFailed,
			PID:          0,
			RestartCount: 2,
			LastExitCode: code,
			LastSignal:   sig,
		}},
	}, coat.ScrubOptions{})

	var text bytes.Buffer
	require.NoError(t, provider.Text(false, &text))
	out := text.String()
	assert.Contains(t, out, "last_exit_code=1")
	assert.Contains(t, out, "last_signal=9")
}

func TestStatusJSONScrubsProcessArguments(t *testing.T) {
	provider := providerWith(coat.SupportReport{
		Daemon: coat.DaemonSnapshot{Reachable: true, Ready: true},
		Processes: []coat.ProcessSnapshot{{
			Name:    "datadog-agent-process",
			State:   coat.ProcessStateRunning,
			Command: "/opt/datadog-agent/embedded/bin/process-agent",
			Args:    []string{"--password", "hunter2-not-in-status", "--verbose"},
		}},
	}, coat.ScrubOptions{})

	stats := make(map[string]interface{})
	require.NoError(t, provider.JSON(false, stats))

	raw, err := json.Marshal(stats["processManager"])
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "hunter2-not-in-status")
	assert.Contains(t, string(raw), "********")
	assert.Contains(t, string(raw), "--verbose")
}

func TestStatusJSONHonoursStripProcArguments(t *testing.T) {
	provider := providerWith(coat.SupportReport{
		Daemon: coat.DaemonSnapshot{Reachable: true, Ready: true},
		Processes: []coat.ProcessSnapshot{{
			Name:    "datadog-agent-process",
			State:   coat.ProcessStateRunning,
			Command: "/opt/datadog-agent/embedded/bin/process-agent",
			Args:    []string{"--config", "/etc/datadog-agent/datadog.yaml"},
		}},
	}, coat.ScrubOptions{StripArguments: true})

	stats := make(map[string]interface{})
	require.NoError(t, provider.JSON(false, stats))

	raw, err := json.Marshal(stats["processManager"])
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "--config")
	assert.Contains(t, string(raw), "process-agent")
}

func TestStatusHTMLRenders(t *testing.T) {
	provider := providerWith(coat.SupportReport{
		SocketPath:  "/var/run/datadog-procmgrd/dd-procmgrd.sock",
		DaemonError: "connect to dd-procmgrd: connection refused",
	}, coat.ScrubOptions{})

	var html bytes.Buffer
	require.NoError(t, provider.HTML(false, &html))
	out := html.String()
	assert.Contains(t, out, "Process Manager")
	assert.Contains(t, out, "Daemon Error: connect to dd-procmgrd: connection refused")
}

func TestScrubOptionsComeFromAgentConfig(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetInTest("process_config.custom_sensitive_words", []string{"passphrase"})
	cfg.SetInTest("process_config.strip_proc_arguments", true)

	opts := scrubOptionsFromConfig(cfg)

	assert.Equal(t, []string{"passphrase"}, opts.CustomSensitiveWords)
	assert.True(t, opts.StripArguments)
}

func TestNewComponentProvidesStatusProvider(t *testing.T) {
	provides := NewComponent(Requires{Config: config.NewMock(t)})
	require.NotNil(t, provides.Status.Provider)
	assert.Equal(t, "Process Manager", provides.Status.Provider.Name())
	assert.Equal(t, "Process Manager", provides.Status.Provider.Section())
}
