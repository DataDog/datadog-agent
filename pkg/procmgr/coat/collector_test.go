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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockClient struct {
	connectErr error
	daemon     DaemonSnapshot
	daemonErr  error
	processes  map[string]ProcessSnapshot
	listErr    error
	// details overrides what Describe returns for a process name. Names absent from it fall
	// back to the List entry, so tests only specify Describe-only fields when they matter.
	details     map[string]ProcessSnapshot
	describeErr error
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

func (s *mockSession) Describe(_ context.Context, nameOrUUID string) (ProcessSnapshot, error) {
	if s.m.describeErr != nil {
		return ProcessSnapshot{}, s.m.describeErr
	}
	if detail, ok := s.m.details[nameOrUUID]; ok {
		return detail, nil
	}
	if listed, ok := s.m.processes[nameOrUUID]; ok {
		return listed, nil
	}
	return ProcessSnapshot{}, errors.New("no such process")
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

// requireNoInstallMarkers asserts the "no install marker" premise the absent-marker tests rely on.
// Most marker paths live under the test's temp root, but on Windows one points at the machine-wide
// fleet packages directory, so a real install on the host would otherwise make those tests pass or
// fail for the wrong reason.
func requireNoInstallMarkers(t *testing.T, root string, service MigratableService) {
	t.Helper()

	for _, marker := range installMarkerPaths(root, service) {
		if marker == "" {
			continue
		}
		require.NoFileExists(t, marker,
			"test requires a host with no %s install marker on disk", service.ID)
	}
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

func TestCollectProcessProcmgrRunning(t *testing.T) {
	process, ok := serviceByID("process")
	require.True(t, ok)
	assert.Equal(t, "datadog-process-agent", process.LegacyWindowsService)

	root := t.TempDir()
	marker := installMarkerForTest(t, root, process, 0)
	require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0o755))
	require.NoError(t, os.WriteFile(marker, []byte("bin"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, processesDirRel), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, processesDirRel, process.ProcmgrConfigFile),
		[]byte("cfg"),
		0o644,
	))

	collector := NewCollectorWithClient(root, &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true, RunningProcesses: 1},
		processes: map[string]ProcessSnapshot{
			"datadog-agent-process": {Name: "datadog-agent-process", State: ProcessStateRunning},
		},
	})

	snapshot := collector.Collect(context.Background())

	service := serviceSnapshotByID(t, snapshot, "process")
	assert.Equal(t, "process", service.ID)
	assert.True(t, service.Installed)
	assert.True(t, service.ProcmgrConfigured)
	assert.Equal(t, ProcessStateRunning, service.ProcmgrState)
	assert.Equal(t, ManagementModeProcmgr, service.ManagementMode)
}

// Catalog entries must name the same process the processes.d basename implies. A mismatch would
// leave management_mode stuck at none even when dd-procmgrd is supervising the service.
func TestMigratableServicesProcessNameMatchesConfigFile(t *testing.T) {
	for _, service := range migratableServices {
		want := strings.TrimSuffix(service.ProcmgrConfigFile, ".yaml")
		assert.Equal(t, want, service.ProcmgrProcessName,
			"service %q: ProcmgrProcessName must be the processes.d basename without .yaml", service.ID)
		assert.NotEmpty(t, service.ProcmgrConfigFile)
		assert.NotEmpty(t, service.InstallMarkerRels, "service %q needs an install marker", service.ID)
	}
}

func TestCollectActionProcmgrRunning(t *testing.T) {
	action, ok := serviceByID("action")
	require.True(t, ok)

	root := t.TempDir()
	marker := installMarkerForTest(t, root, action, 0)
	require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0o755))
	require.NoError(t, os.WriteFile(marker, []byte("bin"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, processesDirRel), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, processesDirRel, action.ProcmgrConfigFile),
		[]byte("cfg"),
		0o644,
	))

	collector := NewCollectorWithClient(root, &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true, RunningProcesses: 1},
		processes: map[string]ProcessSnapshot{
			action.ProcmgrProcessName: {Name: action.ProcmgrProcessName, State: ProcessStateRunning},
		},
	})

	service := serviceSnapshotByID(t, collector.Collect(context.Background()), "action")
	assert.True(t, service.Installed)
	assert.True(t, service.ProcmgrConfigured)
	assert.Equal(t, ProcessStateRunning, service.ProcmgrState)
	assert.Equal(t, ManagementModeProcmgr, service.ManagementMode)
}

func TestCollectPARControlProcmgrRunningWithoutLegacyUnit(t *testing.T) {
	control, ok := serviceByID("par-control")
	require.True(t, ok)

	root := t.TempDir()
	marker := installMarkerForTest(t, root, control, 0)
	require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0o755))
	require.NoError(t, os.WriteFile(marker, []byte("bin"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, processesDirRel), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, processesDirRel, control.ProcmgrConfigFile),
		[]byte("cfg"),
		0o644,
	))

	collector := NewCollectorWithClient(root, &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true, RunningProcesses: 1},
		processes: map[string]ProcessSnapshot{
			control.ProcmgrProcessName: {Name: control.ProcmgrProcessName, State: ProcessStateRunning},
		},
	})

	service := serviceSnapshotByID(t, collector.Collect(context.Background()), "par-control")
	assert.True(t, service.Installed)
	assert.True(t, service.ProcmgrConfigured)
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
	requireNoInstallMarkers(t, root, ddot)

	collector := NewCollectorWithClient(root, &mockClient{})

	snapshot := collector.Collect(context.Background())

	service := serviceSnapshotByID(t, snapshot, "ddot")
	assert.False(t, service.Installed, "without install marker, Installed must stay false")
	assert.True(t, service.ProcmgrConfigured)
	assert.Equal(t, ManagementModeNone, service.ManagementMode)
}

func TestCollectInstallMarkerAbsentButProcmgrSupervises(t *testing.T) {
	ddot, ok := serviceByID("ddot")
	require.True(t, ok)

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, processesDirRel), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, processesDirRel, ddot.ProcmgrConfigFile),
		[]byte("cfg"),
		0o644,
	))
	requireNoInstallMarkers(t, root, ddot)

	collector := NewCollectorWithClient(root, &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true, RunningProcesses: 1},
		processes: map[string]ProcessSnapshot{
			"datadog-agent-ddot": {Name: "datadog-agent-ddot", State: ProcessStateRunning},
		},
	})

	snapshot := collector.Collect(context.Background())

	service := serviceSnapshotByID(t, snapshot, "ddot")
	assert.True(t, service.Installed,
		"procmgr supervision is install evidence when no marker path matches the layout")
	assert.Equal(t, ManagementModeProcmgr, service.ManagementMode)
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

// deadlineRecordingClient records the budget it was handed and fails at once, so a test can assert
// on what Collect passed down without waiting out a real timeout.
type deadlineRecordingClient struct {
	deadline    time.Time
	hasDeadline bool
}

func (c *deadlineRecordingClient) Connect(ctx context.Context) (ProcmgrSession, error) {
	c.deadline, c.hasDeadline = ctx.Deadline()
	return nil, errors.New("dial failed")
}

// The per-service supervisor checks are local, so they are the one part of a snapshot still worth
// having when dd-procmgrd is what failed. They share the collection context, and a context cannot
// outlive an expired parent, so a daemon that hangs until the budget is gone would otherwise leave
// every "systemctl is-active" failing on arrival and every service reporting management_mode "none":
// no supervisor owns this, rather than we could not tell.
func TestCollectLeavesTimeForTheServiceSweepWhenTheDaemonHangs(t *testing.T) {
	client := &deadlineRecordingClient{}
	collector := NewCollectorWithClient(t.TempDir(), client)

	start := time.Now()
	collector.Collect(context.Background())

	require.True(t, client.hasDeadline, "collection must bound every call it makes")
	// Tolerance well under the reserve: at a tolerance of the reserve itself this would hold whether
	// or not any time was actually held back.
	assert.WithinDuration(t, start.Add(clientTimeout-serviceSweepReserve), client.deadline,
		serviceSweepReserve/4,
		"the daemon calls get the collection budget less the reserve, so a hung daemon cannot starve the service sweep")
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
