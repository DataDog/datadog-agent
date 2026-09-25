// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockClient struct {
	connectErr error
	daemon     DaemonSnapshot
	daemonErr  error
	processes  map[string]ProcessSnapshot
	listErr    error
}

func (m *mockClient) Connect(context.Context) (ProcmgrSession, error) {
	if m.connectErr != nil {
		return nil, m.connectErr
	}
	return &mockSession{m: m}, nil
}

type mockSession struct {
	m *mockClient
}

func (s *mockSession) Status(context.Context) (DaemonSnapshot, error) {
	if s.m.daemonErr != nil {
		return DaemonSnapshot{}, s.m.daemonErr
	}
	return s.m.daemon, nil
}

func (s *mockSession) List(context.Context) (map[string]ProcessSnapshot, error) {
	if s.m.listErr != nil {
		return nil, s.m.listErr
	}
	procs := s.m.processes
	if procs == nil {
		procs = map[string]ProcessSnapshot{}
	}
	return procs, nil
}

func (s *mockSession) Disconnect() error {
	return nil
}

func serviceSnapshotByID(t *testing.T, snapshot Snapshot, id string) ServiceSnapshot {
	t.Helper()

	for _, service := range snapshot.Services {
		if service.ID == id {
			return service
		}
	}
	require.Failf(t, "missing service snapshot", "service %q was not collected", id)
	return ServiceSnapshot{}
}

func installMarkerForTest(t *testing.T, root string, service MigratableService, index int) string {
	t.Helper()

	markers := installMarkerPaths(root, service)
	require.Greater(t, len(markers), index)
	return markers[index]
}

func setupDDOTInstallFixture(t *testing.T) string {
	t.Helper()

	ddot, ok := serviceByID("ddot")
	require.True(t, ok)

	root := t.TempDir()
	marker := installMarkerForTest(t, root, ddot, 0)
	require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0o755))
	require.NoError(t, os.WriteFile(marker, []byte("bin"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, processesDirRel), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, processesDirRel, ddot.ProcmgrConfigFile),
		[]byte("cfg"),
		0o644,
	))
	return root
}

func TestCollectInstalledViaStandaloneMarkerOnly(t *testing.T) {
	ddot, ok := serviceByID("ddot")
	require.True(t, ok)

	root := t.TempDir()
	standalone := installMarkerForTest(t, root, ddot, 1)
	require.NoError(t, os.MkdirAll(filepath.Dir(standalone), 0o755))
	require.NoError(t, os.WriteFile(standalone, []byte("bin"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, processesDirRel), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, processesDirRel, ddot.ProcmgrConfigFile),
		[]byte("cfg"),
		0o644,
	))

	collector := NewCollectorWithClient(root, &mockClient{})

	snapshot := collector.Collect(context.Background())
	service := serviceSnapshotByID(t, snapshot, "ddot")
	assert.True(t, service.Installed,
		"standalone datadog-agent-ddot layout uses embedded/bin/otel-agent without ext/ddot")
}

func TestCollectServiceProcmgrRunning(t *testing.T) {
	root := setupDDOTInstallFixture(t)

	collector := NewCollectorWithClient(root, &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true, RunningProcesses: 1},
		processes: map[string]ProcessSnapshot{
			"datadog-agent-ddot": {Name: "datadog-agent-ddot", State: ProcessStateRunning},
		},
	})

	snapshot := collector.Collect(context.Background())

	service := serviceSnapshotByID(t, snapshot, "ddot")
	assert.Equal(t, "ddot", service.ID)
	assert.True(t, service.Installed)
	assert.True(t, service.ProcmgrConfigured)
	assert.Equal(t, ProcessStateRunning, service.ProcmgrState)
	assert.Equal(t, ManagementModeProcmgr, service.ManagementMode)
	assert.True(t, snapshot.Daemon.Reachable)
	assert.True(t, snapshot.Daemon.Ready)
}

func TestCollectADPProcmgrRunning(t *testing.T) {
	adp, ok := serviceByID("agent-data-plane")
	require.True(t, ok)

	root := t.TempDir()
	marker := installMarkerForTest(t, root, adp, 0)
	require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0o755))
	require.NoError(t, os.WriteFile(marker, []byte("bin"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, processesDirRel), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, processesDirRel, "datadog-agent-data-plane.yaml"),
		[]byte("cfg"),
		0o644,
	))

	collector := NewCollectorWithClient(root, &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true, RunningProcesses: 1},
		processes: map[string]ProcessSnapshot{
			"datadog-agent-data-plane": {Name: "datadog-agent-data-plane", State: ProcessStateRunning},
		},
	})

	snapshot := collector.Collect(context.Background())

	service := serviceSnapshotByID(t, snapshot, "agent-data-plane")
	assert.Equal(t, "agent-data-plane", service.ID)
	assert.True(t, service.Installed)
	assert.True(t, service.ProcmgrConfigured)
	assert.Equal(t, ProcessStateRunning, service.ProcmgrState)
	assert.Equal(t, ManagementModeProcmgr, service.ManagementMode)
}

func TestCollectServiceProcmgrNotRunningStillManaged(t *testing.T) {
	root := setupDDOTInstallFixture(t)

	collector := NewCollectorWithClient(root, &mockClient{
		processes: map[string]ProcessSnapshot{
			"datadog-agent-ddot": {Name: "datadog-agent-ddot", State: ProcessStateStarting},
		},
	})

	snapshot := collector.Collect(context.Background())

	service := serviceSnapshotByID(t, snapshot, "ddot")
	assert.Equal(t, ManagementModeProcmgr, service.ManagementMode)
	assert.Equal(t, ProcessStateStarting, service.ProcmgrState)
}

func TestCollectNoProcmgrNoLegacy(t *testing.T) {
	root := t.TempDir()

	collector := NewCollectorWithClient(root, &mockClient{})

	snapshot := collector.Collect(context.Background())

	service := serviceSnapshotByID(t, snapshot, "ddot")
	assert.False(t, service.Installed)
	assert.False(t, service.ProcmgrConfigured)
	assert.Equal(t, ManagementModeNone, service.ManagementMode)
	assert.Equal(t, ProcessStateUnknown, service.ProcmgrState)
}

func TestCollectInstallMarkerAbsent(t *testing.T) {
	ddot, ok := serviceByID("ddot")
	require.True(t, ok)

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, processesDirRel), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, processesDirRel, ddot.ProcmgrConfigFile),
		[]byte("cfg"),
		0o644,
	))

	collector := NewCollectorWithClient(root, &mockClient{})

	snapshot := collector.Collect(context.Background())

	service := serviceSnapshotByID(t, snapshot, "ddot")
	assert.False(t, service.Installed, "without install marker, Installed must stay false")
	assert.True(t, service.ProcmgrConfigured)
	assert.Equal(t, ManagementModeNone, service.ManagementMode)
}

func TestCollectProcmgrConfigAbsent(t *testing.T) {
	ddot, ok := serviceByID("ddot")
	require.True(t, ok)

	root := t.TempDir()
	marker := installMarkerForTest(t, root, ddot, 0)
	require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0o755))
	require.NoError(t, os.WriteFile(marker, []byte("bin"), 0o644))

	collector := NewCollectorWithClient(root, &mockClient{})

	snapshot := collector.Collect(context.Background())

	service := serviceSnapshotByID(t, snapshot, "ddot")
	assert.True(t, service.Installed)
	assert.False(t, service.ProcmgrConfigured)
	assert.Equal(t, ManagementModeNone, service.ManagementMode)
}

func TestCollectDaemonUnreachable(t *testing.T) {
	root := setupDDOTInstallFixture(t)

	collector := NewCollectorWithClient(root, &mockClient{
		daemonErr: errors.New("dial failed"),
		processes: map[string]ProcessSnapshot{
			"datadog-agent-ddot": {Name: "datadog-agent-ddot", State: ProcessStateRunning},
		},
	})

	snapshot := collector.Collect(context.Background())

	assert.False(t, snapshot.Daemon.Reachable, "daemon status error should yield empty snapshot")
	assert.False(t, snapshot.Daemon.Ready)
	service := serviceSnapshotByID(t, snapshot, "ddot")
	assert.Equal(t, ManagementModeNone, service.ManagementMode,
		"daemon failure prevents listing processes")
	assert.Equal(t, ProcessStateUnknown, service.ProcmgrState)
}

func TestCollectDaemonReachableListFails(t *testing.T) {
	root := setupDDOTInstallFixture(t)

	collector := NewCollectorWithClient(root, &mockClient{
		daemon:    DaemonSnapshot{Reachable: true, Ready: true, RunningProcesses: 1},
		listErr:   errors.New("list failed"),
		processes: map[string]ProcessSnapshot{"datadog-agent-ddot": {Name: "datadog-agent-ddot", State: ProcessStateRunning}},
	})

	snapshot := collector.Collect(context.Background())

	assert.True(t, snapshot.Daemon.Reachable)
	assert.True(t, snapshot.Daemon.Ready)
	service := serviceSnapshotByID(t, snapshot, "ddot")
	assert.Equal(t, ManagementModeNone, service.ManagementMode)
	assert.Equal(t, ProcessStateUnknown, service.ProcmgrState)
}
