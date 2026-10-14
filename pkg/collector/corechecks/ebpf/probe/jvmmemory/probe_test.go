// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package jvmmemory

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/jvmmemory/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
	dockerutils "github.com/DataDog/datadog-agent/pkg/util/testutil/docker"
)

// jvmTestImage is a glibc JDK image: its libjvm.so has the HotSpot USDT
// probes, and a JDK can run a single source file. It is pre-pulled on the
// KMT VMs: keep it in sync with _test_docker_image_list in
// tasks/system_probe.py.
const jvmTestImage = "eclipse-temurin:21.0.12.1_1-jdk-noble"

func TestJVMMemoryProbe(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		skipIfUnsupported(t)

		start := time.Now()
		containerName := "jvm-memory-test-" + strconv.FormatInt(start.UnixNano(), 36)
		pid, containerID := runJVM(t, containerName)

		rootPidNs := uint64(0)
		initPidNs := uint64(procPidInitIno)
		for _, tc := range []struct {
			name            string
			pidNamespaceIno *uint64
		}{
			// bpf_get_current_pid_tgid
			{name: "root PID namespace", pidNamespaceIno: &rootPidNs},
			// walk of the PID namespaces of the task, which must find the
			// root one at level 0
			{name: "PID namespace walk", pidNamespaceIno: &initPidNs},
		} {
			t.Run(tc.name, func(t *testing.T) {
				probe := newTestProbe(t, tc.pidNamespaceIno)
				waitForJVMStats(t, probe, pid, containerID, start)
			})
		}

		// the exited JVM must be forgotten, and its eBPF map entries deleted
		probe := newTestProbe(t, &rootPidNs)
		waitForJVMStats(t, probe, pid, containerID, start)
		require.NoError(t, exec.Command("docker", "rm", "-f", containerName).Run())
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			_, found := findJVM(probe.GetStats(), pid)
			assert.False(c, found, "the exited JVM is still reported")

			var gc ebpfGCStats
			assert.ErrorIs(c, probe.gcStats.Lookup(&pid, &gc), ebpf.ErrKeyNotExist, "the GC entry of the exited JVM is still there")
			var key ebpfPoolKey
			var value ebpfPoolValue
			it := probe.pools.Iterate()
			for it.Next(&key, &value) {
				assert.NotEqual(c, pid, key.Tgid, "the %s pool entry of the exited JVM is still there", unix.ByteSliceToString(key.Name[:]))
			}
		}, 30*time.Second, 500*time.Millisecond)
	})
}

func skipIfUnsupported(t *testing.T) {
	t.Helper()
	if err := checkKernelVersion(); err != nil {
		t.Skipf("kernel not supported by the JVM memory probe: %s", err)
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not available")
	}
}

func newTestProbe(t *testing.T, pidNamespaceIno *uint64) *Probe {
	t.Helper()
	probe, err := newProbe(ddebpf.NewConfig(), probeOptions{
		procRoot:        kernel.ProcFSRoot(),
		scanInterval:    time.Second,
		pidNamespaceIno: pidNamespaceIno,
	})
	require.NoError(t, err)
	t.Cleanup(probe.Close)
	return probe
}

// runJVM starts GcLoop.java in a container, and returns the host PID of the
// JVM and the id of the container.
func runJVM(t *testing.T, containerName string) (uint32, string) {
	t.Helper()

	cwd, err := os.Getwd()
	require.NoError(t, err)
	testdata := filepath.Join(cwd, "testdata")

	scanner, err := testutil.NewScanner(regexp.MustCompile("GcLoop started"), testutil.NoPattern)
	require.NoError(t, err)
	cfg := dockerutils.NewRunConfig(
		dockerutils.NewBaseConfig(containerName, scanner, dockerutils.WithTimeout(2*time.Minute)),
		jvmTestImage,
		"java",
		// pin the collector: the JVM chooses one depending on the resources
		dockerutils.WithBinaryArgs([]string{"-XX:+UseSerialGC", "-Xmx64m", "/testdata/GcLoop.java"}),
		dockerutils.WithMounts(map[string]string{testdata: "/testdata"}),
	)
	require.NoError(t, dockerutils.Run(t, cfg))

	pid, err := dockerutils.GetMainPID(containerName)
	require.NoError(t, err)
	containerID, err := dockerutils.GetContainerID(containerName)
	require.NoError(t, err)
	return uint32(pid), containerID
}

func findJVM(stats []model.JVMStats, pid uint32) (model.JVMStats, bool) {
	for _, s := range stats {
		if s.PID == pid {
			return s, true
		}
	}
	return model.JVMStats{}, false
}

func findPool(pools []model.PoolStats, pattern string) (model.PoolStats, error) {
	re := regexp.MustCompile(pattern)
	for _, pool := range pools {
		if re.MatchString(pool.Name) {
			return pool, nil
		}
	}
	return model.PoolStats{}, errors.New("no pool matches " + pattern)
}

// waitForJVMStats waits until the probe reports the memory pools and the GC
// pauses of the JVM of pid.
func waitForJVMStats(t *testing.T, probe *Probe, pid uint32, containerID string, start time.Time) {
	t.Helper()

	var last model.JVMStats
	ok := assert.EventuallyWithT(t, func(c *assert.CollectT) {
		stats, found := findJVM(probe.GetStats(), pid)
		require.True(c, found, "JVM %d not reported", pid)
		last = stats

		assert.Equal(c, containerID, stats.ContainerID)

		// GcLoop allocates continuously (young collections) and calls
		// System.gc() every second (full collections)
		assert.NotZero(c, stats.GC.Pauses)
		assert.NotZero(c, stats.GC.PauseTimeNs)
		assert.NotZero(c, stats.GC.FullPauses, "the full flag of gc__begin was never read")
		assert.Less(c, stats.GC.FullPauses, stats.GC.Pauses, "every pause is flagged as full")
		assert.Greater(c, stats.GC.LastPauseEnd, start.UnixNano())

		// pool names of the serial collector
		eden, err := findPool(stats.Pools, `(?i)eden`)
		if assert.NoError(c, err) {
			assert.NotZero(c, eden.Committed)
			assert.NotEqual(c, model.UndefinedSize, eden.Max)
			assert.Greater(c, eden.LastUpdate, start.UnixNano())
			assert.LessOrEqual(c, eden.LastUpdate, time.Now().UnixNano())
		}
		old, err := findPool(stats.Pools, `(?i)(old|tenured)`)
		if assert.NoError(c, err) {
			assert.NotZero(c, old.Used)
			assert.LessOrEqual(c, old.Used, old.Committed)
		}
		metaspace, err := findPool(stats.Pools, `^Metaspace$`)
		if assert.NoError(c, err) {
			assert.NotZero(c, metaspace.Used)
			// no -XX:MaxMetaspaceSize: HotSpot's SIZE_MAX, forwarded as is
			assert.Equal(c, model.UndefinedSize, metaspace.Max)
		}
	}, 2*time.Minute, time.Second)
	if !ok {
		t.Fatalf("last stats of JVM %d: %+v", pid, last)
	}
	t.Logf("JVM %d (%s): %+v", pid, runtime.GOARCH, last)
}
