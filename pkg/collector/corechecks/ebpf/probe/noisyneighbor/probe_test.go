// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package noisyneighbor is the system-probe side of the Noisy Neighbor check.
package noisyneighbor

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var kv = kernel.MustHostVersion()

func TestNoisyNeighborProbe(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		if kv < minimumKernelVersion {
			t.Skipf("Kernel version %v is not supported by the Noisy Neighbor probe", kv)
		}

		if strings.Contains(os.Getenv("CI_JOB_NAME"), "fedora_38") {
			t.Skipf("Noisy Neighbor probe is not supported on this environment: %s", os.Getenv("CI_JOB_NAME"))
		}

		t.Logf("testing on %s", os.Getenv("CI_JOB_NAME"))

		cfg := testConfig()
		probe, err := NewProbe(cfg)
		require.NoError(t, err)
		t.Cleanup(probe.Close)

		// With empty watchlist, eBPF fast-exits and no stats are collected
		resp := probe.GetAndFlush()
		assert.Empty(t, resp.CgroupStats, "should have no stats with empty watchlist")

		// Read our own cgroup ID and add it to the watchlist
		cgroupID := readSelfCgroupID(t)
		if cgroupID == 0 {
			t.Skip("could not determine self cgroup ID (cgroupv2 may not be available)")
		}
		{
			err = probe.UpdateWatchlist([]uint64{cgroupID})
			require.NoError(t, err)

			// With our cgroup watched, we should see stats
			require.Eventually(t, func() bool {
				resp := probe.GetAndFlush()
				for _, r := range resp.CgroupStats {
					if r.EventCount > 0 || r.ForeignPreemptionCount > 0 || r.SelfPreemptionCount > 0 {
						return true
					}
				}
				return false
			}, 10*time.Second, 500*time.Millisecond, "failed to get noisy neighbor stats with watchlist active")

			// Clear watchlist
			err = probe.UpdateWatchlist(nil)
			require.NoError(t, err)
		}
	})
}

func TestUpdateWatchlist(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		if kv < minimumKernelVersion {
			t.Skipf("Kernel version %v is not supported by the Noisy Neighbor probe", kv)
		}

		if strings.Contains(os.Getenv("CI_JOB_NAME"), "fedora_38") {
			t.Skipf("Noisy Neighbor probe is not supported on this environment: %s", os.Getenv("CI_JOB_NAME"))
		}

		cfg := testConfig()
		probe, err := NewProbe(cfg)
		require.NoError(t, err)
		t.Cleanup(probe.Close)

		// Empty watchlist
		err = probe.UpdateWatchlist(nil)
		require.NoError(t, err)

		// Set watchlist
		err = probe.UpdateWatchlist([]uint64{1, 2, 3})
		require.NoError(t, err)

		// Replace watchlist
		err = probe.UpdateWatchlist([]uint64{4, 5})
		require.NoError(t, err)

		// Clear again
		err = probe.UpdateWatchlist(nil)
		require.NoError(t, err)
	})
}

func readSelfCgroupID(t *testing.T) uint64 {
	t.Helper()
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		t.Logf("cannot read /proc/self/cgroup: %v", err)
		return 0
	}
	// On cgroup v2, the line looks like "0::/some/path"
	// We need the kernfs inode of the cgroup directory
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "0::") {
			relPath := strings.TrimPrefix(line, "0::")
			relPath = strings.TrimSpace(relPath)
			cgPath := "/sys/fs/cgroup" + relPath
			var stat syscall.Stat_t
			if err := syscall.Stat(cgPath, &stat); err != nil {
				t.Logf("cannot stat cgroup path %s: %v", cgPath, err)
				return 0
			}
			t.Logf("cgroup path: %s, inode: %d", cgPath, stat.Ino)
			return stat.Ino
		}
	}
	return 0
}

func testConfig() *ebpf.Config {
	cfg := ebpf.NewConfig()
	return cfg
}
