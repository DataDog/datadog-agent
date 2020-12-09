// +build linux

package procutil

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/gopsutil/process"
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
	// this test is flaky as the underlying procfs could change during
	// the comparison of procutil and gopsutil,
	// but we could use it to test locally
	t.Skip("flaky test in CI")
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
	// this test is flaky as the underlying procfs could change during
	// the comparison of procutil and gopsutil,
	// but we could use it to test locally
	t.Skip("flaky test in CI")
	testProcessesByPID(t)
}

func testProcessesByPID(t *testing.T) {
	probe := NewProcessProbe()
	defer probe.Close()

	pids, err := probe.getActivePIDs()
	assert.NoError(t, err)

	procByPID, err := probe.ProcessesByPID()
	assert.NoError(t, err)

	// make sure the process that has no command line doesn't get included in the output
	for _, pid := range pids {
		cmd := strings.Join(probe.getCmdline(filepath.Join(probe.procRootLoc, strconv.Itoa(int(pid)))), " ")
		if cmd == "" {
			assert.NotContains(t, procByPID, pid)
		} else {
			assert.Contains(t, procByPID, pid)
		}
	}
}

func TestMultipleProbes(t *testing.T) {
	os.Setenv("HOST_PROC", "resources/test_procfs/proc/")
	defer os.Unsetenv("HOST_PROC")

	probe1 := NewProcessProbe()
	defer probe1.Close()

	probe2 := NewProcessProbe()
	defer probe2.Close()

	procByPID1, err := probe1.ProcessesByPID()
	assert.NoError(t, err)
	procByPID2, err := probe2.ProcessesByPID()
	assert.NoError(t, err)
	for i := 0; i < 10; i++ {
		currProcByPID1, err := probe1.ProcessesByPID()
		assert.NoError(t, err)
		currProcByPID2, err := probe2.ProcessesByPID()
		assert.NoError(t, err)
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

	procByPID, err := probe.ProcessesByPID()
	assert.NoError(t, err)

	// update the procfs file structure to add a pid, make sure next time it reads in the updates
	err = os.Rename("resources/10389", "resources/test_procfs/proc/10389")
	assert.NoError(t, err)
	defer func() {
		err = os.Rename("resources/test_procfs/proc/10389", "resources/10389")
		assert.NoError(t, err)
	}()
	newProcByPID1, err := probe.ProcessesByPID()
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
	newProcByPID2, err := probe.ProcessesByPID()
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
	// this test is flaky as the underlying procfs could change during
	// the comparison of procutil and gopsutil,
	// but we could use it to test locally
	t.Skip("flaky test in CI")
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
			require.NoError(b, err)
			_, err = proc.Cmdline()
			require.NoError(b, err)
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

func BenchmarkGetPIDsGopsutilLocalFS(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := process.Pids()
		require.NoError(b, err)
	}
}

func BenchmarkGetPIDsProcutilLocalFS(b *testing.B) {
	probe := NewProcessProbe()
	defer probe.Close()
	for i := 0; i < b.N; i++ {
		_, err := probe.getActivePIDs()
		require.NoError(b, err)
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
