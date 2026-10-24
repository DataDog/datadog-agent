// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package procinfo

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeLinuxProcFixture(t *testing.T) string {
	t.Helper()
	procPath := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(procPath, "stat"),
		[]byte("cpu 0 0 0 0\nbtime 1000000000\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(procPath, "uptime"),
		[]byte("1000.00 0.00\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(procPath, "meminfo"),
		[]byte("MemTotal: 1048576 kB\n"),
		0o644,
	))
	return procPath
}

func writeLinuxProcEntry(
	t *testing.T,
	procPath string,
	pid, ppid, session int,
	utime, stime, rssPages string,
) {
	t.Helper()
	pidPath := filepath.Join(procPath, strconv.Itoa(pid))
	require.NoError(t, os.Mkdir(pidPath, 0o755))
	stat := fmt.Sprintf(
		"%d (worker-%d) S %d %d %d 0 -1 0 0 0 0 0 %s %s 0 0 20 0 1 0 100 8388608 %s\n",
		pid,
		pid,
		ppid,
		pid,
		session,
		utime,
		stime,
		rssPages,
	)
	require.NoError(t, os.WriteFile(filepath.Join(pidPath, "stat"), []byte(stat), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(pidPath, "status"),
		[]byte("Name:\tworker\nUid:\t1000 1000 1000 1000\n"),
		0o644,
	))
}

func TestReadProcResourceMetrics(t *testing.T) {
	procPath := t.TempDir()
	pid := 42
	pidPath := filepath.Join(procPath, "42")
	require.NoError(t, os.Mkdir(pidPath, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pidPath, "stat"),
		[]byte("42 (safe worker) S 1 42 42 0 -1 0 0 0 0 0 200 100 0 0 20 0 1 0 100 8388608 256\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(pidPath, "status"),
		[]byte("Name:\tsafe worker\nUid:\t1000 1000 1000 1000\n"),
		0o644,
	))

	requested := MetricStartTime | MetricCPUTime | MetricElapsed |
		MetricRSS | MetricVSZ | MetricPMem | MetricPCPU
	info, err := readProc(procPath, pid, 1_000_000_000, linuxMetricInputs{
		requested:   requested,
		uptime:      1000 * time.Second,
		uptimeValid: true,
		memTotalKiB: 1_048_576,
	})
	require.NoError(t, err)

	require.Equal(t, "safe worker", info.Cmd)
	require.Equal(t, 3*time.Second, info.CPUTime)
	require.Equal(t, "00:00:03", info.Time)
	require.Equal(t, 999*time.Second, info.Elapsed)
	require.Equal(t, uint64(8192), info.VSZKiB)
	require.Equal(t, uint64(256*os.Getpagesize()/1024), info.RSSKiB)
	require.InDelta(t, 100*float64(info.RSSKiB)/1_048_576, info.PMem, 0.0001)
	require.InDelta(t, 100*3.0/999.0, info.PCPU, 0.0001)
	require.True(t, info.Has(requested))
}

func TestReadProcBoundsCPUInteger(t *testing.T) {
	procPath := writeLinuxProcFixture(t)
	const pid = 43
	maxTicks := int64(math.MaxInt64) / int64(time.Second/clkTck)
	writeLinuxProcEntry(
		t,
		procPath,
		pid,
		1,
		pid,
		strconv.FormatInt(maxTicks, 10),
		"0",
		"1",
	)

	info, err := readProc(procPath, pid, 1_000_000_000, linuxMetricInputs{
		requested:   MetricPCPU,
		uptime:      time.Second + time.Nanosecond,
		uptimeValid: true,
	})
	require.NoError(t, err)

	maxInt := int(^uint(0) >> 1)
	require.True(t, info.Has(MetricPCPU))
	require.Equal(t, time.Nanosecond, info.Elapsed)
	require.Greater(t, info.PCPU, float64(maxInt))
	require.Equal(t, maxInt, info.CPU)
}

func TestGetByPIDsStandaloneDerivedMetricMasks(t *testing.T) {
	procPath := writeLinuxProcFixture(t)
	const pid = 42
	writeLinuxProcEntry(t, procPath, pid, 1, pid, "200", "100", "256")

	t.Run("pcpu loads uptime dependency", func(t *testing.T) {
		procs, err := getByPIDs(context.Background(), procPath, []int{pid}, MetricPCPU)
		require.NoError(t, err)
		require.Len(t, procs, 1)

		info := procs[0]
		require.True(t, info.Has(MetricPCPU|MetricCPUTime|MetricElapsed))
		require.InDelta(t, 100*3.0/999.0, info.PCPU, 0.0001)
		require.False(t, info.Has(MetricPMem|MetricRSS|MetricVSZ))
	})

	t.Run("pmem loads rss and total memory dependencies", func(t *testing.T) {
		procs, err := getByPIDs(context.Background(), procPath, []int{pid}, MetricPMem)
		require.NoError(t, err)
		require.Len(t, procs, 1)

		info := procs[0]
		require.True(t, info.Has(MetricPMem|MetricRSS))
		require.InDelta(t, 100*float64(info.RSSKiB)/1_048_576, info.PMem, 0.0001)
		require.False(t, info.Has(MetricPCPU|MetricElapsed|MetricVSZ))
	})

	t.Run("elapsed loads uptime dependency", func(t *testing.T) {
		procs, err := getByPIDs(context.Background(), procPath, []int{pid}, MetricElapsed)
		require.NoError(t, err)
		require.Len(t, procs, 1)

		info := procs[0]
		require.True(t, info.Has(MetricElapsed))
		require.Equal(t, 999*time.Second, info.Elapsed)
		require.False(t, info.Has(MetricPCPU|MetricPMem|MetricRSS|MetricVSZ))
	})
}

func TestReadProcLeavesUnavailableDerivedMetricsUnset(t *testing.T) {
	procPath := t.TempDir()
	pidPath := filepath.Join(procPath, "7")
	require.NoError(t, os.Mkdir(pidPath, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pidPath, "stat"),
		[]byte("7 (short) S 1 7 7 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 100\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(filepath.Join(pidPath, "status"), []byte("Uid:\t1000\n"), 0o644))

	info, err := readProc(procPath, 7, 0, linuxMetricInputs{
		requested: MetricElapsed | MetricRSS | MetricVSZ | MetricPMem | MetricPCPU,
	})
	require.NoError(t, err)
	require.False(t, info.Has(MetricElapsed))
	require.False(t, info.Has(MetricRSS))
	require.False(t, info.Has(MetricVSZ))
	require.False(t, info.Has(MetricPMem))
	require.False(t, info.Has(MetricPCPU))
}

func TestGetSessionStopsAtCyclicParentChain(t *testing.T) {
	procPath := writeLinuxProcFixture(t)
	selfPID := os.Getpid()
	otherPID := selfPID + 1
	writeLinuxProcEntry(t, procPath, selfPID, otherPID, selfPID, "0", "0", "1")
	writeLinuxProcEntry(t, procPath, otherPID, selfPID, selfPID, "0", "0", "1")

	procs, err := getSession(context.Background(), procPath, 0)
	require.NoError(t, err)
	require.Len(t, procs, 2)
	require.ElementsMatch(t, []int{selfPID, otherPID}, []int{procs[0].PID, procs[1].PID})
}

func TestProcAggregateMetricInputs(t *testing.T) {
	procPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(procPath, "uptime"), []byte("123.45 0.00\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(procPath, "meminfo"), []byte(
		fmt.Sprintf("MemFree: 1 kB\nMemTotal: %d kB\n", 16_777_216),
	), 0o644))

	uptime, err := procUptime(procPath)
	require.NoError(t, err)
	require.Equal(t, 123*time.Second+450*time.Millisecond, uptime)

	total, err := procMemTotalKiB(procPath)
	require.NoError(t, err)
	require.Equal(t, uint64(16_777_216), total)
}

func TestReadBoundedProcFileAcceptsExactLimitAndRejectsLargerInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proc-data")
	require.NoError(t, os.WriteFile(path, []byte("1234"), 0o644))

	data, err := readBoundedProcFile(path, 4)
	require.NoError(t, err)
	require.Equal(t, []byte("1234"), data)

	require.NoError(t, os.WriteFile(path, []byte("12345"), 0o644))
	data, err = readBoundedProcFile(path, 4)
	require.EqualError(t, err, "data exceeds 4 bytes")
	require.Nil(t, data)
}

func TestReadProcRejectsOverflowingResourceCounters(t *testing.T) {
	t.Run("cpu tick sum", func(t *testing.T) {
		procPath := writeLinuxProcFixture(t)
		const pid = 51
		writeLinuxProcEntry(
			t,
			procPath,
			pid,
			1,
			pid,
			strconv.FormatInt(math.MaxInt64, 10),
			"1",
			"1",
		)

		info, err := readProc(procPath, pid, 1_000_000_000, linuxMetricInputs{})
		require.NoError(t, err)
		require.False(t, info.Has(MetricCPUTime))
		require.Equal(t, "-", info.Time)
	})

	t.Run("resident page conversion", func(t *testing.T) {
		procPath := writeLinuxProcFixture(t)
		const pid = 52
		writeLinuxProcEntry(
			t,
			procPath,
			pid,
			1,
			pid,
			"0",
			"0",
			strconv.FormatUint(math.MaxUint64, 10),
		)

		info, err := readProc(procPath, pid, 1_000_000_000, linuxMetricInputs{
			requested: MetricRSS,
		})
		require.NoError(t, err)
		require.False(t, info.Has(MetricRSS))
		require.Zero(t, info.RSSKiB)
	})
}

func TestTicksToDurationRejectsNegativeAndOverflow(t *testing.T) {
	_, ok := ticksToDuration(-1)
	require.False(t, ok)

	_, ok = ticksToDuration(math.MaxInt64)
	require.False(t, ok)

	duration, ok := ticksToDuration(123)
	require.True(t, ok)
	require.Equal(t, time.Second+230*time.Millisecond, duration)
}

func TestProcStartTimePreservesFractionalTicksAndRejectsOverflow(t *testing.T) {
	start, ok := procStartTime(1_000_000_000, 123, nil)
	require.True(t, ok)
	require.Equal(t, time.Unix(1_000_000_001, 230_000_000), start)

	_, ok = procStartTime(math.MaxInt64, 100, nil)
	require.False(t, ok)
}

func TestReadProcSanitizesUnsafeCommandCharacters(t *testing.T) {
	procPath := writeLinuxProcFixture(t)
	const pid = 53
	writeLinuxProcEntry(t, procPath, pid, 1, pid, "0", "0", "1")

	statPath := filepath.Join(procPath, strconv.Itoa(pid), "stat")
	data, err := os.ReadFile(statPath)
	require.NoError(t, err)
	data = []byte(strings.Replace(
		string(data),
		"(worker-53)",
		"(safe)name\nline\t\x1b[31m\x7f)",
		1,
	))
	require.NoError(t, os.WriteFile(statPath, data, 0o644))

	info, err := readProc(procPath, pid, 1_000_000_000, linuxMetricInputs{})
	require.NoError(t, err)
	require.Equal(t, "safe)name?line??[31m?", info.Cmd)
}
