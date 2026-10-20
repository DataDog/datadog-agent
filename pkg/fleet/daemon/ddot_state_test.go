// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package daemon

import (
	"context"
	"crypto/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/box"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/procmgr/coat"
)

type fakeProcmgrCollector struct {
	snapshot coat.Snapshot
}

func (f *fakeProcmgrCollector) Collect(context.Context) coat.Snapshot {
	return f.snapshot
}

func ddotSnapshot(mode coat.ManagementMode, state string) coat.Snapshot {
	return coat.Snapshot{
		Daemon: coat.DaemonSnapshot{Reachable: true, Ready: true},
		Services: []coat.ServiceSnapshot{{
			ID:                coat.ServiceIDDDOT,
			Installed:         mode != coat.ManagementModeNone,
			ProcmgrConfigured: mode == coat.ManagementModeProcmgr,
			ProcmgrState:      state,
			ManagementMode:    mode,
		}},
	}
}

func TestDDOTProcessStateWithoutCollector(t *testing.T) {
	d := &daemonImpl{}
	assert.Equal(t, coat.ProcessStateUnknown, d.ddotProcessState(context.Background()))
}

func TestDDOTProcessStateFromCollector(t *testing.T) {
	tests := []struct {
		name     string
		snapshot coat.Snapshot
		expected string
	}{
		{
			name:     "procmgr running",
			snapshot: ddotSnapshot(coat.ManagementModeProcmgr, coat.ProcessStateRunning),
			expected: coat.ProcessStateRunning,
		},
		{
			name:     "procmgr failed",
			snapshot: ddotSnapshot(coat.ManagementModeProcmgr, coat.ProcessStateFailed),
			expected: coat.ProcessStateFailed,
		},
		{
			name:     "systemd managed",
			snapshot: ddotSnapshot(coat.ManagementModeSystemd, coat.ProcessStateUnknown),
			expected: coat.ProcessStateRunning,
		},
		{
			name:     "not installed",
			snapshot: ddotSnapshot(coat.ManagementModeNone, coat.ProcessStateUnknown),
			expected: coat.ProcessStateNotInstalled,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := &daemonImpl{procmgrCollector: &fakeProcmgrCollector{snapshot: test.snapshot}}
			assert.Equal(t, test.expected, d.ddotProcessState(context.Background()))
		})
	}
}

func TestRefreshStateReportsDDOTProcessState(t *testing.T) {
	pm := &testPackageManager{}
	pm.On("AvailableDiskSpace").Return(uint64(1000000000), nil)
	pm.On("ConfigAndPackageStates", mock.Anything).Return(&repository.PackageStates{
		States: map[string]repository.State{
			"datadog-agent":      {Stable: "7.50.0"},
			"datadog-apm-inject": {Stable: "0.20.0"},
		},
		ConfigStates: map[string]repository.State{},
	}, nil)
	rcc := newTestRemoteConfigClient(t)
	taskDB, err := newTaskDB(filepath.Join(t.TempDir(), "tasks.db"))
	require.NoError(t, err)
	defer taskDB.Close()
	secretsPubKey, secretsPrivKey, err := box.GenerateKey(rand.Reader)
	require.NoError(t, err)

	daemon := newDaemon(
		&remoteConfig{client: rcc},
		func(_ *env.Env) installer.Installer { return pm },
		&env.Env{RemoteUpdates: true},
		taskDB,
		30*time.Second,
		1*time.Hour,
		secretsPubKey,
		secretsPrivKey,
	)
	daemon.procmgrCollector = &fakeProcmgrCollector{
		snapshot: ddotSnapshot(coat.ManagementModeProcmgr, coat.ProcessStateRunning),
	}
	daemon.refreshState(context.Background())

	state := rcc.GetInstallerState()
	require.NotNil(t, state)
	require.Len(t, state.Packages, 2)
	for _, pkg := range state.Packages {
		if pkg.Package == "datadog-agent" {
			assert.Equal(t, coat.ProcessStateRunning, pkg.ProcessStates[coat.ServiceIDDDOT])
			continue
		}
		assert.Empty(t, pkg.ProcessStates, "DDOT state must only be reported on the agent package")
	}

	pm.AssertExpectations(t)
}
