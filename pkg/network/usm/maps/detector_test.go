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

func TestCheckPIDKeyedMaps(t *testing.T) {
	kv, err := kernel.HostVersion()
	require.NoError(t, err)

	// m.Info() requires kernel >= 4.10 for fdinfo support
	if kv < kernel.VersionCode(4, 10, 0) {
		t.Skipf("Test requires kernel >= 4.10 for eBPF map info support, got %s", kv)
	}

	require.NoError(t, rlimit.RemoveMemlock())

	// Create test maps that simulate the TLS maps
	createTestMap := func(name string) *ebpf.Map {
		m, err := ebpf.NewMap(&ebpf.MapSpec{
			Name:       name,
			Type:       ebpf.Hash,
			KeySize:    8, // uint64 for pid_tgid
			ValueSize:  1, // minimal value
			MaxEntries: 100,
		})
		require.NoError(t, err)
		return m
	}

	testReadMap := createTestMap("test_read_map")
	defer testReadMap.Close()

	testWriteMap := createTestMap("test_write_map")
	defer testWriteMap.Close()

	// Start two living processes
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd1 := exec.CommandContext(ctx, "sleep", "1000")
	require.NoError(t, cmd1.Start())

	cmd2 := exec.CommandContext(ctx, "sleep", "1000")
	require.NoError(t, cmd2.Start())

	pid1 := uint32(cmd1.Process.Pid)
	pid2 := uint32(cmd2.Process.Pid)

	key1 := uint64(pid1)<<32 | uint64(pid1) // pid_tgid format
	key2 := uint64(pid2)<<32 | uint64(pid2)
	value := byte(0)

	// Insert both PIDs into test_read_map
	require.NoError(t, testReadMap.Put(unsafe.Pointer(&key1), unsafe.Pointer(&value)))
	require.NoError(t, testReadMap.Put(unsafe.Pointer(&key2), unsafe.Pointer(&value)))

	// Insert both PIDs into test_write_map
	require.NoError(t, testWriteMap.Put(unsafe.Pointer(&key1), unsafe.Pointer(&value)))
	require.NoError(t, testWriteMap.Put(unsafe.Pointer(&key2), unsafe.Pointer(&value)))

	// Test 1: Both processes alive - no leaks should be detected
	info1, err := ValidatePIDKeyedMap("test_read_map", testReadMap)
	require.NoError(t, err)
	require.Equal(t, 2, info1.TotalEntries, "should have 2 entries")
	require.Equal(t, 0, info1.LeakedEntries, "living processes should NOT be leaks")
	require.Empty(t, info1.DeadPIDs, "no dead PIDs for living processes")

	info2, err := ValidatePIDKeyedMap("test_write_map", testWriteMap)
	require.NoError(t, err)
	require.Equal(t, 2, info2.TotalEntries, "should have 2 entries")
	require.Equal(t, 0, info2.LeakedEntries, "living processes should NOT be leaks")

	// Kill first process
	require.NoError(t, cmd1.Process.Kill())
	_ = cmd1.Wait()

	// Test 2: One process dead, one alive - should detect 1 leak
	info3, err := ValidatePIDKeyedMap("test_read_map", testReadMap)
	require.NoError(t, err)
	require.Equal(t, 2, info3.TotalEntries, "should still have 2 entries")
	require.Equal(t, 1, info3.LeakedEntries, "one dead process should be detected as leak")
	require.Contains(t, info3.DeadPIDs, pid1, "dead PID should be in list")
	require.NotContains(t, info3.DeadPIDs, pid2, "alive PID should NOT be in list")

	// Kill second process
	cancel()
	_ = cmd2.Wait()

	// Test 3: Both processes dead - should detect 2 leaks
	info4, err := ValidatePIDKeyedMap("test_read_map", testReadMap)
	require.NoError(t, err)
	require.Equal(t, 2, info4.TotalEntries, "should still have 2 entries")
	require.Equal(t, 2, info4.LeakedEntries, "both dead processes should be detected as leaks")
	require.Contains(t, info4.DeadPIDs, pid1, "first dead PID should be in list")
	require.Contains(t, info4.DeadPIDs, pid2, "second dead PID should be in list")

	// Verify leak rate calculation
	require.Equal(t, 1.0, info4.LeakRate, "leak rate should be 100%")
}
