// +build linux

package procutil

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

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
			input:  []byte{0, 0, 47, 115, 98, 105, 110, 47, 105, 110, 105, 116, 0},
			expect: []string{string([]byte{47, 115, 98, 105, 110, 47, 105, 110, 105, 116})},
		},
	} {
		actual := trimAndSplitBytes(tc.input)
		assert.ElementsMatch(t, tc.expect, actual)
	}
}

func TestGetCmdline(t *testing.T) {
	hostProc := "resources/test_procfs/proc"
	os.Setenv("HOST_PROC", hostProc)
	defer os.Unsetenv("HOST_PROC")

	probe := NewProcessProbe()
	defer probe.Close()

	pids, err := probe.getActivePIDs()
	assert.NoError(t, err)

	for _, pid := range pids {
		actual := strings.Join(probe.getCmdline(filepath.Join(hostProc, strconv.Itoa(int(pid)))), " ")
		expProc, err := process.NewProcess(pid)
		assert.NoError(t, err)

		expect, err := expProc.Cmdline()
		assert.NoError(t, err)

		assert.Equal(t, expect, actual)
	}
}

func TestParseStatus(t *testing.T) {
	hostProc := "resources/linux_postgres/proc"
	os.Setenv("HOST_PROC", hostProc)
	defer os.Unsetenv("HOST_PROC")

	probe := NewProcessProbe()
	defer probe.Close()

	pids, err := probe.getActivePIDs()
	assert.NoError(t, err)

	for _, pid := range pids {
		actual := probe.parseStatus(filepath.Join(hostProc, strconv.Itoa(int(pid))))
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

		assert.Equal(t, expMemInfo.RSS, actual.memInfo.RSS)
		assert.Equal(t, expMemInfo.VMS, actual.memInfo.VMS)
		assert.Equal(t, expMemInfo.Swap, actual.memInfo.Swap)

		assert.Equal(t, expCtxSwitches.Voluntary, actual.ctxSwitches.Voluntary)
		assert.Equal(t, expCtxSwitches.Involuntary, actual.ctxSwitches.Involuntary)
	}
}
