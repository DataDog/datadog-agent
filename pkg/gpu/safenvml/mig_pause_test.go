// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	gputestutil "github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestPollNvmlMigModePausesOnPending(t *testing.T) {
	lib := gputestutil.GetBasicNvmlMockWithOptions(
		gputestutil.WithDeviceCount(1),
		gputestutil.WithMigMode(nvml.DEVICE_MIG_DISABLE, nvml.DEVICE_MIG_ENABLE),
	)
	WithPartialMockNVML(t, lib, allSymbols)

	s := &singleton
	s.pollNvmlMigMode()
	controller := s.getPauseController()
	require.True(t, controller.IsPaused(), "expected NVML to be paused when MIG mode is pending")
}

func TestPollMigStateUnpausesOnProcfs(t *testing.T) {
	WithPartialMockNVML(t, gputestutil.GetBasicNvmlMock(), allSymbols)

	originalRoot := procfsRoot
	procfsRoot = t.TempDir()
	t.Cleanup(func() {
		procfsRoot = originalRoot
	})

	migPath := filepath.Join(procfsRoot, "driver", "nvidia", "capabilities", "gpu0", "mig", "gi0", "ci0")
	require.NoError(t, os.MkdirAll(migPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(migPath, "access"), []byte("1"), 0o644))

	s := &singleton
	controller := s.getPauseController()
	controller.Pause("test")

	SetMigConfigStateProvider(nil)
	t.Cleanup(func() {
		SetMigConfigStateProvider(nil)
	})
	s.pollMigState()

	require.False(t, controller.IsPaused(), "expected NVML to be unpaused when MIG instances are present")
}

func TestPollMigStateLabelOverrides(t *testing.T) {
	WithPartialMockNVML(t, gputestutil.GetBasicNvmlMock(), allSymbols)

	s := &singleton
	controller := s.getPauseController()
	controller.Pause("test")

	SetMigConfigStateProvider(func(_ context.Context) (string, bool, error) {
		return "ready", true, nil
	})
	t.Cleanup(func() {
		SetMigConfigStateProvider(nil)
	})
	s.pollMigState()
	require.False(t, controller.IsPaused(), "expected label to clear pause")

	SetMigConfigStateProvider(func(_ context.Context) (string, bool, error) {
		return "pending", true, nil
	})
	s.pollMigState()
	require.True(t, controller.IsPaused(), "expected label to force pause")
}

func TestPollMigStateFallbackUnpause(t *testing.T) {
	WithPartialMockNVML(t, gputestutil.GetBasicNvmlMock(), allSymbols)

	s := &singleton
	controller := s.getPauseController()
	controller.Pause("test")
	controller.mu.Lock()
	controller.pauseSince = time.Now().Add(-migFallbackUnpause - time.Second)
	controller.mu.Unlock()

	SetMigConfigStateProvider(nil)
	t.Cleanup(func() {
		SetMigConfigStateProvider(nil)
	})
	originalRoot := procfsRoot
	procfsRoot = t.TempDir()
	t.Cleanup(func() {
		procfsRoot = originalRoot
	})

	s.pollMigState()
	require.False(t, controller.IsPaused(), "expected fallback timer to clear pause")
}
