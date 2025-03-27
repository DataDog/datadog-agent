// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm/consts"
	fileopener "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

func testArch(t *testing.T, arch string) {
	cfg := utils.NewUSMEmptyConfig()
	cfg.EnableNativeTLSMonitoring = true

	if !usmconfig.TLSSupported(cfg) {
		t.Skip("shared library tracing not supported for this platform")
	}

	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	// Named site-packages/ddtrace since it is used from servicediscovery tests too.
	libmmap := filepath.Join(curDir, "testdata", "site-packages", "ddtrace")
	lib := filepath.Join(libmmap, fmt.Sprintf("libssl.so.%s", arch))

	monitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)
	require.NotNil(t, monitor)

	cmd, err := fileopener.OpenFromAnotherProcess(t, lib)
	require.NoError(t, err)

	if arch == runtime.GOARCH {
		utils.WaitForProgramsToBeTraced(t, consts.USMModuleName, "shared_libraries", cmd.Process.Pid, utils.ManualTracingFallbackDisabled)
	} else {
		utils.WaitForPathToBeBlocked(t, consts.USMModuleName, "shared_libraries", lib)
	}
}

func TestArchAmd64(t *testing.T) {
	testArch(t, "amd64")
}

func TestArchArm64(t *testing.T) {
	testArch(t, "arm64")
}

func TestContainerdTmpErrEnvironment(t *testing.T) {
	hookFunction := addHooks(nil, "foo", nil)
	path := utils.FilePath{PID: uint32(os.Getpid()), HostPath: "/foo/tmpmounts/containerd-mount/bar"}
	err := hookFunction(path)
	require.ErrorIs(t, err, utils.ErrEnvironment)
}

// TestSSLMapsCleaner verifies that SSL-related kernel maps are cleared correctly.
// the map entry is deleted when the thread exits, also periodic map cleaner removes dead threads.
func TestSSLMapsCleaner(t *testing.T) {
	// setup monitor
	cfg := utils.NewUSMEmptyConfig()
	cfg.EnableNativeTLSMonitoring = true
	// test cleanup is faster without event stream, this test does not require event stream
	cfg.EnableUSMEventStream = false

	if !usmconfig.TLSSupported(cfg) {
		t.Skip("SSL maps cleaner not supported for this platform")
	}
	// use the monitor and its eBPF manager to check and access SSL related maps
	monitor := setupUSMTLSMonitor(t, cfg, reInitEventConsumer)
	require.NotNil(t, monitor)

	cleanProtocolMaps(t, "ssl", monitor.ebpfProgram.Manager.Manager)
	cleanProtocolMaps(t, "bio_new_socket_args", monitor.ebpfProgram.Manager.Manager)

	// find maps by names
	maps := getMaps(t, monitor.ebpfProgram.Manager.Manager, sslPidKeyMaps)
	require.Equal(t, len(maps), len(sslPidKeyMaps))

	// add random pid to the maps
	pid := 100
	addPidEntryToMaps(t, maps, pid)
	checkPidExistsInMaps(t, monitor.ebpfProgram.Manager.Manager, maps, pid)

	// verify that map is empty after cleaning up terminated processes
	cleanDeadPidsInSslMaps(t, monitor.ebpfProgram.Manager.Manager)
	checkPidNotFoundInMaps(t, monitor.ebpfProgram.Manager.Manager, maps, pid)

	// start dummy program and add its pid to the map
	cmd, cancel := startDummyProgram(t)
	addPidEntryToMaps(t, maps, cmd.Process.Pid)
	checkPidExistsInMaps(t, monitor.ebpfProgram.Manager.Manager, maps, cmd.Process.Pid)

	// verify exit of process cleans the map
	cancel()
	_ = cmd.Wait()
	checkPidNotFoundInMaps(t, monitor.ebpfProgram.Manager.Manager, maps, cmd.Process.Pid)
}

// getMaps returns eBPF maps searched by names.
func getMaps(t *testing.T, manager *manager.Manager, mapNames []string) []*ebpf.Map {
	maps := make([]*ebpf.Map, 0, len(mapNames))
	for _, mapName := range mapNames {
		emap, _, _ := manager.GetMap(mapName)
		require.NotNil(t, emap)
		maps = append(maps, emap)
	}
	return maps
}

// addPidEntryToMaps adds an entry to maps using the PID as a key.
func addPidEntryToMaps(t *testing.T, maps []*ebpf.Map, pid int) {
	for _, m := range maps {
		require.Equal(t, m.KeySize(), uint32(unsafe.Sizeof(uint64(0))), "wrong key size")

		// make the key for single thread process when pid and tgid are the same
		key := uint64(pid)<<32 | uint64(pid)
		value := make([]byte, m.ValueSize())

		err := m.Put(unsafe.Pointer(&key), unsafe.Pointer(&value))
		require.NoError(t, err)
	}
}

// checkPidExistsInMaps checks that pid exists in all provided maps.
func checkPidExistsInMaps(t *testing.T, manager *manager.Manager, maps []*ebpf.Map, pid int) {
	// make the key for single thread process when pid and tgid are the same
	key := uint64(pid)<<32 | uint64(pid)

	for _, m := range maps {
		require.Equal(t, m.KeySize(), uint32(unsafe.Sizeof(uint64(0))), "wrong key size")
		mapInfo, err := m.Info()
		require.NoError(t, err)

		assert.Eventually(t, func() bool {
			return findKeyInMap(m, key)
		}, 1*time.Second, 100*time.Millisecond)
		if t.Failed() {
			t.Logf("pid '%d' not found in the map %q", pid, mapInfo.Name)
			ebpftest.DumpMapsTestHelper(t, manager.DumpMaps, mapInfo.Name)
			t.FailNow()
		}
	}
}

// checkPidNotFoundInMaps checks that pid does not exist in all provided maps.
func checkPidNotFoundInMaps(t *testing.T, manager *manager.Manager, maps []*ebpf.Map, pid int) {
	// make the key for single thread process when pid and tgid are the same
	key := uint64(pid)<<32 | uint64(pid)

	for _, m := range maps {
		require.Equal(t, m.KeySize(), uint32(unsafe.Sizeof(uint64(0))), "wrong key size")
		mapInfo, err := m.Info()
		require.NoError(t, err)

		if findKeyInMap(m, key) == true {
			t.Logf("pid '%d' was found in the map %q", pid, mapInfo.Name)
			ebpftest.DumpMapsTestHelper(t, manager.DumpMaps, mapInfo.Name)
			t.FailNow()
		}
	}
}

// findKeyInMap returns true if 'theKey' was found in the map, otherwise returns false.
func findKeyInMap(m *ebpf.Map, theKey uint64) bool {
	var key uint64
	value := make([]byte, m.ValueSize())
	iter := m.Iterate()

	for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
		if key == theKey {
			return true
		}
	}
	return false
}

// startDummyProgram starts sleeping thread.
func startDummyProgram(t *testing.T) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	cmd := exec.CommandContext(ctx, "sleep", "1000")
	err := cmd.Start()
	require.NoError(t, err)

	return cmd, cancel
}

// cleanDeadPidsInSslMap delete terminated pid entries in the SSL maps.
func cleanDeadPidsInSslMaps(t *testing.T, manager *manager.Manager) {
	for _, mapName := range sslPidKeyMaps {
		err := deleteDeadPidsInMap(manager, mapName, nil)
		require.NoError(t, err)
	}
}
