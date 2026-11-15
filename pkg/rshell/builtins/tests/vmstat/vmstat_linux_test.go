// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package vmstat_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	vmstatcmd "github.com/DataDog/datadog-agent/pkg/rshell/builtins/vmstat"
)

// procPathMu serializes mutations of vmstatcmd.ProcPath across all tests in
// this package. Any code that writes to ProcPath must hold this lock for the
// duration of the test to prevent data races if tests are run in parallel.
// Tests in this package must NOT call t.Parallel() — doing so would cause
// test goroutines to block indefinitely on procPathMu.Lock() while another
// test holds the lock.
var procPathMu sync.Mutex

const syntheticProcStat = `cpu  201313 3153 46753 1049295 3200 0 1465 0 0 0
intr 1234567 0 0 0
ctxt 987654
btime 1600000000
processes 12345
procs_running 2
procs_blocked 1
softirq 555555 0 1 2 3
`

const syntheticMeminfo = `MemTotal:        8000000 kB
MemFree:         2000000 kB
MemAvailable:    4000000 kB
Buffers:          100000 kB
Cached:          1500000 kB
SReclaimable:     200000 kB
SwapCached:            0 kB
Active:          3000000 kB
Inactive:        1000000 kB
SwapTotal:       2000000 kB
SwapFree:        1900000 kB
`

const syntheticVmstat = `nr_free_pages 500000
pgpgin 123456
pgpgout 654321
pswpin 10
pswpout 20
pgfault 999
`

const syntheticLoadavg = `0.12 0.34 0.56 1/234 5678
`

const syntheticUptime = `12345.67 98765.43
`

// writeSyntheticProc writes a synthetic /proc tree (stat/meminfo/vmstat/
// loadavg/uptime) to a temp directory, patches vmstatcmd.ProcPath to point at
// it, and restores the original path via t.Cleanup.
//
// It acquires procPathMu for the duration of the test to prevent data races.
func writeSyntheticProc(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
	write("stat", syntheticProcStat)
	write("meminfo", syntheticMeminfo)
	write("vmstat", syntheticVmstat)
	write("loadavg", syntheticLoadavg)
	write("uptime", syntheticUptime)

	procPathMu.Lock()
	orig := vmstatcmd.ProcPath
	vmstatcmd.ProcPath = dir
	t.Cleanup(func() {
		vmstatcmd.ProcPath = orig
		procPathMu.Unlock()
	})
}

func TestVmstatSnapshotDefault(t *testing.T) {
	writeSyntheticProc(t)
	stdout, stderr, code := cmdRun(t, "vmstat")
	require.Equal(t, 0, code, "stderr: %s", stderr)
	lines := splitLines(stdout)
	require.Len(t, lines, 3, "header (2 lines) + one data row")
	assert.Contains(t, lines[0], "procs")
	assert.Contains(t, lines[0], "memory")
	assert.Contains(t, lines[0], "swap")
	assert.Contains(t, lines[0], "io")
	assert.Contains(t, lines[0], "system")
	assert.Contains(t, lines[0], "cpu")
	assert.Contains(t, lines[1], "r")
	assert.Contains(t, lines[1], "swpd")
	assert.Contains(t, lines[1], "us")
}

func TestVmstatSnapshotActiveMemory(t *testing.T) {
	writeSyntheticProc(t)
	stdout, stderr, code := cmdRun(t, "vmstat -a")
	require.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Contains(t, stdout, "inact")
	assert.Contains(t, stdout, "active")
}

func TestVmstatSnapshotWide(t *testing.T) {
	writeSyntheticProc(t)
	narrow, _, code1 := cmdRun(t, "vmstat")
	require.Equal(t, 0, code1)
	wide, stderr, code2 := cmdRun(t, "vmstat -w")
	require.Equal(t, 0, code2, "stderr: %s", stderr)
	assert.Greater(t, len(wide), len(narrow), "wide output should use wider columns")
}

func TestVmstatSnapshotUnitScale(t *testing.T) {
	writeSyntheticProc(t)
	stdoutK, _, code1 := cmdRun(t, "vmstat -S K")
	require.Equal(t, 0, code1)
	stdoutM, stderr, code2 := cmdRun(t, "vmstat -S M")
	require.Equal(t, 0, code2, "stderr: %s", stderr)
	assert.NotEqual(t, stdoutK, stdoutM, "different -S units should scale memory columns differently")
}

func TestVmstatInvalidUnit(t *testing.T) {
	writeSyntheticProc(t)
	_, stderr, code := cmdRun(t, "vmstat -S bogus")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid unit")
}

func TestVmstatStatsFlag(t *testing.T) {
	writeSyntheticProc(t)
	stdout, stderr, code := cmdRun(t, "vmstat -s")
	require.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Contains(t, stdout, "total memory")
	assert.Contains(t, stdout, "total swap")
	assert.Contains(t, stdout, "runnable processes")
	assert.Contains(t, stdout, "CPU user ticks")
	assert.Contains(t, stdout, "minute load average")
	assert.Contains(t, stdout, "6000000 K used memory", "procps vmstat defines used memory as total minus free")
	assert.Contains(t, stdout, "1700000 K swap cache", "cache includes reclaimable slab")
}

func TestVmstatStatsIgnoresOperand(t *testing.T) {
	writeSyntheticProc(t)
	stdout, stderr, code := cmdRun(t, "vmstat -s 1")
	require.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Contains(t, stdout, "total memory")
}

func TestVmstatStatsRejectsInvalidOperand(t *testing.T) {
	writeSyntheticProc(t)
	_, stderr, code := cmdRun(t, "vmstat -s notanumber")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid delay")
}

func TestVmstatSamplingWithCount(t *testing.T) {
	writeSyntheticProc(t)
	stdout, stderr, code := cmdRun(t, "vmstat 1 3")
	require.Equal(t, 0, code, "stderr: %s", stderr)
	lines := splitLines(stdout)
	require.Len(t, lines, 5, "header (2 lines) + 3 data rows")
}

func TestVmstatInvalidDelay(t *testing.T) {
	writeSyntheticProc(t)
	_, stderr, code := cmdRun(t, "vmstat abc")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid delay")
}

func TestVmstatInvalidCount(t *testing.T) {
	writeSyntheticProc(t)
	_, stderr, code := cmdRun(t, "vmstat 1 0")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid count")
}

func TestVmstatRequiresCountForSampling(t *testing.T) {
	writeSyntheticProc(t)
	_, stderr, code := cmdRun(t, "vmstat 1")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "count is required")
}

func TestVmstatRejectsExcessiveSamplingDuration(t *testing.T) {
	writeSyntheticProc(t)
	_, stderr, code := cmdRun(t, "vmstat 2 30")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "sampling duration exceeds 29s")
}

func TestVmstatExtraOperand(t *testing.T) {
	writeSyntheticProc(t)
	_, stderr, code := cmdRun(t, "vmstat 1 2 3")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "extra operand")
}

func TestVmstatHelp(t *testing.T) {
	writeSyntheticProc(t)
	stdout, stderr, code := cmdRun(t, "vmstat --help")
	require.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Usage: vmstat")
	assert.Contains(t, stdout, "--stats")
}

func TestVmstatUnknownFlagRejected(t *testing.T) {
	writeSyntheticProc(t)
	_, stderr, code := cmdRun(t, "vmstat -d")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid option -- 'd'")
}

func TestVmstatMissingProcFilesShowsDashes(t *testing.T) {
	dir := t.TempDir()
	// Only meminfo is present; stat/vmstat/loadavg/uptime are absent, so the
	// procs/swap-rate/io/system/cpu column groups must render as dashes
	// instead of fabricated zeros.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "meminfo"), []byte(syntheticMeminfo), 0o644))

	procPathMu.Lock()
	orig := vmstatcmd.ProcPath
	vmstatcmd.ProcPath = dir
	t.Cleanup(func() {
		vmstatcmd.ProcPath = orig
		procPathMu.Unlock()
	})
	stdout, stderr, code := cmdRun(t, "vmstat")
	require.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Contains(t, stdout, "-", "missing counter groups should render as dashes, not panic or fabricate zeros")
}

func TestVmstatReadFailureDoesNotPrintHeader(t *testing.T) {
	dir := t.TempDir()
	procPathMu.Lock()
	orig := vmstatcmd.ProcPath
	vmstatcmd.ProcPath = dir
	t.Cleanup(func() {
		vmstatcmd.ProcPath = orig
		procPathMu.Unlock()
	})

	stdout, stderr, code := cmdRun(t, "vmstat")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "no /proc/")
}

func splitLines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}
