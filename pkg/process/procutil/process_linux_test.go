// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package procutil

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/gopsutil/host"
	"github.com/DataDog/gopsutil/process"
)

var (
	// change this to false to run all tests against local procfs
	skipLocalTest = true
)

func getProbe(options ...Option) *probe {
	return NewProcessProbe(options...).(*probe)
}

func getProbeWithPermission(options ...Option) *probe {
	options = append(options, WithPermission(true))
	return getProbe(options...)
}

func TestGetActivePIDs(t *testing.T) {
	t.Setenv("HOST_PROC", "resources/test_procfs/proc")
	probe := getProbeWithPermission(WithProcFSRoot("resources/test_procfs/proc"))
	defer probe.Close()

	actual, err := probe.getActivePIDs()
	assert.NoError(t, err)

	t.Setenv("HOST_PROC", "resources/test_procfs/proc") // Used by gopsutil
	expect, err := process.Pids()
	assert.NoError(t, err)

	assert.ElementsMatch(t, expect, actual)
}

func TestTrimAndSplitBytes(t *testing.T) {
	for _, tc := range []struct {
		input  []byte
		expect []string
	}{
		{
			input:  []byte{115, 115, 104, 100, 58, 32, 115, 117, 110, 110, 121, 46, 107, 108, 97, 105, 114, 64, 112, 116, 115, 47, 48},
			expect: []string{string([]byte{115, 115, 104, 100, 58, 32, 115, 117, 110, 110, 121, 46, 107, 108, 97, 105, 114, 64, 112, 116, 115, 47, 48})},
		},
		{
			input:  []byte{40, 115, 100, 45, 112, 97, 109, 41, 0, 0, 0},
			expect: []string{string([]byte{40, 115, 100, 45, 112, 97, 109, 41})},
		},
		{
			input:  []byte{115, 115, 104, 100, 58, 32, 118, 97, 103, 114, 97, 110, 116, 64, 112, 116, 115, 47, 48, 0, 0},
			expect: []string{string([]byte{115, 115, 104, 100, 58, 32, 118, 97, 103, 114, 97, 110, 116, 64, 112, 116, 115, 47, 48})},
		},
		{
			input: []byte{47, 117, 115, 114, 47, 98, 105, 110, 47, 100, 111, 99, 107, 101, 114, 100, 0, 45, 72, 0, 102, 100, 58, 47, 47, 0},
			expect: []string{
				string([]byte{47, 117, 115, 114, 47, 98, 105, 110, 47, 100, 111, 99, 107, 101, 114, 100}),
				string([]byte{45, 72}),
				string([]byte{102, 100, 58, 47, 47}),
			},
		},
		{
			input: []byte{47, 117, 115, 114, 47, 98, 105, 110, 47, 100, 111, 99, 107, 101, 114, 100, 0, 45, 72, 0, 102, 100, 58, 47, 47},
			expect: []string{
				string([]byte{47, 117, 115, 114, 47, 98, 105, 110, 47, 100, 111, 99, 107, 101, 114, 100}),
				string([]byte{45, 72}),
				string([]byte{102, 100, 58, 47, 47}),
			},
		},
		{
			input: []byte{47, 117, 115, 114, 47, 98, 105, 110, 47, 100, 111, 99, 107, 101, 114, 100, 0, 0, 0, 45, 72, 0, 102, 100, 58, 47, 47},
			expect: []string{
				string([]byte{47, 117, 115, 114, 47, 98, 105, 110, 47, 100, 111, 99, 107, 101, 114, 100}),
				string([]byte{45, 72}),
				string([]byte{102, 100, 58, 47, 47}),
			},
		},
		{
			input:  []byte{0, 0, 47, 115, 98, 105, 110, 47, 105, 110, 105, 116, 0},
			expect: []string{string([]byte{47, 115, 98, 105, 110, 47, 105, 110, 105, 116})},
		},
	} {
		actual := trimAndSplitBytes(tc.input)
		assert.ElementsMatch(t, tc.expect, actual)
	}
}

func TestGetCmdlineTestFS(t *testing.T) {
	t.Setenv("HOST_PROC", "resources/test_procfs/proc") // For gopsutil
	testGetCmdline(t, WithProcFSRoot("resources/test_procfs/proc"))
}

func TestGetCmdlineLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	testGetCmdline(t)
}

func testGetCmdline(t *testing.T, probeOptions ...Option) {
	probe := getProbeWithPermission(probeOptions...)
	defer probe.Close()

	pids, err := probe.getActivePIDs()
	assert.NoError(t, err)

	for _, pid := range pids {
		actual := strings.Join(probe.getCmdline(filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid)))), " ")
		expProc, err := process.NewProcess(pid)
		assert.NoError(t, err)

		expect, err := expProc.Cmdline()
		assert.NoError(t, err)

		assert.Equal(t, expect, actual)
	}
}

func TestGetCommandName(t *testing.T) {
	probe := getProbeWithPermission(WithProcFSRoot("resources/test_procfs/proc/"))
	defer probe.Close()

	// Hardcode pid that has `comm` file set
	pid := 3254
	actual := probe.getCommandName(filepath.Join(probe.procRootLoc, strconv.Itoa(pid)))
	assert.Equal(t, "ruby", actual)
}

func TestProcessesByPIDTestFS(t *testing.T) {
	t.Setenv("HOST_PROC", "resources/test_procfs/proc/")
	testProcessesByPID(t, WithProcFSRoot("resources/test_procfs/proc/"))
}

func TestProcessesByPIDLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	testProcessesByPID(t)
}

func testProcessesByPID(t *testing.T, probeOptions ...Option) {
	// disable log output from gopsutil, the testFS doesn't have `cwd`, `fd` and `exe` dir setup,
	// gopsutil print verbose debug log regarding this
	seelog.UseLogger(seelog.Disabled)

	probe := getProbeWithPermission(probeOptions...)
	defer probe.Close()

	expectedProcs, err := process.AllProcesses()
	assert.NoError(t, err)

	procByPID, err := probe.ProcessesByPID(time.Now(), true)
	assert.NoError(t, err)

	// make sure the process that has no command line doesn't get included in the output
	for pid, expectProc := range expectedProcs {
		pathForPID := filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid)))
		cmd := strings.Join(probe.getCmdline(pathForPID), " ")
		statInfo := probe.parseStat(pathForPID, pid, time.Now())
		if cmd == "" && isKernelThread(statInfo.flags) {
			assert.NotContains(t, procByPID, pid)
		} else {
			assert.Contains(t, procByPID, pid)
			compareProcess(t, ConvertFromFilledProcess(expectProc), procByPID[pid])
		}
	}

	// Test processesByPID with collectStats == false
	procByPID, err = probe.ProcessesByPID(time.Now(), false)
	assert.NoError(t, err)
	for _, proc := range procByPID {
		// Make sure that the createTime is there
		assert.NotEmpty(t, proc.Stats.CreateTime)

		assert.NotEmpty(t, proc.Pid)
		assert.NotEmpty(t, proc.Name)

		// Make sure that the memory stats are not collected
		assert.Empty(t, proc.Stats.MemInfoEx)
	}
}

func compareProcess(t *testing.T, procV1, procV2 *Process) {
	assert.Equal(t, procV1.Pid, procV2.Pid)
	assert.Equal(t, procV1.Ppid, procV2.Ppid)
	assert.Equal(t, procV1.NsPid, procV2.NsPid)
	oldCmd := strings.Trim(strings.Join(procV1.Cmdline, " "), " ")
	newCmd := strings.Join(procV2.Cmdline, " ")
	assert.Equal(t, oldCmd, newCmd)
	assert.Equal(t, procV1.Username, procV2.Username)
	assert.Equal(t, procV1.Cwd, procV2.Cwd)
	assert.Equal(t, procV1.Exe, procV2.Exe)
	assert.Equal(t, procV1.Name, procV2.Name, "expected:%+v actual:%+v", procV1, procV2)
	assert.ElementsMatch(t, procV1.Uids, procV2.Uids)
	assert.ElementsMatch(t, procV1.Gids, procV2.Gids)
	compareStats(t, procV1.Stats, procV2.Stats)
}

func compareStats(t *testing.T, st1, st2 *Stats) {
	// CPU Timestamp might be different between gopsutil and procutil fetches data,
	// so we compare with tolerance of 1s, then compare CpuTime without `Timestamp` field
	assert.InDelta(t, st1.CPUTime.Timestamp, st2.CPUTime.Timestamp, 1.0)
	st1.CPUTime.Timestamp = 0
	st2.CPUTime.Timestamp = 0
	assert.EqualValues(t, st1.CPUTime, st2.CPUTime)

	assert.Equal(t, st1.CreateTime, st2.CreateTime)
	assert.Equal(t, st1.OpenFdCount, st2.OpenFdCount)
	assert.Equal(t, st1.Status, st2.Status)
	assert.Equal(t, st1.NumThreads, st2.NumThreads)
	assert.EqualValues(t, st1.CtxSwitches, st2.CtxSwitches)
	assert.EqualValues(t, st1.MemInfo, st2.MemInfo)
	// gopsutil has a bug in statm parsing https://github.com/shirou/gopsutil/issues/277
	// so we compare after swapping the value of field `Data` and `Dirty` from gopsutil
	// TODO: fix the problem in gopsutil forked by `Datadog`
	st1.MemInfoEx.Dirty, st1.MemInfoEx.Data = st1.MemInfoEx.Data, st1.MemInfoEx.Dirty
	assert.EqualValues(t, st1.MemInfoEx, st2.MemInfoEx)
	assert.EqualValues(t, st1.IOStat, st2.IOStat)
}

func TestStatsForPIDsTestFS(t *testing.T) {
	t.Setenv("HOST_PROC", "resources/test_procfs/proc/")
	testStatsForPIDs(t, WithProcFSRoot("resources/test_procfs/proc/"))
}

func TestStatsForPIDsLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	testStatsForPIDs(t)
}

func testStatsForPIDs(t *testing.T, probeOptions ...Option) {
	// disable log output from gopsutil, the testFS doesn't have `cwd`, `fd` and `exe` dir setup,
	// gopsutil print verbose debug log regarding this
	seelog.UseLogger(seelog.Disabled)

	probe := getProbeWithPermission(probeOptions...)
	defer probe.Close()

	expectProcs, err := process.AllProcesses()
	require.NoError(t, err)

	pids := make([]int32, 0, len(expectProcs))

	// empty PIDs should yield empty stats
	stats, err := probe.StatsForPIDs(pids, time.Now())
	require.NoError(t, err)
	require.Empty(t, stats)

	for p := range expectProcs {
		pids = append(pids, p)
	}

	stats, err = probe.StatsForPIDs(pids, time.Now())
	require.NoError(t, err)
	assert.NotEmpty(t, stats)
	assert.Len(t, stats, len(pids))
	for pid, stat := range stats {
		assert.Contains(t, pids, pid)
		compareStats(t, ConvertFilledProcessesToStats(expectProcs[pid]), stat)
	}
}

func TestMultipleProbes(t *testing.T) {
	probe1 := getProbeWithPermission(WithProcFSRoot("resources/test_procfs/proc/"))
	defer probe1.Close()

	probe2 := getProbeWithPermission(WithProcFSRoot("resources/test_procfs/proc/"))
	defer probe2.Close()

	now := time.Now()

	procByPID1, err := probe1.ProcessesByPID(now, true)
	assert.NoError(t, err)
	resetNiceValues(procByPID1)
	procByPID2, err := probe2.ProcessesByPID(now, true)
	assert.NoError(t, err)
	resetNiceValues(procByPID2)
	for i := 0; i < 10; i++ {
		currProcByPID1, err := probe1.ProcessesByPID(now, true)
		assert.NoError(t, err)
		resetNiceValues(currProcByPID1)
		currProcByPID2, err := probe2.ProcessesByPID(now, true)
		assert.NoError(t, err)
		resetNiceValues(currProcByPID2)
		assert.EqualValues(t, currProcByPID1, currProcByPID2)
		assert.EqualValues(t, currProcByPID1, procByPID1)
		assert.EqualValues(t, currProcByPID2, procByPID2)
		procByPID1 = currProcByPID1
		procByPID2 = currProcByPID2
	}
}

func TestProcfsChange(t *testing.T) {
	probe := getProbeWithPermission(WithProcFSRoot("resources/test_procfs/proc"))
	defer probe.Close()

	now := time.Now()

	procByPID, err := probe.ProcessesByPID(now, true)
	assert.NoError(t, err)

	// update the procfs file structure to add a pid, make sure next time it reads in the updates
	err = os.Rename("resources/10389", "resources/test_procfs/proc/10389")
	assert.NoError(t, err)
	defer func() {
		err = os.Rename("resources/test_procfs/proc/10389", "resources/10389")
		assert.NoError(t, err)
	}()
	newProcByPID1, err := probe.ProcessesByPID(now, true)
	assert.NoError(t, err)
	assert.Contains(t, newProcByPID1, int32(10389))
	assert.NotContains(t, procByPID, int32(10389))

	// remove a pid from procfs, make sure it's gone from the result
	err = os.Rename("resources/test_procfs/proc/29613", "resources/29613")
	assert.NoError(t, err)
	defer func() {
		err = os.Rename("resources/29613", "resources/test_procfs/proc/29613")
		assert.NoError(t, err)
	}()
	newProcByPID2, err := probe.ProcessesByPID(now, true)
	assert.NoError(t, err)
	assert.NotContains(t, newProcByPID2, int32(29613))
	assert.Contains(t, procByPID, int32(29613))
}

func TestParseStatusLine(t *testing.T) {
	probe := getProbeWithPermission()
	defer probe.Close()

	for _, tc := range []struct {
		line     []byte
		expected *statusInfo
	}{
		{
			line: []byte("Name:\tpostgres"),
			expected: &statusInfo{
				name:        "postgres",
				memInfo:     &MemoryInfoStat{},
				ctxSwitches: &NumCtxSwitchesStat{},
			},
		},
		{
			line: []byte("Name:\t\t  \t\t\t  postgres"),
			expected: &statusInfo{
				name:        "postgres",
				memInfo:     &MemoryInfoStat{},
				ctxSwitches: &NumCtxSwitchesStat{},
			},
		},
		{
			line: []byte("State:\tS (sleeping)"),
			expected: &statusInfo{
				status:      "S",
				memInfo:     &MemoryInfoStat{},
				ctxSwitches: &NumCtxSwitchesStat{},
			},
		},
		{
			line: []byte("State:\tR (running)"),
			expected: &statusInfo{
				status:      "R",
				memInfo:     &MemoryInfoStat{},
				ctxSwitches: &NumCtxSwitchesStat{},
			},
		},
		{
			line: []byte("Uid:\t112\t112\t112\t112"),
			expected: &statusInfo{
				uids:        []int32{112, 112, 112, 112},
				memInfo:     &MemoryInfoStat{},
				ctxSwitches: &NumCtxSwitchesStat{},
			},
		},
		{
			line: []byte("Gid:\t1000\t1000\t1000\t1000"),
			expected: &statusInfo{
				gids:        []int32{1000, 1000, 1000, 1000},
				memInfo:     &MemoryInfoStat{},
				ctxSwitches: &NumCtxSwitchesStat{},
			},
		},
		{
			line: []byte("NSpid:\t123"),
			expected: &statusInfo{
				nspid:       123,
				memInfo:     &MemoryInfoStat{},
				ctxSwitches: &NumCtxSwitchesStat{},
			},
		},
		{
			line: []byte("NSpid:\t123\t456"),
			expected: &statusInfo{
				nspid:       456,
				memInfo:     &MemoryInfoStat{},
				ctxSwitches: &NumCtxSwitchesStat{},
			},
		},
		{
			line: []byte("Threads:\t1"),
			expected: &statusInfo{
				numThreads:  1,
				memInfo:     &MemoryInfoStat{},
				ctxSwitches: &NumCtxSwitchesStat{},
			},
		},
		{
			line: []byte("voluntary_ctxt_switches:\t3"),
			expected: &statusInfo{
				memInfo: &MemoryInfoStat{},
				ctxSwitches: &NumCtxSwitchesStat{
					Voluntary: 3,
				},
			},
		},
		{
			line: []byte("nonvoluntary_ctxt_switches:\t411"),
			expected: &statusInfo{
				memInfo: &MemoryInfoStat{},
				ctxSwitches: &NumCtxSwitchesStat{
					Involuntary: 411,
				},
			},
		},
		{
			line: []byte("bad status line"), // bad status
			expected: &statusInfo{
				memInfo:     &MemoryInfoStat{},
				ctxSwitches: &NumCtxSwitchesStat{},
			},
		},
		{
			line: []byte("Name:\t"), // edge case for parsing failure
			expected: &statusInfo{
				memInfo:     &MemoryInfoStat{},
				ctxSwitches: &NumCtxSwitchesStat{},
			},
		},
		{
			line: []byte("VmRSS:\t712 kB"),
			expected: &statusInfo{
				memInfo:     &MemoryInfoStat{RSS: 712 * 1024},
				ctxSwitches: &NumCtxSwitchesStat{},
			},
		},
		{
			line: []byte("VmSize:\t14652 kB"),
			expected: &statusInfo{
				memInfo:     &MemoryInfoStat{VMS: 14652 * 1024},
				ctxSwitches: &NumCtxSwitchesStat{},
			},
		},
		{
			line: []byte("VmSwap:\t3 kB"),
			expected: &statusInfo{
				memInfo:     &MemoryInfoStat{Swap: 3 * 1024},
				ctxSwitches: &NumCtxSwitchesStat{},
			},
		},
	} {
		result := &statusInfo{memInfo: &MemoryInfoStat{}, ctxSwitches: &NumCtxSwitchesStat{}}
		probe.parseStatusLine(tc.line, result)
		assert.EqualValues(t, tc.expected, result)
	}
}

func TestParseStatusTestFS(t *testing.T) {
	t.Setenv("HOST_PROC", "resources/test_procfs/proc/")
	testParseStatus(t, WithProcFSRoot("resources/test_procfs/proc/"))
}

func TestParseStatusLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	testParseStatus(t)
}

func testParseStatus(t *testing.T, probeOptions ...Option) {
	probe := getProbeWithPermission(probeOptions...)
	defer probe.Close()

	pids, err := probe.getActivePIDs()
	assert.NoError(t, err)

	for _, pid := range pids {
		actual := probe.parseStatus(filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid))))
		expProc, err := process.NewProcess(pid)
		assert.NoError(t, err)

		expName, err := expProc.Name()
		assert.NoError(t, err)
		expStatus, err := expProc.Status()
		assert.NoError(t, err)
		expUIDs, err := expProc.Uids()
		assert.NoError(t, err)
		expGIDs, err := expProc.Gids()
		assert.NoError(t, err)
		expThreads, err := expProc.NumThreads()
		assert.NoError(t, err)
		expMemInfo, err := expProc.MemoryInfo()
		assert.NoError(t, err)
		expCtxSwitches, err := expProc.NumCtxSwitches()
		assert.NoError(t, err)

		assert.Equal(t, expName, actual.name)
		assert.Equal(t, expStatus, actual.status)
		assert.EqualValues(t, expUIDs, actual.uids)
		assert.EqualValues(t, expGIDs, actual.gids)
		assert.Equal(t, expThreads, actual.numThreads)
		assert.Equal(t, expProc.NsPid, actual.nspid)

		assert.Equal(t, expMemInfo.RSS, actual.memInfo.RSS)
		assert.Equal(t, expMemInfo.VMS, actual.memInfo.VMS)

		assert.Equal(t, expCtxSwitches.Voluntary, actual.ctxSwitches.Voluntary)
		assert.Equal(t, expCtxSwitches.Involuntary, actual.ctxSwitches.Involuntary)
	}
}

func TestFillNsPidFromStatus(t *testing.T) {
	probe := getProbeWithPermission(WithProcFSRoot("resources/test_procfs/proc/"))
	defer probe.Close()

	t.Run("Linux versions 4.1+", func(t *testing.T) {
		// Process started on the host namespace
		// NSpid:	6320
		pid := 6320
		actual := probe.parseStatus(filepath.Join(probe.procRootLoc, strconv.Itoa(pid)))
		assert.Equal(t, int32(pid), actual.nspid)

		// Process spawned from P1 with its own namespace
		// NSpid:	6321	1
		pid = 6321
		actual = probe.parseStatus(filepath.Join(probe.procRootLoc, strconv.Itoa(pid)))
		assert.Equal(t, int32(1), actual.nspid)

		// Process spawned from P2 with its own namespace
		// NSpid:	6322	2	1
		pid = 6322
		actual = probe.parseStatus(filepath.Join(probe.procRootLoc, strconv.Itoa(pid)))
		assert.Equal(t, int32(1), actual.nspid)
	})

	t.Run("Older Linux versions", func(t *testing.T) {
		// The following processes have been generated by the same testing program but
		// older versions of Linux don't provide the NSpid on the status file.
		// We expect the library not to populate the NsPid field
		// True NSpid:	8225
		pid := 8225
		actual := probe.parseStatus(filepath.Join(probe.procRootLoc, strconv.Itoa(pid)))
		assert.Equal(t, int32(0), actual.nspid)

		// Process spawned from P1 with its own namespace
		// True NSpid:	8226	1
		pid = 8226
		actual = probe.parseStatus(filepath.Join(probe.procRootLoc, strconv.Itoa(pid)))
		assert.Equal(t, int32(0), actual.nspid)

		// Process spawned from P2 with its own namespace
		// True NSpid:	8227	2	1
		pid = 8227
		actual = probe.parseStatus(filepath.Join(probe.procRootLoc, strconv.Itoa(pid)))
		assert.Equal(t, int32(0), actual.nspid)
	})
}

func TestParseIOLine(t *testing.T) {
	probe := getProbeWithPermission()
	defer probe.Close()

	for _, tc := range []struct {
		line     []byte
		expected *IOCountersStat
	}{
		{
			line:     []byte("syscr: 467721"),
			expected: &IOCountersStat{ReadCount: 467721},
		},
		{
			line:     []byte("syscw: 4842687"),
			expected: &IOCountersStat{WriteCount: 4842687},
		},
		{
			line:     []byte("read_bytes: 65536"),
			expected: &IOCountersStat{ReadBytes: 65536},
		},
		{
			line:     []byte("write_bytes: 4096"),
			expected: &IOCountersStat{WriteBytes: 4096},
		},
	} {
		result := &IOCountersStat{}
		probe.parseIOLine(tc.line, result)
		assert.EqualValues(t, tc.expected, result)
	}
}

func TestParseIOTestFS(t *testing.T) {
	t.Setenv("HOST_PROC", "resources/test_procfs/proc/")
	testParseIO(t, WithProcFSRoot("resources/test_procfs/proc/"))
}

func TestParseIOLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	testParseIO(t)
}

func testParseIO(t *testing.T, probeOptions ...Option) {
	probe := getProbeWithPermission(probeOptions...)
	defer probe.Close()

	pids, err := probe.getActivePIDs()
	require.NoError(t, err)

	for _, pid := range pids {
		actual := probe.parseIO(filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid))))
		expProc, err := process.NewProcess(pid)
		require.NoError(t, err)
		expIO, err := expProc.IOCounters()
		require.NoError(t, err)
		assert.EqualValues(t, ConvertFromIOStats(expIO), actual)
	}
}

func TestFetchFieldsWithoutPermission(t *testing.T) {
	t.Skip("This test is not working in CI, but could be tested locally")
	probe := getProbe()
	defer probe.Close()
	// PID 1 should be owned by root so we would always get permission error
	pid := int32(1)
	actual := probe.parseIO(filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid))))
	assert.Equal(t, int64(-1), actual.ReadCount)
	assert.Equal(t, int64(-1), actual.ReadBytes)
	assert.Equal(t, int64(-1), actual.WriteCount)
	assert.Equal(t, int64(-1), actual.WriteBytes)
	fd := probe.getFDCount(strconv.Itoa(int(pid)))
	assert.Equal(t, int32(-1), fd)
}

func TestParseStatContent(t *testing.T) {
	probe := getProbeWithPermission()
	defer probe.Close()

	// hard code the bootTime so we get consistent calculation for createTime
	probe.bootTime.Store(1606181252)
	now := time.Now()

	testCases := []struct {
		name           string
		line           []byte
		expected       *statInfo
		isKernelThread bool
	}{
		{
			line: []byte("1 (systemd) S 0 1 1 0 -1 4194560 425768 306165945 70 4299 4890 2184 563120 375308 20 0 1 0 15 189849600 1541 18446744073709551615 94223912931328 94223914360080 140733806473072 140733806469312 140053573122579 0 671173123 4096 1260 1 0 0 17 0 0 0 155 0 0 94223914368000 942\n23914514184 94223918080000 140733806477086 140733806477133 140733806477133 140733806477283 0"),
			expected: &statInfo{
				ppid:       0,
				createTime: 1606181252000,
				cpuStat: &CPUTimesStat{
					User:      48.9,
					System:    21.84,
					Timestamp: now.Unix(),
				},
				flags: 4194560,
			},
			isKernelThread: false,
		},
		{
			name: "command line has brackets around",
			line: []byte("1 ((sd-pam)) S 0 1 1 0 -1 4194560 425768 306165945 70 4299 4890 2184 563120 375308 20 0 1 0 15 189849600 1541 18446744073709551615 94223912931328 94223914360080 140733806473072 140733806469312 140053573122579 0 671173123 4096 1260 1 0 0 17 0 0 0 155 0 0 94223914368000 942\n23914514184 94223918080000 140733806477086 140733806477133 140733806477133 140733806477283 0"),
			expected: &statInfo{
				ppid:       0,
				createTime: 1606181252000,
				cpuStat: &CPUTimesStat{
					User:      48.9,
					System:    21.84,
					Timestamp: now.Unix(),
				},
				flags: 4194560,
			},
			isKernelThread: false,
		},
		{
			name: "fields are separated by multiple white spaces",
			line: []byte("5  (kworker/0:0H)   S 2 0 0 0 -1   69238880 0 0  0 0  0 0 0 0 0  -20 1 0 17 0 0 18446744073709551615 0 0 0 0 0 0 0 2147483647 0 0 0 0 17 0 0 0 0 0 0 0 0 0 0 0 0 0 0"),
			expected: &statInfo{
				ppid:       2,
				createTime: 1606181252000,
				cpuStat: &CPUTimesStat{
					User:      0,
					System:    0,
					Timestamp: now.Unix(),
				},
				flags: 69238880,
			},
			isKernelThread: true,
		},
		{
			name: "flags are greater than int32",
			line: []byte("44 (kintegrityd/0) S 2 0 0 0 -1 2216722496 0 0 0 0 0 0 0 0 20 0 1 0 31 0 0 18446744073709551615 0 0 0 0 0 0 0 2147483647 0 18446744071579499573 0 0 17 0 0 0 0 0 0"),
			expected: &statInfo{
				ppid:       2,
				createTime: 1606181252000,
				cpuStat: &CPUTimesStat{
					User:      0,
					System:    0,
					Timestamp: now.Unix(),
				},
				flags: 2216722496,
			},
			isKernelThread: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := probe.parseStatContent(tc.line, &statInfo{cpuStat: &CPUTimesStat{}}, int32(1), now)
			// nice value is fetched at the run time so we just assign the actual value for the sake for comparison
			tc.expected.nice = actual.nice
			assert.EqualValues(t, tc.expected, actual)
			assert.Equal(t, tc.isKernelThread, isKernelThread(actual.flags))
		})
	}
}

func TestParseStatTestFS(t *testing.T) {
	t.Setenv("HOST_PROC", "resources/test_procfs/proc/")
	testParseStat(t, WithProcFSRoot("resources/test_procfs/proc/"))
}

// TestParseStatLocalFS has to run on its own because gopsutil caches boot time,
// so other tests might set the boot time to a different value, and the values
// in this tests would be messed up
func TestParseStatLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	testParseStat(t)
}

func testParseStat(t *testing.T, probeOptions ...Option) {
	probe := getProbeWithPermission(probeOptions...)
	defer probe.Close()

	pids, err := probe.getActivePIDs()
	assert.NoError(t, err)

	for _, pid := range pids {
		actual := probe.parseStat(filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid))), pid, time.Now())
		expProc, err := process.NewProcess(pid)
		assert.NoError(t, err)
		expCreate, err := expProc.CreateTime()
		assert.NoError(t, err)
		expPpid, err := expProc.Ppid()
		assert.NoError(t, err)
		exptimes, err := expProc.Times()
		assert.NoError(t, err)

		assert.Equal(t, expCreate, actual.createTime)
		assert.Equal(t, expPpid, actual.ppid)
		assert.Equal(t, exptimes.User, actual.cpuStat.User)
		assert.Equal(t, exptimes.System, actual.cpuStat.System)
	}
}

func TestBootTime(t *testing.T) {
	bootT, err := bootTime("resources/test_procfs/proc/")
	assert.NoError(t, err)
	assert.Equal(t, uint64(1606127264), bootT)
}

// TestBootTimeLocalFS has to run on its own because gopsutil caches boot time,
// so other tests might set the boot time to a different value
func TestBootTimeLocalFS(t *testing.T) {
	maySkipLocalTest(t)

	probe := getProbeWithPermission()
	defer probe.Close()
	expectT, err := host.BootTime()
	assert.NoError(t, err)
	assert.Equal(t, expectT, probe.bootTime.Load())
}

func TestBootTimeRefresh(t *testing.T) {
	probe := getProbeWithPermission(WithBootTimeRefreshInterval(500*time.Millisecond), WithProcFSRoot("resources/test_procfs/proc/"))
	defer probe.Close()

	assert.Equal(t, uint64(1606127264), probe.bootTime.Load())
	err := os.Rename("resources/test_procfs/proc/stat", "resources/test_procfs/proc/stat_temp")
	require.NoError(t, err)
	err = os.Rename("resources/test_procfs/proc/stat2", "resources/test_procfs/proc/stat")
	require.NoError(t, err)

	assert.Eventually(t, func() bool { return uint64(1606127364) == probe.bootTime.Load() }, time.Second, 100*time.Millisecond)

	err = os.Rename("resources/test_procfs/proc/stat", "resources/test_procfs/proc/stat2")
	require.NoError(t, err)
	err = os.Rename("resources/test_procfs/proc/stat_temp", "resources/test_procfs/proc/stat")
	require.NoError(t, err)
}

func TestParseStatmTestFS(t *testing.T) {
	t.Setenv("HOST_PROC", "resources/test_procfs/proc/")
	testParseStatm(t, WithProcFSRoot("resources/test_procfs/proc/"))
}

func TestParseStatmLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	testParseStatm(t)
}

func testParseStatm(t *testing.T, probeOptions ...Option) {
	probe := getProbeWithPermission(probeOptions...)
	defer probe.Close()

	pids, err := probe.getActivePIDs()
	assert.NoError(t, err)

	for _, pid := range pids {
		actual := probe.parseStatm(filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid))))
		expProc, err := process.NewProcess(pid)
		assert.NoError(t, err)
		memInfo, err := expProc.MemoryInfoEx()
		assert.NoError(t, err)
		assert.Equal(t, memInfo.VMS, actual.VMS)
		assert.Equal(t, memInfo.RSS, actual.RSS, pid)
		assert.Equal(t, memInfo.Shared, actual.Shared)
		assert.Equal(t, memInfo.Text, actual.Text)
		assert.Equal(t, memInfo.Lib, actual.Lib)
		// gopsutil has a bug in statm parsing https://github.com/shirou/gopsutil/issues/277
		// so we compare after swapping the value of field `Data` and `Dirty` from gopsutil
		// TODO: fix the problem in gopsutil forked by `Datadog`
		assert.Equal(t, memInfo.Data, actual.Dirty)
		assert.Equal(t, memInfo.Dirty, actual.Data)
	}
}

func TestParseStatmStatusMatchTestFS(t *testing.T) {
	testParseStatmStatusMatch(t, WithProcFSRoot("resources/test_procfs/proc/"))
}

func TestParseStatmStatusMatchLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	testParseStatmStatusMatch(t)
}

func testParseStatmStatusMatch(t *testing.T, probeOptions ...Option) {
	probe := getProbeWithPermission(probeOptions...)
	defer probe.Close()

	pids, err := probe.getActivePIDs()
	assert.NoError(t, err)

	for _, pid := range pids {
		statm := probe.parseStatm(filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid))))
		status := probe.parseStatus(filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid))))
		assert.Equal(t, statm.VMS, status.memInfo.VMS)
		assert.Equal(t, statm.RSS, status.memInfo.RSS)
	}
}

func TestGetLinkWithAuthCheckTestFS(t *testing.T) {
	t.Setenv("HOST_PROC", "resources/test_procfs/proc/")

	testGetLinkWithAuthCheck(t)
}

func TestGetLinkWithAuthCheckLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	testGetLinkWithAuthCheck(t)
}

func testGetLinkWithAuthCheck(t *testing.T) {
	probe := getProbeWithPermission()
	defer probe.Close()

	pids, err := probe.getActivePIDs()
	assert.NoError(t, err)

	for _, pid := range pids {
		pathForPID := filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid)))
		cwd := probe.getLinkWithAuthCheck(pathForPID, "cwd")
		exe := probe.getLinkWithAuthCheck(pathForPID, "exe")

		expProc, err := process.NewProcess(pid)
		assert.NoError(t, err)
		if expCwd, err := expProc.Cwd(); err == nil {
			assert.Equal(t, expCwd, cwd)
		}
		if expExe, err := expProc.Exe(); err == nil {
			assert.Equal(t, expExe, exe)
		}
	}
}

func TestGetFDCountLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	probe := getProbe()
	defer probe.Close()

	pids, err := probe.getActivePIDs()
	assert.NoError(t, err)

	for _, pid := range pids {
		pathForPID := filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid)))
		fdCount := probe.getFDCount(pathForPID)
		expProc, err := process.NewProcess(pid)
		assert.NoError(t, err)
		// test both with and without permission issues
		if expFdCount, err := expProc.NumFDs(); err == nil {
			assert.Equal(t, expFdCount, fdCount)
		} else {
			assert.Equal(t, int32(-1), fdCount)
		}
	}
}

func TestStatsWithPermByPID(t *testing.T) {
	// create a fd dir so that the FD collection doesn't return -1
	err := os.Mkdir("resources/zero_io/3/fd", 0500)
	t.Cleanup(func() { _ = os.Remove("resources/zero_io/3/fd") })
	require.NoError(t, err)

	probe := getProbeWithPermission(WithProcFSRoot("resources/zero_io"))
	defer probe.Close()

	WithReturnZeroPermStats(true)(probe)
	pid := int32(3)
	stats, err := probe.StatsWithPermByPID([]int32{pid})
	require.NoError(t, err)
	require.Contains(t, stats, pid)
	assert.True(t, stats[pid].IOStat.IsZeroValue())

	WithReturnZeroPermStats(false)(probe)
	stats, err = probe.StatsWithPermByPID([]int32{pid})
	require.NoError(t, err)
	assert.Empty(t, stats)
}

func TestStatsForPIDsAndPerm(t *testing.T) {
	probe := getProbeWithPermission(WithProcFSRoot("resources/test_procfs/proc"))
	defer probe.Close()
	stats, err := probe.StatsForPIDs([]int32{1}, time.Now())
	require.NoError(t, err)
	require.Contains(t, stats, int32(1))
	assert.False(t, stats[1].IOStat.IsZeroValue())

	WithPermission(false)(probe)
	stats, err = probe.StatsForPIDs([]int32{1}, time.Now())
	require.NoError(t, err)
	require.Contains(t, stats, int32(1))
	assert.EqualValues(t, &IOCountersStat{
		ReadCount:  -1,
		WriteCount: -1,
		ReadBytes:  -1,
		WriteBytes: -1,
	}, stats[1].IOStat)
}

func TestProcessesByPIDsAndPerm(t *testing.T) {
	probe := getProbeWithPermission(WithProcFSRoot("resources/test_procfs/proc"))
	probe.procRootLoc = "resources/test_procfs/proc"
	defer probe.Close()
	procs, err := probe.ProcessesByPID(time.Now(), true)
	require.NoError(t, err)
	for _, p := range procs {
		assert.False(t, p.Stats.IOStat.IsZeroValue())
	}

	WithPermission(false)(probe)
	procs, err = probe.ProcessesByPID(time.Now(), true)
	require.NoError(t, err)
	for _, p := range procs {
		assert.EqualValues(t, &IOCountersStat{
			ReadCount:  -1,
			WriteCount: -1,
			ReadBytes:  -1,
			WriteBytes: -1,
		}, p.Stats.IOStat)
	}
}

func BenchmarkGetCmdGopsutilTestFS(b *testing.B) {
	b.Setenv("HOST_PROC", "resources/test_procfs/proc")

	benchmarkGetCmdGopsutil(b)
}

func BenchmarkGetCmdProcutilTestFS(b *testing.B) {
	b.Setenv("HOST_PROC", "resources/test_procfs/proc")

	benchmarkGetCmdProcutil(b)
}

func BenchmarkGetCmdGopsutilLocalFS(b *testing.B) {
	benchmarkGetCmdGopsutil(b)
}

func BenchmarkGetCmdProcutilLocalFS(b *testing.B) {
	benchmarkGetCmdProcutil(b)
}

func benchmarkGetCmdGopsutil(b *testing.B) {
	pids, err := process.Pids()
	require.NoError(b, err)

	for i := 0; i < b.N; i++ {
		for _, pid := range pids {
			proc, err := process.NewProcess(pid)
			if err == nil {
				_, _ = proc.Cmdline()
			}
		}
	}
}

func benchmarkGetCmdProcutil(b *testing.B) {
	probe := getProbeWithPermission()
	defer probe.Close()

	pids, err := probe.getActivePIDs()
	require.NoError(b, err)

	for i := 0; i < b.N; i++ {
		for _, pid := range pids {
			probe.getCmdline(filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid))))
		}
	}
}

func BenchmarkTestFSStatusGopsutil(b *testing.B) {
	hostProc := "resources/test_procfs/proc"
	b.Setenv("HOST_PROC", hostProc)

	pids, err := process.Pids()
	require.NoError(b, err)

	for i := 0; i < b.N; i++ {
		for _, pid := range pids {
			expProc, err := process.NewProcess(pid)
			if err == nil {
				_, _ = expProc.Status()
			}
		}
	}
}

func BenchmarkTestFSStatusProcutil(b *testing.B) {
	hostProc := "resources/test_procfs/proc"
	b.Setenv("HOST_PROC", hostProc)

	probe := getProbeWithPermission()
	defer probe.Close()

	pids, err := probe.getActivePIDs()
	require.NoError(b, err)

	for i := 0; i < b.N; i++ {
		for _, pid := range pids {
			probe.parseStatus(filepath.Join(hostProc, strconv.Itoa(int(pid))))
		}
	}
}

func BenchmarkLocalFSStatusGopsutil(b *testing.B) {
	pids, err := process.Pids()
	assert.NoError(b, err)

	for i := 0; i < b.N; i++ {
		for _, pid := range pids {
			expProc, err := process.NewProcess(pid)
			if err == nil {
				_, _ = expProc.Status()
			}
		}
	}
}

func BenchmarkLocalFSStatusProcutil(b *testing.B) {
	probe := getProbeWithPermission()
	defer probe.Close()

	pids, err := probe.getActivePIDs()
	require.NoError(b, err)

	for i := 0; i < b.N; i++ {
		for _, pid := range pids {
			probe.parseStatus(filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid))))
		}
	}
}

func BenchmarkGetPIDsGopsutilLocalFS(b *testing.B) {
	for i := 0; i < b.N; i++ {
		// ignore errors for benchmarking
		_, _ = process.Pids()
	}
}

func BenchmarkGetPIDsProcutilLocalFS(b *testing.B) {
	probe := getProbeWithPermission()
	defer probe.Close()
	for i := 0; i < b.N; i++ {
		// ignore errors when doing benchmarking
		_, _ = probe.getActivePIDs()
	}
}

func BenchmarkParseIOGopsutilTestFS(b *testing.B) {
	b.Setenv("HOST_PROC", "resources/test_procfs/proc")

	benchmarkParseIOGopsutil(b)
}

func BenchmarkParseIOProcutilTestFS(b *testing.B) {
	b.Setenv("HOST_PROC", "resources/test_procfs/proc")

	benchmarkParseIOProcutil(b)
}

func BenchmarkParseIOGopsutilLocalFS(b *testing.B) {
	benchmarkParseIOGopsutil(b)
}

func BenchmarkParseIOProcutilLocalFS(b *testing.B) {
	benchmarkParseIOProcutil(b)
}

func benchmarkParseIOGopsutil(b *testing.B) {
	pids, err := process.Pids()
	require.NoError(b, err)

	for i := 0; i < b.N; i++ {
		for _, pid := range pids {
			// ignore error for benchmarking
			proc, _ := process.NewProcess(pid)
			// ignore permission error for benchmarking
			_, _ = proc.IOCounters()
		}
	}
}

func benchmarkParseIOProcutil(b *testing.B) {
	probe := getProbeWithPermission()
	defer probe.Close()

	pids, err := probe.getActivePIDs()
	require.NoError(b, err)

	for i := 0; i < b.N; i++ {
		for _, pid := range pids {
			probe.parseIO(filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid))))
		}
	}
}

func BenchmarkGetProcsGopsutilTestFS(b *testing.B) {
	b.Setenv("HOST_PROC", "resources/test_procfs/proc")

	benchmarkGetProcsGopsutil(b)
}

func BenchmarkGetProcsProcutilTestFS(b *testing.B) {
	b.Setenv("HOST_PROC", "resources/test_procfs/proc")

	benchmarkGetProcsProcutil(b)
}

func BenchmarkGetProcsGopsutilLocalFS(b *testing.B) {
	benchmarkGetProcsGopsutil(b)
}

func BenchmarkGetProcsProcutilLocalFS(b *testing.B) {
	benchmarkGetProcsProcutil(b)
}

func benchmarkGetProcsGopsutil(b *testing.B) {
	// disable log output from gopsutil
	seelog.UseLogger(seelog.Disabled)
	for i := 0; i < b.N; i++ {
		// ignore errors for benchmarking
		_, _ = process.AllProcesses()
	}
}

func benchmarkGetProcsProcutil(b *testing.B) {
	probe := getProbeWithPermission()
	defer probe.Close()

	now := time.Now()
	for i := 0; i < b.N; i++ {
		// ignore errors for benchmarking
		_, _ = probe.ProcessesByPID(now, true)
	}
}

func maySkipLocalTest(t *testing.T) {
	if skipLocalTest {
		t.Skip("flaky test in CI")
	}
}

func BenchmarkNativeReaddirnames(b *testing.B) {
	dirPath := "/tmp/benchmark_dir/"
	fileCount := 20000
	makeBenchmarkDir(b, dirPath, fileCount)
	defer os.RemoveAll(dirPath)

	for i := 0; i < b.N; i++ {
		d, err := os.Open(dirPath)
		assert.NoError(b, err)
		defer d.Close()

		names, err := d.Readdirnames(-1)
		assert.NoError(b, err)

		assert.Equal(b, fileCount, len(names))
	}
}

func BenchmarkImprovedReaddirnames(b *testing.B) {
	dirPath := "/tmp/benchmark_dir/"
	fileCount := 20000
	makeBenchmarkDir(b, dirPath, fileCount)
	defer os.RemoveAll(dirPath)

	for i := 0; i < b.N; i++ {
		d, err := os.Open(dirPath)
		assert.NoError(b, err)
		defer d.Close()

		buf := make([]byte, 8192)
		count := 0

		for i := 0; ; i++ {
			n, _ := syscall.ReadDirent(int(d.Fd()), buf)
			if n <= 0 {
				break
			}

			_, numDirs := countDirent(buf[:n])
			count += numDirs
		}

		assert.Equal(b, fileCount, count)
	}
}

func makeBenchmarkDir(b *testing.B, dirPath string, fileCount int) {
	err := os.Mkdir(dirPath, 0755)
	require.NoError(b, err)

	createEmptyFile := func(name string) {
		d := []byte("")
		err = os.WriteFile(name, d, 0755)
		require.NoError(b, err)
	}

	for i := 0; i < fileCount; i++ {
		createEmptyFile(dirPath + strconv.Itoa(i))
	}
}

// resetNiceValues takes a group of processes and reset the "nice" values on them.
// this is needed because the "nice" values are not extract from procfs but using system call,
// so it might cause test flakiness if we don't reset the value
func resetNiceValues(procs map[int32]*Process) {
	for _, p := range procs {
		p.Stats.Nice = 0
	}
}

func BenchmarkGetFDCount(b *testing.B) {
	probe := getProbe()
	defer probe.Close()

	for i := 0; i < 100; i++ {
		f, err := os.Open("/proc/self/comm")
		if err != nil {
			b.Fatal(err)
		}
		defer f.Close()
	}

	b.Run("self_proc", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = probe.getFDCount("/proc/self")
		}
	})
}
