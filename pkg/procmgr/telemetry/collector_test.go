// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockClient struct {
	daemon    DaemonSnapshot
	processes map[string]ProcessSnapshot
}

func (m *mockClient) DaemonStatus(context.Context) (DaemonSnapshot, error) {
	return m.daemon, nil
}

func (m *mockClient) ListProcesses(context.Context) (map[string]ProcessSnapshot, error) {
	return m.processes, nil
}

func TestCollectServiceProcmgrRunning(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "ext", "ddot", "embedded", "bin", "otel-agent")
	require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0o755))
	require.NoError(t, os.WriteFile(marker, []byte("bin"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "processes.d"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "processes.d", "datadog-agent-ddot.yaml"), []byte("cfg"), 0o644))

	collector := NewCollectorWithClient(root, &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true, RunningProcesses: 1},
		processes: map[string]ProcessSnapshot{
			"datadog-agent-ddot": {Name: "datadog-agent-ddot", State: "Running"},
		},
	})

	snapshot := collector.Collect(context.Background())
	require.Len(t, snapshot.Services, 1)

	service := snapshot.Services[0]
	assert.Equal(t, "ddot", service.ID)
	assert.True(t, service.Installed)
	assert.True(t, service.ProcmgrConfigured)
	assert.Equal(t, "Running", service.ProcmgrState)
	assert.Equal(t, ManagementModeProcmgr, service.ManagementMode)
	assert.True(t, snapshot.Daemon.Reachable)
	assert.True(t, snapshot.Daemon.Ready)
}

func TestCollectServiceProcmgrNotRunningStillManaged(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "ext", "ddot", "embedded", "bin", "otel-agent")
	require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0o755))
	require.NoError(t, os.WriteFile(marker, []byte("bin"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "processes.d"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "processes.d", "datadog-agent-ddot.yaml"), []byte("cfg"), 0o644))

	collector := NewCollectorWithClient(root, &mockClient{
		processes: map[string]ProcessSnapshot{
			"datadog-agent-ddot": {Name: "datadog-agent-ddot", State: "Starting"},
		},
	})

	snapshot := collector.Collect(context.Background())
	require.Len(t, snapshot.Services, 1)

	service := snapshot.Services[0]
	assert.Equal(t, ManagementModeProcmgr, service.ManagementMode)
	assert.Equal(t, "Starting", service.ProcmgrState)
}

func TestCollectServiceLegacySystemd(t *testing.T) {
	if detectLegacySupervisor(migratableServices[0]) == ManagementModeNone {
		t.Skip("legacy supervisor detection is platform-specific")
	}

	root := t.TempDir()
	marker := filepath.Join(root, "ext", "ddot", "embedded", "bin", "otel-agent")
	require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0o755))
	require.NoError(t, os.WriteFile(marker, []byte("bin"), 0o644))

	collector := NewCollectorWithClient(root, &mockClient{
		processes: map[string]ProcessSnapshot{},
	})

	snapshot := collector.Collect(context.Background())
	require.Len(t, snapshot.Services, 1)

	service := snapshot.Services[0]
	if service.ManagementMode != ManagementModeSystemd {
		t.Skip("systemd units not active in test environment")
	}
	assert.True(t, service.Installed)
	assert.Equal(t, ManagementModeSystemd, service.ManagementMode)
}
