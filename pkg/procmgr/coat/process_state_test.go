// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
)

func TestServiceProcessStateProcmgrManaged(t *testing.T) {
	states := map[pb.ProcessState]string{
		pb.ProcessState_UNKNOWN:  ProcessStateUnknown,
		pb.ProcessState_CREATED:  ProcessStateCreated,
		pb.ProcessState_STARTING: ProcessStateStarting,
		pb.ProcessState_RUNNING:  ProcessStateRunning,
		pb.ProcessState_STOPPING: ProcessStateStopping,
		pb.ProcessState_STOPPED:  ProcessStateStopped,
		pb.ProcessState_CRASHED:  ProcessStateCrashed,
		pb.ProcessState_EXITED:   ProcessStateExited,
		pb.ProcessState_FAILED:   ProcessStateFailed,
	}

	for state, expected := range states {
		t.Run(state.String(), func(t *testing.T) {
			snapshot := Snapshot{
				Daemon: DaemonSnapshot{Reachable: true, Ready: true},
				Services: []ServiceSnapshot{{
					ID:                ServiceIDDDOT,
					Installed:         true,
					ProcmgrConfigured: true,
					ProcmgrState:      state,
					ManagementMode:    ManagementModeProcmgr,
				}},
			}
			assert.Equal(t, expected, snapshot.ServiceProcessState(ServiceIDDDOT))
		})
	}
}

func TestServiceProcessStateLegacySupervisors(t *testing.T) {
	for _, mode := range []ManagementMode{ManagementModeSystemd, ManagementModeWindowsService} {
		t.Run(string(mode), func(t *testing.T) {
			snapshot := Snapshot{
				Services: []ServiceSnapshot{{
					ID:             ServiceIDDDOT,
					Installed:      true,
					ManagementMode: mode,
				}},
			}
			assert.Equal(t, ProcessStateRunning, snapshot.ServiceProcessState(ServiceIDDDOT))
		})
	}
}

func TestServiceProcessStateNotInstalled(t *testing.T) {
	snapshot := Snapshot{
		Daemon: DaemonSnapshot{Reachable: true, Ready: true},
		Services: []ServiceSnapshot{{
			ID:             ServiceIDDDOT,
			ManagementMode: ManagementModeNone,
		}},
	}
	assert.Equal(t, ProcessStateNotInstalled, snapshot.ServiceProcessState(ServiceIDDDOT))
}

func TestServiceProcessStateProcmgrUnreachable(t *testing.T) {
	snapshot := Snapshot{
		Daemon: DaemonSnapshot{},
		Services: []ServiceSnapshot{{
			ID:                ServiceIDDDOT,
			Installed:         true,
			ProcmgrConfigured: true,
			ManagementMode:    ManagementModeNone,
		}},
	}
	assert.Equal(t, ProcessStateUnknown, snapshot.ServiceProcessState(ServiceIDDDOT))
}

func TestServiceProcessStateInstalledWithoutSupervisor(t *testing.T) {
	snapshot := Snapshot{
		Daemon: DaemonSnapshot{Reachable: true, Ready: true},
		Services: []ServiceSnapshot{{
			ID:                ServiceIDDDOT,
			Installed:         true,
			ProcmgrConfigured: true,
			ManagementMode:    ManagementModeNone,
		}},
	}
	assert.Equal(t, ProcessStateStopped, snapshot.ServiceProcessState(ServiceIDDDOT))
}

func TestServiceProcessStateUnknownService(t *testing.T) {
	snapshot := Snapshot{
		Services: []ServiceSnapshot{{ID: ServiceIDDDOT, ManagementMode: ManagementModeProcmgr}},
	}
	assert.Equal(t, ProcessStateUnset, snapshot.ServiceProcessState("not-a-service"))
}

func TestCollectServiceProcessStateRunning(t *testing.T) {
	root := setupDDOTInstallFixture(t)

	collector := NewCollectorWithClient(root, &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true, RunningProcesses: 1},
		processes: map[string]ProcessSnapshot{
			"datadog-agent-ddot": {Name: "datadog-agent-ddot", State: pb.ProcessState_RUNNING},
		},
	})

	snapshot := collector.Collect(context.Background())

	assert.Equal(t, ProcessStateRunning, snapshot.ServiceProcessState(ServiceIDDDOT))
}

func TestCollectServiceProcessStateDaemonUnreachable(t *testing.T) {
	root := setupDDOTInstallFixture(t)

	collector := NewCollectorWithClient(root, &mockClient{connectErr: os.ErrNotExist})

	snapshot := collector.Collect(context.Background())

	assert.Equal(t, ProcessStateUnknown, snapshot.ServiceProcessState(ServiceIDDDOT))
}

func TestCollectServiceProcessStateNotInstalled(t *testing.T) {
	collector := NewCollectorWithClient(t.TempDir(), &mockClient{
		daemon: DaemonSnapshot{Reachable: true, Ready: true},
	})

	snapshot := collector.Collect(context.Background())

	assert.Equal(t, ProcessStateNotInstalled, snapshot.ServiceProcessState(ServiceIDDDOT))
}
