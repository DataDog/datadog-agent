// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	fileopener "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

func testArch(t *testing.T, arch string) {
	cfg := config.New()
	cfg.EnableNativeTLSMonitoring = true

	if !usmconfig.TLSSupported(cfg) {
		t.Skip("shared library tracing not supported for this platform")
	}

	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	// Named site-packages/ddtrace since it is used from servicediscovery tests too.
	libmmap := filepath.Join(curDir, "testdata", "site-packages", "ddtrace")
	lib := filepath.Join(libmmap, fmt.Sprintf("libssl.so.%s", arch))

	monitor := setupUSMTLSMonitor(t, cfg)
	require.NotNil(t, monitor)

	cmd, err := fileopener.OpenFromAnotherProcess(t, lib)
	require.NoError(t, err)

	if arch == runtime.GOARCH {
		utils.WaitForProgramsToBeTraced(t, "shared_libraries", cmd.Process.Pid, utils.ManualTracingFallbackDisabled)
	} else {
		utils.WaitForPathToBeBlocked(t, "shared_libraries", lib)
	}
}

func TestArchAmd64(t *testing.T) {
	testArch(t, "amd64")
}

func TestArchArm64(t *testing.T) {
	testArch(t, "arm64")
}

func TestNativeShortLivedProcess(t *testing.T) {
	cfg := config.New()
	cfg.EnableNativeTLSMonitoring = true

	if !usmconfig.TLSSupported(cfg) {
		t.Skip("shared library tracing not supported for this platform")
	}

	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	libmmap := filepath.Join(curDir, "testdata", "site-packages", "ddtrace")
	lib := filepath.Join(libmmap, fmt.Sprintf("libssl.so.%s", runtime.GOARCH))

	var pidMap sync.Map
	utils.RegisterHook = func(pid uint32) {
		pidMap.Store(pid, struct{}{})
	}
	t.Cleanup(func() {
		// Will be called after monitor is stopped (due to t.Cleanup() ordering)
		// so should be race-free.
		utils.RegisterHook = nil
	})

	monitor := setupUSMTLSMonitor(t, cfg)
	require.NotNil(t, monitor)

	pids := make([]uint32, 10)
	for i := 0; i < 10; i++ {
		cmd := exec.Command("cat", lib)
		require.NoError(t, cmd.Run())
		pid := cmd.Process.Pid
		require.False(t, utils.IsProgramTraced("shared_libraries", pid))
		pids = append(pids, uint32(pid))
	}

	require.False(t, utils.IsBlocked(t, "shared_libraries", lib))

	cmd, err := fileopener.OpenFromAnotherProcess(t, lib)
	require.NoError(t, err)
	utils.WaitForProgramsToBeTraced(t, "shared_libraries", cmd.Process.Pid, utils.ManualTracingFallbackDisabled)

	// Check at end to avoid any races with parallel registration.  If nothing
	// has been registered, then the test will pass (nothing will be blocked),
	// but the result will not be reliable since it would not have verified what
	// it was trying to verify.
	registered := false
	for _, pid := range pids {
		if _, found := pidMap.Load(uint32(pid)); found {
			registered = true
			break
		}
	}
	if !registered {
		t.Skip("Register never called on process, test result cannot be trusted")
	}
}
