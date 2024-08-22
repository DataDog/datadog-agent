// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

const (
	defaultEnvoyName = "/bin/envoy"
)

func TestIsIstioBinary(t *testing.T) {
	procRoot := uprobes.CreateFakeProcFS(t, []uprobes.FakeProcFSEntry{})
	m := newIstioTestMonitor(t, procRoot)

	t.Run("an actual envoy process", func(t *testing.T) {
		assert.True(t, m.isIstioBinary(defaultEnvoyName, uprobes.NewProcInfo(procRoot, 1)))
	})
	t.Run("something else", func(t *testing.T) {
		assert.False(t, m.isIstioBinary("", uprobes.NewProcInfo(procRoot, 2)))
	})
}

func TestGetEnvoyPathWithConfig(t *testing.T) {
	cfg := config.New()
	cfg.EnableIstioMonitoring = true
	cfg.EnvoyPath = "/test/envoy"
	monitor := newIstioTestMonitorWithCFG(t, cfg)

	assert.True(t, monitor.isIstioBinary(cfg.EnvoyPath, uprobes.NewProcInfo("", 0)))
	assert.False(t, monitor.isIstioBinary("something/else/", uprobes.NewProcInfo("", 0)))
}

func TestIstioSync(t *testing.T) {
	t.Run("calling sync for the first time", func(tt *testing.T) {
		procRoot := uprobes.CreateFakeProcFS(tt, []uprobes.FakeProcFSEntry{
			{Pid: 1, Exe: defaultEnvoyName},
			{Pid: 2, Exe: "/bin/bash"},
			{Pid: 3, Exe: defaultEnvoyName},
		})
		monitor := newIstioTestMonitor(tt, procRoot)

		mockRegistry := &uprobes.MockFileRegistry{}
		monitor.attacher.SetRegistry(mockRegistry)
		mockRegistry.On("GetRegisteredProcesses").Return(map[uint32]struct{}{})
		mockRegistry.On("Register", defaultEnvoyName, uint32(1), mock.Anything, mock.Anything).Return(nil)
		mockRegistry.On("Register", defaultEnvoyName, uint32(3), mock.Anything, mock.Anything).Return(nil)

		// Calling sync should detect the two envoy processes
		monitor.attacher.Sync(true, true)

		mockRegistry.AssertExpectations(tt)
	})

	t.Run("detecting a dangling process", func(tt *testing.T) {
		procRoot := uprobes.CreateFakeProcFS(tt, []uprobes.FakeProcFSEntry{
			{Pid: 1, Exe: defaultEnvoyName},
			{Pid: 2, Exe: "/bin/bash"},
			{Pid: 3, Exe: defaultEnvoyName},
		})
		monitor := newIstioTestMonitor(tt, procRoot)

		mockRegistry := &uprobes.MockFileRegistry{}
		monitor.attacher.SetRegistry(mockRegistry)
		mockRegistry.On("GetRegisteredProcesses").Return(map[uint32]struct{}{})
		mockRegistry.On("Register", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil) // Tell the mock to just say ok to everything, we'll validate later

		monitor.attacher.Sync(true, true)

		mockRegistry.AssertCalled(tt, "Register", defaultEnvoyName, uint32(1), mock.Anything, mock.Anything)
		mockRegistry.AssertCalled(tt, "Register", defaultEnvoyName, uint32(3), mock.Anything, mock.Anything)
		mockRegistry.AssertCalled(tt, "GetRegisteredProcesses")

		// At this point we should have received:
		// * 2 register calls
		// * 1 GetRegisteredProcesses call
		// * 0 unregister calls
		require.Equal(tt, 3, len(mockRegistry.Calls), "calls made: %v", mockRegistry.Calls)
		mockRegistry.AssertNotCalled(t, "Unregister", mock.Anything)

		// Now we emulate a process termination for PID 3 by removing it from the fake
		// procFS tree
		require.NoError(tt, os.RemoveAll(filepath.Join(procRoot, "3")))

		// Now clear the mock registry expected calls and make it return the state as if the two PIDs were registered
		mockRegistry.ExpectedCalls = nil
		mockRegistry.On("Register", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil) // Tell the mock to just say ok to everything, we'll validate later
		mockRegistry.On("GetRegisteredProcesses").Return(map[uint32]struct{}{1: {}, 3: {}})
		mockRegistry.On("Unregister", mock.Anything).Return(nil)

		// Once we call sync() again, PID 3 termination should be detected
		// and the unregister callback should be executed
		monitor.attacher.Sync(true, true)
		mockRegistry.AssertCalled(tt, "Unregister", uint32(3))
	})
}

func newIstioTestMonitor(t *testing.T, procRoot string) *istioMonitor {
	cfg := config.New()
	cfg.EnableIstioMonitoring = true
	cfg.ProcRoot = procRoot

	return newIstioTestMonitorWithCFG(t, cfg)
}

func newIstioTestMonitorWithCFG(t *testing.T, cfg *config.Config) *istioMonitor {
	monitor := newIstioMonitor(cfg, nil)
	require.NotNil(t, monitor)

	return monitor
}
