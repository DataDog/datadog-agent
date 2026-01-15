// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf && test

package maps

import (
	"context"
	"os/exec"
	"testing"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestValidatePIDKeyedMap(t *testing.T) {
	kv, err := kernel.HostVersion()
	require.NoError(t, err)

	// m.Info() requires kernel >= 4.10 for fdinfo support
	if kv < kernel.VersionCode(4, 10, 0) {
		t.Skipf("Test requires kernel >= 4.10 for eBPF map info support, got %s", kv)
	}

	require.NoError(t, rlimit.RemoveMemlock())

	// Create test map
	testMap, err := ebpf.NewMap(&ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    8, // uint64 for pid_tgid
		ValueSize:  1, // minimal value
		MaxEntries: 100,
	})
	require.NoError(t, err)
	defer testMap.Close()

	// Start a living process
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "sleep", "1000")
	require.NoError(t, cmd.Start())

	pid := cmd.Process.Pid
	key := uint64(pid)<<32 | uint64(pid) // pid_tgid format
	value := byte(0)

	// Insert living PID into map
	err = testMap.Put(unsafe.Pointer(&key), unsafe.Pointer(&value))
	require.NoError(t, err)

	// Test 1: Living process should NOT be detected as leak
	info, err := ValidatePIDKeyedMap("test_map", testMap)
	require.NoError(t, err)
	require.Equal(t, 1, info.TotalEntries, "should have 1 entry")
	require.Equal(t, 0, info.LeakedEntries, "living process should NOT be leak")
	require.Empty(t, info.DeadPIDs, "no dead PIDs for living process")

	// Kill the process
	cancel()
	_ = cmd.Wait()

	// Test 2: Same PID should now be detected as leak
	info, err = ValidatePIDKeyedMap("test_map", testMap)
	require.NoError(t, err)
	require.Equal(t, 1, info.TotalEntries, "should still have 1 entry")
	require.Equal(t, 1, info.LeakedEntries, "dead process should be detected as leak")
	require.Contains(t, info.DeadPIDs, uint32(pid), "dead PID should be in list")
}
