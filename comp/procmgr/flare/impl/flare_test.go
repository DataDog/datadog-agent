// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package flareimpl

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/pkg/procmgr/coat"
)

type fakeReporter struct {
	report coat.SupportReport
}

func (f fakeReporter) Report(context.Context) coat.SupportReport {
	return f.report
}

// fillFlareWith runs the provider against a mock builder and returns the decoded flare file.
func fillFlareWith(t *testing.T, report coat.SupportReport) (*helpers.FlareBuilderMock, map[string]any) {
	t.Helper()

	p := &procmgrFlare{reporter: fakeReporter{report: report}}
	mock := helpers.NewFlareBuilderMock(t, false)
	require.NoError(t, p.fillFlare(context.Background(), mock))
	require.True(t, mock.AssertFileExists(flareFile))

	raw, err := os.ReadFile(filepath.Join(mock.Root, flareFile))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return mock, decoded
}

func TestFillFlareWritesSupervisedState(t *testing.T) {
	_, decoded := fillFlareWith(t, coat.SupportReport{
		Daemon: coat.DaemonSnapshot{Reachable: true, Ready: true, Version: "1.2.3"},
		Processes: []coat.ProcessSnapshot{
			{Name: "datadog-agent-process", State: coat.ProcessStateRunning, PID: 42, AutoStart: true},
		},
		Services: []coat.ServiceSnapshot{
			{ID: "process", ManagementMode: coat.ManagementModeProcmgr, ProcmgrState: coat.ProcessStateRunning},
		},
		Notes: []string{"how to read this"},
	})

	daemon, ok := decoded["daemon"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, daemon["reachable"])
	assert.Equal(t, "1.2.3", daemon["version"])

	processes, ok := decoded["processes"].([]any)
	require.True(t, ok)
	require.Len(t, processes, 1)
	process, ok := processes[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "datadog-agent-process", process["name"])
	assert.Equal(t, "running", process["state"])
	assert.Equal(t, true, process["auto_start"])

	assert.Contains(t, decoded, "services")
	assert.Contains(t, decoded, "notes")
}

// An unreachable daemon is the case support most needs to see, so the file must still be there.
func TestFillFlareKeepsFileWhenDaemonUnreachable(t *testing.T) {
	_, decoded := fillFlareWith(t, coat.SupportReport{
		SocketPath:  `\\.\pipe\datadog-procmgrd`,
		DaemonError: "connect to dd-procmgrd: file does not exist",
	})

	assert.Equal(t, "connect to dd-procmgrd: file does not exist", decoded["daemon_error"])
	assert.Equal(t, `\\.\pipe\datadog-procmgrd`, decoded["socket_path"])
}

// The notes compare states with ">", which default JSON marshalling would escape to \u003e in a
// file whose only purpose is being read by a person.
func TestFillFlareLeavesComparisonsReadable(t *testing.T) {
	mock, _ := fillFlareWith(t, coat.SupportReport{
		Notes: []string{"state=failed with restart_count>0: a crash loop."},
	})

	raw, err := os.ReadFile(filepath.Join(mock.Root, flareFile))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "restart_count>0")
	assert.NotContains(t, string(raw), `\u003e`)
}

func TestFillFlareScrubsProcessArguments(t *testing.T) {
	mock, _ := fillFlareWith(t, coat.SupportReport{
		Daemon: coat.DaemonSnapshot{Reachable: true, Ready: true},
		Processes: []coat.ProcessSnapshot{
			{
				Name:    "datadog-agent-process",
				State:   coat.ProcessStateRunning,
				Command: "C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent\\process-agent.exe",
				Args:    []string{"--api_key=abcdef0123456789abcdef0123456789"},
			},
		},
	})

	raw, err := os.ReadFile(filepath.Join(mock.Root, flareFile))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "abcdef0123456789abcdef0123456789",
		"process arguments reach the flare and must go through the scrubbing AddFile")
}

// A secret passed as its own argv token is the harder case: the flare's scrubber works line by
// line, and each element of an args array is serialized onto its own line, so by the time it runs
// the value has been separated from the flag that gives it away.
func TestFillFlareScrubsSecretPassedAsSeparateArgument(t *testing.T) {
	mock, _ := fillFlareWith(t, coat.SupportReport{
		Daemon: coat.DaemonSnapshot{Reachable: true, Ready: true},
		Processes: []coat.ProcessSnapshot{
			{
				Name:    "datadog-agent-process",
				State:   coat.ProcessStateRunning,
				Command: "/opt/datadog-agent/embedded/bin/process-agent",
				Args:    []string{"--password", "hunter2-not-in-a-flare", "--verbose"},
			},
		},
	})

	raw, err := os.ReadFile(filepath.Join(mock.Root, flareFile))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "hunter2-not-in-a-flare",
		"a secret in its own argv token must be redacted before the report is serialized")
	assert.Contains(t, string(raw), "--verbose", "non-sensitive arguments should survive")
}
