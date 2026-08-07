// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package noisyneighbor is the system-probe side of the Noisy Neighbor check.
package noisyneighbor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	ciliumebpf "github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
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
		require.Zero(t, probe.loadMask)
		require.Zero(t, probe.GetStats().PerfFDCount)
		require.Len(t, probe.mgr.Probes, 3, "disabled PMU events must not attach another tracepoint")

		require.Eventually(t, func() bool {
			for _, r := range probe.GetAndFlush() {
				if r.EventCount > 0 || r.PreemptionCount > 0 {
					return true
				}
			}
			return false
		}, 10*time.Second, 500*time.Millisecond, "failed to get noisy neighbor stats")
	})
}

func TestNoisyNeighborExactCPUMigrationAttribution(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		if kv < minimumKernelVersion {
			t.Skipf("Kernel version %v is not supported by the Noisy Neighbor probe", kv)
		}

		cpus := allowedTestCPUs(t)
		cgroupID, workload := startCgroupWorkload(t)
		probe, err := NewProbe(testConfig(), Config{EventMask: model.EventCPUMigrations, MaxTrackedCgroups: 64})
		require.NoError(t, err)
		t.Cleanup(probe.Close)

		// Moving a task from an unwatched cgroup must not create a PMU sample.
		_, err = probe.ReplaceWatchlist(model.WatchlistRequest{CgroupIDs: []uint64{cgroupID + 1}})
		require.NoError(t, err)
		migrateWorkload(t, workload.Process.Pid, cpus, 4)
		for _, stat := range probe.GetAndFlush() {
			if stat.CgroupID == cgroupID {
				require.Zero(t, stat.SampledEventMask&model.EventCPUMigrations)
			}
		}

		_, err = probe.ReplaceWatchlist(model.WatchlistRequest{CgroupIDs: []uint64{cgroupID}})
		require.NoError(t, err)
		// Establish CPU 0 and discard that setup migration before the exact count.
		require.NoError(t, setAffinity(workload.Process.Pid, cpus[0]))
		probe.GetAndFlush()

		const migrations = 20
		migrateWorkload(t, workload.Process.Pid, cpus, migrations)
		var observed uint64
		require.Eventually(t, func() bool {
			for _, stat := range probe.GetAndFlush() {
				if stat.CgroupID == cgroupID && stat.SampledEventMask&model.EventCPUMigrations != 0 {
					observed = stat.CPUMigrations
					return true
				}
			}
			return false
		}, 5*time.Second, 50*time.Millisecond, "expected watched task migrations")
		require.EqualValues(t, migrations, observed)
	})
}

func TestReplaceWatchlistPreservesRetainedGenerations(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		if kv < minimumKernelVersion {
			t.Skipf("Kernel version %v is not supported by the Noisy Neighbor probe", kv)
		}

		probe, err := NewProbe(testConfig(), Config{EventMask: model.EventCPUMigrations, MaxTrackedCgroups: 64})
		require.NoError(t, err)
		t.Cleanup(probe.Close)

		response, err := probe.ReplaceWatchlist(model.WatchlistRequest{CgroupIDs: []uint64{100, 200}})
		require.NoError(t, err)
		watchlist, found, err := probe.mgr.GetMap("pmu_watchlist")
		require.NoError(t, err)
		require.True(t, found)

		var retainedGeneration uint64
		require.NoError(t, watchlist.Lookup(uint64(100), &retainedGeneration))
		require.Equal(t, response.Generation, retainedGeneration)

		response, err = probe.ReplaceWatchlist(model.WatchlistRequest{CgroupIDs: []uint64{100, 300}})
		require.NoError(t, err)
		var currentGeneration uint64
		require.NoError(t, watchlist.Lookup(uint64(100), &currentGeneration))
		require.Equal(t, retainedGeneration, currentGeneration)
		require.ErrorIs(t, watchlist.Lookup(uint64(200), &currentGeneration), ciliumebpf.ErrKeyNotExist)
		require.NoError(t, watchlist.Lookup(uint64(300), &currentGeneration))
		require.Equal(t, response.Generation, currentGeneration)
	})
}

func allowedTestCPUs(t *testing.T) []int {
	t.Helper()
	var affinity unix.CPUSet
	require.NoError(t, unix.SchedGetaffinity(0, &affinity))
	var cpus []int
	for cpu := 0; cpu < 1024 && len(cpus) < 2; cpu++ {
		if affinity.IsSet(cpu) {
			cpus = append(cpus, cpu)
		}
	}
	if len(cpus) < 2 {
		t.Skip("exact CPU migration test requires at least two allowed CPUs")
	}
	return cpus
}

func startCgroupWorkload(t *testing.T) (uint64, *exec.Cmd) {
	t.Helper()
	data, err := os.ReadFile("/proc/self/cgroup")
	require.NoError(t, err)
	var relativePath string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "0::") {
			relativePath = strings.TrimPrefix(line, "0::")
			break
		}
	}
	if relativePath == "" {
		t.Skip("exact CPU migration test requires cgroup v2")
	}
	cgroupPath := filepath.Join("/sys/fs/cgroup", relativePath, fmt.Sprintf("noisy-neighbor-test-%d", os.Getpid()))
	if err := os.Mkdir(cgroupPath, 0o755); err != nil {
		t.Skipf("cannot create test cgroup: %v", err)
	}

	workload := exec.Command("sh", "-c", "while :; do :; done")
	require.NoError(t, workload.Start())
	if err := os.WriteFile(filepath.Join(cgroupPath, "cgroup.procs"), []byte(strconv.Itoa(workload.Process.Pid)), 0o600); err != nil {
		_ = workload.Process.Kill()
		_ = workload.Wait()
		_ = os.Remove(cgroupPath)
		t.Skipf("cannot move workload to test cgroup: %v", err)
	}
	t.Cleanup(func() {
		_ = workload.Process.Kill()
		_ = workload.Wait()
		_ = os.Remove(cgroupPath)
	})

	var stat unix.Stat_t
	require.NoError(t, unix.Stat(cgroupPath, &stat))
	return stat.Ino, workload
}

func migrateWorkload(t *testing.T, pid int, cpus []int, migrations int) {
	t.Helper()
	for i := 0; i < migrations; i++ {
		require.NoError(t, setAffinity(pid, cpus[(i+1)%2]))
	}
}

func setAffinity(pid, cpu int) error {
	var affinity unix.CPUSet
	affinity.Set(cpu)
	return unix.SchedSetaffinity(pid, &affinity)
}

func testConfig() *ebpf.Config {
	cfg := ebpf.NewConfig()
	return cfg
}
