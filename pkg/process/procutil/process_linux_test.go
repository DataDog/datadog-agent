// +build linux

package procutil

import (
	"io/ioutil"
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

func TestGetActivePIDs(t *testing.T) {
	os.Setenv("HOST_PROC", "resources/test_procfs/proc")
	defer os.Unsetenv("HOST_PROC")

	probe := NewProcessProbe()
	defer probe.Close()

	actual, err := probe.getActivePIDs()
	assert.NoError(t, err)

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
	os.Setenv("HOST_PROC", "resources/test_procfs/proc")
	defer os.Unsetenv("HOST_PROC")

	testGetCmdline(t)
}

func TestGetCmdlineLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	testGetCmdline(t)
}

func testGetCmdline(t *testing.T) {
	probe := NewProcessProbe()
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

func TestProcessesByPIDTestFS(t *testing.T) {
	os.Setenv("HOST_PROC", "resources/test_procfs/proc/")
	defer os.Unsetenv("HOST_PROC")

	testProcessesByPID(t)
}

func TestProcessesByPIDLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	testProcessesByPID(t)
}

func testProcessesByPID(t *testing.T) {
	// disable log output from gopsutil, the testFS doesn't have `cwd`, `fd` and `exe` dir setup,
	// gopsutil print verbose debug log regarding this
	seelog.UseLogger(seelog.Disabled)

	probe := NewProcessProbe()
	defer probe.Close()

	expectedProcs, err := process.AllProcesses()
	assert.NoError(t, err)

	procByPID, err := probe.ProcessesByPID(time.Now())
	assert.NoError(t, err)

	// make sure the process that has no command line doesn't get included in the output
	for pid, expectProc := range expectedProcs {
		cmd := strings.Join(probe.getCmdline(filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid)))), " ")
		if cmd == "" {
			assert.NotContains(t, procByPID, pid)
		} else {
			assert.Contains(t, procByPID, pid)
			compareFilledProcs(t, expectProc, ConvertToFilledProcess(procByPID[pid]))
		}
	}
}

func compareFilledProcs(t *testing.T, procV1, procV2 *process.FilledProcess) {
	assert.Equal(t, procV1.Pid, procV2.Pid)
	assert.Equal(t, procV1.Ppid, procV2.Ppid)
	assert.Equal(t, procV1.NsPid, procV2.NsPid)
	oldCmd := strings.Trim(strings.Join(procV1.Cmdline, " "), " ")
	newCmd := strings.Join(procV2.Cmdline, " ")
	assert.Equal(t, oldCmd, newCmd)
	// CPU Timestamp might be different between gopsutil and procutil fetches data,
	// so we compare with tolerance of 1s, then compare CpuTime without `Timestamp` field
	assert.InDelta(t, procV1.CpuTime.Timestamp, procV2.CpuTime.Timestamp, 1.0)
	procV1.CpuTime.Timestamp = 0
	procV2.CpuTime.Timestamp = 0
	assert.EqualValues(t, procV1.CpuTime, procV2.CpuTime)

	assert.Equal(t, procV1.CreateTime, procV2.CreateTime)
	assert.Equal(t, procV1.OpenFdCount, procV2.OpenFdCount)
	assert.Equal(t, procV1.Name, procV2.Name)
	assert.Equal(t, procV1.Status, procV2.Status)
	assert.ElementsMatch(t, procV1.Uids, procV2.Uids)
	assert.ElementsMatch(t, procV1.Gids, procV2.Gids)
	assert.Equal(t, procV1.NumThreads, procV2.NumThreads)
	assert.EqualValues(t, procV1.CtxSwitches, procV2.CtxSwitches)
	assert.EqualValues(t, procV1.MemInfo, procV2.MemInfo)
	// gopsutil has a bug in statm parsing https://github.com/shirou/gopsutil/issues/277
	// so we compare after swapping the value of field `Data` and `Dirty` from gopsutil
	// TODO: fix the problem in gopsutil forked by `Datadog`
	procV1.MemInfoEx.Dirty, procV1.MemInfoEx.Data = procV1.MemInfoEx.Data, procV1.MemInfoEx.Dirty
	assert.EqualValues(t, procV1.MemInfoEx, procV2.MemInfoEx)
	assert.Equal(t, procV1.Cwd, procV2.Cwd)
	assert.Equal(t, procV1.Exe, procV2.Exe)
	assert.EqualValues(t, procV1.IOStat, procV2.IOStat)
	assert.Equal(t, procV1.Username, procV2.Username)
}

func TestStatsForPIDsTestFS(t *testing.T) {
	os.Setenv("HOST_PROC", "resources/test_procfs/proc/")
	defer os.Unsetenv("HOST_PROC")

	testStatsForPIDs(t)
}

func TestStatsForPIDsLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	testStatsForPIDs(t)
}

func testStatsForPIDs(t *testing.T) {
	// disable log output from gopsutil, the testFS doesn't have `cwd`, `fd` and `exe` dir setup,
	// gopsutil print verbose debug log regarding this
	seelog.UseLogger(seelog.Disabled)

	probe := NewProcessProbe()
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
		compareStats(t, expectProcs[pid], ConvertToFilledProcess(&Process{Pid: pid, Stats: stat}))
	}
}

func compareStats(t *testing.T, procV1, procV2 *process.FilledProcess) {
	assert.Equal(t, procV1.Pid, procV2.Pid)
	// CPU Timestamp might be different between gopsutil and procutil fetches data,
	// so we compare with tolerance of 1s, then compare CpuTime without `Timestamp` field
	assert.InDelta(t, procV1.CpuTime.Timestamp, procV2.CpuTime.Timestamp, 1.0)
	procV1.CpuTime.Timestamp = 0
	procV2.CpuTime.Timestamp = 0
	assert.EqualValues(t, procV1.CpuTime, procV2.CpuTime)

	assert.Equal(t, procV1.CreateTime, procV2.CreateTime)
	assert.Equal(t, procV1.OpenFdCount, procV2.OpenFdCount)
	assert.Equal(t, procV1.Status, procV2.Status)
	assert.Equal(t, procV1.NumThreads, procV2.NumThreads)
	assert.EqualValues(t, procV1.CtxSwitches, procV2.CtxSwitches)
	assert.EqualValues(t, procV1.MemInfo, procV2.MemInfo)
	// gopsutil has a bug in statm parsing https://github.com/shirou/gopsutil/issues/277
	// so we compare after swapping the value of field `Data` and `Dirty` from gopsutil
	// TODO: fix the problem in gopsutil forked by `Datadog`
	procV1.MemInfoEx.Dirty, procV1.MemInfoEx.Data = procV1.MemInfoEx.Data, procV1.MemInfoEx.Dirty
	assert.EqualValues(t, procV1.MemInfoEx, procV2.MemInfoEx)
	assert.EqualValues(t, procV1.IOStat, procV2.IOStat)
}

func TestMultipleProbes(t *testing.T) {
	os.Setenv("HOST_PROC", "resources/test_procfs/proc/")
	defer os.Unsetenv("HOST_PROC")

	probe1 := NewProcessProbe()
	defer probe1.Close()

	probe2 := NewProcessProbe()
	defer probe2.Close()

	now := time.Now()

	procByPID1, err := probe1.ProcessesByPID(now)
	assert.NoError(t, err)
	resetNiceValues(procByPID1)
	procByPID2, err := probe2.ProcessesByPID(now)
	assert.NoError(t, err)
	resetNiceValues(procByPID2)
	for i := 0; i < 10; i++ {
		currProcByPID1, err := probe1.ProcessesByPID(now)
		assert.NoError(t, err)
		resetNiceValues(currProcByPID1)
		currProcByPID2, err := probe2.ProcessesByPID(now)
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
	os.Setenv("HOST_PROC", "resources/test_procfs/proc/")
	defer os.Unsetenv("HOST_PROC")

	probe := NewProcessProbe()
	defer probe.Close()

	now := time.Now()

	procByPID, err := probe.ProcessesByPID(now)
	assert.NoError(t, err)

	// update the procfs file structure to add a pid, make sure next time it reads in the updates
	err = os.Rename("resources/10389", "resources/test_procfs/proc/10389")
	assert.NoError(t, err)
	defer func() {
		err = os.Rename("resources/test_procfs/proc/10389", "resources/10389")
		assert.NoError(t, err)
	}()
	newProcByPID1, err := probe.ProcessesByPID(now)
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
	newProcByPID2, err := probe.ProcessesByPID(now)
	assert.NoError(t, err)
	assert.NotContains(t, newProcByPID2, int32(29613))
	assert.Contains(t, procByPID, int32(29613))
}

func TestParseStatusLine(t *testing.T) {
	probe := NewProcessProbe()
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
	os.Setenv("HOST_PROC", "resources/test_procfs/proc/")
	defer os.Unsetenv("HOST_PROC")

	testParseStatus(t)
}

func TestParseStatusLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	testParseStatus(t)
}

func testParseStatus(t *testing.T) {
	probe := NewProcessProbe()
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
		assert.Equal(t, expMemInfo.Swap, actual.memInfo.Swap)

		assert.Equal(t, expCtxSwitches.Voluntary, actual.ctxSwitches.Voluntary)
		assert.Equal(t, expCtxSwitches.Involuntary, actual.ctxSwitches.Involuntary)
	}
}

func TestFillNsPidFromStatus(t *testing.T) {
	os.Setenv("HOST_PROC", "resources/test_procfs/proc/")
	defer os.Unsetenv("HOST_PROC")

	probe := NewProcessProbe()
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
	probe := NewProcessProbe()
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
	os.Setenv("HOST_PROC", "resources/test_procfs/proc/")
	defer os.Unsetenv("HOST_PROC")

	testParseIO(t)
}

func TestParseIOLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	testParseIO(t)
}

func testParseIO(t *testing.T) {
	probe := NewProcessProbe()
	defer probe.Close()

	pids, err := probe.getActivePIDs()
	assert.NoError(t, err)

	for _, pid := range pids {
		actual := probe.parseIO(filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid))))
		expProc, err := process.NewProcess(pid)
		assert.NoError(t, err)
		expIO, err := expProc.IOCounters()
		assert.NoError(t, err)
		assert.Equal(t, expIO.ReadCount, actual.ReadCount)
		assert.Equal(t, expIO.ReadBytes, actual.ReadBytes)
		assert.Equal(t, expIO.WriteCount, actual.WriteCount)
		assert.Equal(t, expIO.WriteBytes, actual.WriteBytes)
	}
}

func TestParseStatContent(t *testing.T) {
	probe := NewProcessProbe()
	defer probe.Close()

	// hard code the bootTime so we get consistent calculation for createTime
	probe.bootTime = 1606181252
	now := time.Now()

	for _, tc := range []struct {
		line     []byte
		expected *statInfo
	}{
		// standard content
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
			},
		},
		// command line has brackets around
		{
			line: []byte("1 ((sd-pam)) S 0 1 1 0 -1 4194560 425768 306165945 70 4299 4890 2184 563120 375308 20 0 1 0 15 189849600 1541 18446744073709551615 94223912931328 94223914360080 140733806473072 140733806469312 140053573122579 0 671173123 4096 1260 1 0 0 17 0 0 0 155 0 0 94223914368000 942\n23914514184 94223918080000 140733806477086 140733806477133 140733806477133 140733806477283 0"),
			expected: &statInfo{
				ppid:       0,
				createTime: 1606181252000,
				cpuStat: &CPUTimesStat{
					User:      48.9,
					System:    21.84,
					Timestamp: now.Unix(),
				},
			},
		},
		// fields are separated by multiple white spaces
		{
			line: []byte("5  (kworker/0:0H)   S 2 0 0 0 -1   69238880 0 0  0 0  0 0 0 0 0  -20 1 0 17 0 0 18446744073709551615 0 0 0 0 0 0 0 2147483647 0 0 0 0 17 0 0 0 0 0 0 0 0 0 0 0 0 0 0"),
			expected: &statInfo{
				ppid:       2,
				createTime: 1606181252000,
				cpuStat: &CPUTimesStat{
					User:      0,
					System:    0,
					Timestamp: now.Unix(),
				},
			},
		},
	} {

		actual := probe.parseStatContent(tc.line, &statInfo{cpuStat: &CPUTimesStat{}}, int32(1), now)
		// nice value is fetched at the run time so we just assign the actual value for the sake for comparison
		tc.expected.nice = actual.nice
		assert.EqualValues(t, tc.expected, actual)
	}
}

func TestParseStatTestFS(t *testing.T) {
	os.Setenv("HOST_PROC", "resources/test_procfs/proc/")
	defer os.Unsetenv("HOST_PROC")

	testParseStat(t)
}

// TestParseStatLocalFS has to run on its own because gopsutil caches boot time,
// so other tests might set the boot time to a different value, and the values
// in this tests would be messed up
func TestParseStatLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	testParseStat(t)
}

func testParseStat(t *testing.T) {
	probe := NewProcessProbe()
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

	probe := NewProcessProbe()
	defer probe.Close()
	expectT, err := host.BootTime()
	assert.NoError(t, err)
	assert.Equal(t, expectT, probe.bootTime)
}

func TestParseStatmTestFS(t *testing.T) {
	os.Setenv("HOST_PROC", "resources/test_procfs/proc/")
	defer os.Unsetenv("HOST_PROC")

	testParseStatm(t)
}

func TestParseStatmLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	testParseStatm(t)
}

func testParseStatm(t *testing.T) {
	probe := NewProcessProbe()
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
	os.Setenv("HOST_PROC", "resources/test_procfs/proc/")
	defer os.Unsetenv("HOST_PROC")

	testParseStatmStatusMatch(t)
}

func TestParseStatmStatusMatchLocalFS(t *testing.T) {
	maySkipLocalTest(t)
	testParseStatmStatusMatch(t)
}

func testParseStatmStatusMatch(t *testing.T) {
	probe := NewProcessProbe()
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

func TestGetLinkWithAuthCheck(t *testing.T) {
	maySkipLocalTest(t)
	probe := NewProcessProbe()
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
	probe := NewProcessProbe()
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

func TestGetFDCountLocalFSImproved(t *testing.T) {
	maySkipLocalTest(t)
	probe := NewProcessProbe()
	defer probe.Close()

	pids, err := probe.getActivePIDs()
	assert.NoError(t, err)

	for _, pid := range pids {
		pathForPID := filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid)))
		fdCount := probe.getFDCountImproved(pathForPID)
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

func BenchmarkGetCmdGopsutilTestFS(b *testing.B) {
	os.Setenv("HOST_PROC", "resources/test_procfs/proc")
	defer os.Unsetenv("HOST_PROC")

	benchmarkGetCmdGopsutil(b)
}

func BenchmarkGetCmdProcutilTestFS(b *testing.B) {
	os.Setenv("HOST_PROC", "resources/test_procfs/proc")
	defer os.Unsetenv("HOST_PROC")

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
	probe := NewProcessProbe()
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
	os.Setenv("HOST_PROC", hostProc)
	defer os.Unsetenv("HOST_PROC")

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
	os.Setenv("HOST_PROC", hostProc)
	defer os.Unsetenv("HOST_PROC")

	probe := NewProcessProbe()
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
	probe := NewProcessProbe()
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
	probe := NewProcessProbe()
	defer probe.Close()
	for i := 0; i < b.N; i++ {
		// ignore errors when doing benchmarking
		_, _ = probe.getActivePIDs()
	}
}

func BenchmarkParseIOGopsutilTestFS(b *testing.B) {
	os.Setenv("HOST_PROC", "resources/test_procfs/proc")
	defer os.Unsetenv("HOST_PROC")

	benchmarkParseIOGopsutil(b)
}

func BenchmarkParseIOProcutilTestFS(b *testing.B) {
	os.Setenv("HOST_PROC", "resources/test_procfs/proc")
	defer os.Unsetenv("HOST_PROC")

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
	probe := NewProcessProbe()
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
	os.Setenv("HOST_PROC", "resources/test_procfs/proc")
	defer os.Unsetenv("HOST_PROC")

	benchmarkGetProcsGopsutil(b)
}

func BenchmarkGetProcsProcutilTestFS(b *testing.B) {
	os.Setenv("HOST_PROC", "resources/test_procfs/proc")
	defer os.Unsetenv("HOST_PROC")

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
	probe := NewProcessProbe()
	defer probe.Close()

	now := time.Now()
	for i := 0; i < b.N; i++ {
		// ignore errors for benchmarking
		_, _ = probe.ProcessesByPID(now)
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
		err = ioutil.WriteFile(name, d, 0755)
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
