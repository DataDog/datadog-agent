// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"math/rand"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

//nolint:deadcode,unused
func procsToHash(procs []*procutil.Process) (procsByPid map[int32]*procutil.Process) {
	procsByPid = make(map[int32]*procutil.Process)
	for _, p := range procs {
		procsByPid[p.Pid] = p
	}
	return
}

func makeProcess(pid int32, cmdline string) *procutil.Process {
	return &procutil.Process{
		Pid:     pid,
		Cmdline: strings.Split(cmdline, " "),
		Stats: &procutil.Stats{
			CPUPercent: &procutil.CPUPercentStat{
				UserPct:   float64(rand.Uint64()),
				SystemPct: float64(rand.Uint64()),
			},
			MemInfo: &procutil.MemoryInfoStat{
				RSS: rand.Uint64(),
				VMS: rand.Uint64(),
			},
			MemInfoEx:   &procutil.MemoryInfoExStat{},
			IOStat:      &procutil.IOCountersStat{},
			CtxSwitches: &procutil.NumCtxSwitchesStat{},
		},
	}
}

func makeProcessWithCreateTime(pid int32, cmdline string, createTime int64) *procutil.Process {
	p := makeProcess(pid, cmdline)
	p.Stats.CreateTime = createTime
	return p
}

func makeProcessModel(t *testing.T, process *procutil.Process) *model.Process {
	t.Helper()

	stats := process.Stats
	mem := stats.MemInfo
	cpu := stats.CPUPercent
	return &model.Process{
		Pid:     process.Pid,
		Command: &model.Command{Args: process.Cmdline},
		User:    &model.ProcessUser{},
		Memory:  &model.MemoryStat{Rss: mem.RSS, Vms: mem.VMS},
		Cpu: &model.CPUStat{
			LastCpu:   "cpu",
			UserPct:   float32(cpu.UserPct),
			SystemPct: float32(cpu.SystemPct),
			TotalPct:  float32(cpu.UserPct + cpu.SystemPct),
		},
		CreateTime: process.Stats.CreateTime,
		IoStat:     &model.IOStat{},
	}
}

func makeProcessStatModels(t *testing.T, processes ...*procutil.Process) []*model.ProcessStat {
	t.Helper()

	models := make([]*model.ProcessStat, 0, len(processes))
	for _, process := range processes {
		stats := process.Stats
		mem := stats.MemInfo
		cpu := stats.CPUPercent
		models = append(models, &model.ProcessStat{
			Pid:    process.Pid,
			Memory: &model.MemoryStat{Rss: mem.RSS, Vms: mem.VMS},
			Cpu: &model.CPUStat{
				LastCpu:   "cpu",
				UserPct:   float32(cpu.UserPct),
				SystemPct: float32(cpu.SystemPct),
				TotalPct:  float32(cpu.UserPct + cpu.SystemPct),
			},
			IoStat: &model.IOStat{},
		})
	}

	return models
}

func TestPercentCalculation(t *testing.T) {
	// Capping at NUM CPU * 100 if we get odd values for delta-{Proc,Time}
	assert.True(t, floatEquals(calculatePct(100, 50, 1), 100))

	// Zero deltaTime case
	assert.True(t, floatEquals(calculatePct(100, 0, 8), 0.0))

	// Negative CPU values should be sanitized to 0
	assert.True(t, floatEquals(calculatePct(100, -200, 1), 0.0))
	assert.True(t, floatEquals(calculatePct(-100, 200, 1), 0.0))

	assert.True(t, floatEquals(calculatePct(0, 8.08, 8), 0.0))
	if runtime.GOOS != "windows" {
		// on *nix systems, CPU utilization is multiplied by number of cores to emulate top
		assert.True(t, floatEquals(calculatePct(100, 200, 2), 100))
		assert.True(t, floatEquals(calculatePct(0.04, 8.08, 8), 3.960396))
		assert.True(t, floatEquals(calculatePct(1.09, 8.08, 8), 107.920792))
	} else {
		// on Windows, CPU utilization is not multiplied by number of cores
		assert.True(t, floatEquals(calculatePct(100, 200, 2), 50))
		assert.True(t, floatEquals(calculatePct(0.04, 8.08, 8), 0.4950495))
		assert.True(t, floatEquals(calculatePct(1.09, 8.08, 8), 13.490099))
	}
}

func TestRateCalculation(t *testing.T) {
	now := time.Now()
	prev := now.Add(-1 * time.Second)
	var empty time.Time
	assert.True(t, floatEquals(calculateRate(5, 1, prev), 4))
	assert.True(t, floatEquals(calculateRate(5, 1, prev.Add(-2*time.Second)), float32(1.33333333)))
	assert.True(t, floatEquals(calculateRate(5, 1, now), 0))
	assert.True(t, floatEquals(calculateRate(5, 0, prev), 0))
	assert.True(t, floatEquals(calculateRate(5, 1, empty), 0))

	// Underflow on cur - prev
	assert.True(t, floatEquals(calculateRate(0, 1, prev), 0))
}

func TestFormatCommand(t *testing.T) {
	process := &procutil.Process{
		Pid:     11,
		Ppid:    1,
		Cmdline: []string{"git", "clone", "google.com"},
		Cwd:     "/home/dog",
		Exe:     "git",
	}
	expected := &model.Command{
		Args: []string{"git", "clone", "google.com"},
		Cwd:  "/home/dog",
		Ppid: 1,
		Exe:  "git",
	}
	assert.Equal(t, expected, formatCommand(process))
}

func TestFormatIO(t *testing.T) {
	fp := &procutil.Stats{
		IOStat: &procutil.IOCountersStat{
			ReadCount:  6,
			WriteCount: 8,
			ReadBytes:  10,
			WriteBytes: 12,
		},
	}

	last := &procutil.IOCountersStat{
		ReadCount:  1,
		WriteCount: 2,
		ReadBytes:  3,
		WriteBytes: 4,
	}

	// fp.IOStat is nil
	assert.NotNil(t, formatIO(&procutil.Stats{}, last, time.Now().Add(-2*time.Second)))

	// IOStats have 0 values
	result := formatIO(&procutil.Stats{IOStat: &procutil.IOCountersStat{}}, last, time.Now().Add(-2*time.Second))
	assert.Equal(t, float32(0), result.ReadRate)
	assert.Equal(t, float32(0), result.WriteRate)
	assert.Equal(t, float32(0), result.ReadBytesRate)
	assert.Equal(t, float32(0), result.WriteBytesRate)

	// Elapsed time < 1s
	assert.NotNil(t, formatIO(fp, last, time.Now()))

	// IOStats have permission problem
	result = formatIO(&procutil.Stats{IOStat: &procutil.IOCountersStat{
		ReadCount:  -1,
		WriteCount: -1,
		ReadBytes:  -1,
		WriteBytes: -1,
	}}, last, time.Now().Add(-1*time.Second))
	assert.Equal(t, float32(-1), result.ReadRate)
	assert.Equal(t, float32(-1), result.WriteRate)
	assert.Equal(t, float32(-1), result.ReadBytesRate)
	assert.Equal(t, float32(-1), result.WriteBytesRate)

	result = formatIO(fp, last, time.Now().Add(-1*time.Second))
	require.NotNil(t, result)
	assert.Equal(t, float32(5), result.ReadRate)
	assert.Equal(t, float32(6), result.WriteRate)
	assert.Equal(t, float32(7), result.ReadBytesRate)
	assert.Equal(t, float32(8), result.WriteBytesRate)
}

func TestFormatIORates(t *testing.T) {
	ioRateStat := &procutil.IOCountersRateStat{
		ReadRate:       10.1,
		WriteRate:      20.2,
		ReadBytesRate:  30.3,
		WriteBytesRate: 40.4,
	}

	expected := &model.IOStat{
		ReadRate:       10.1,
		WriteRate:      20.2,
		ReadBytesRate:  30.3,
		WriteBytesRate: 40.4,
	}

	assert.Equal(t, expected, formatIORates(ioRateStat))
}

func TestFormatMemory(t *testing.T) {
	for name, test := range map[string]struct {
		stats    *procutil.Stats
		expected *model.MemoryStat
	}{
		"basic": {
			stats: &procutil.Stats{
				MemInfo: &procutil.MemoryInfoStat{
					RSS:  101,
					VMS:  202,
					Swap: 303,
				},
			},
			expected: &model.MemoryStat{
				Rss:  101,
				Vms:  202,
				Swap: 303,
			},
		},
		"extended": {
			stats: &procutil.Stats{
				MemInfo: &procutil.MemoryInfoStat{
					RSS:  101,
					VMS:  202,
					Swap: 303,
				},
				MemInfoEx: &procutil.MemoryInfoExStat{
					Shared: 404,
					Text:   505,
					Lib:    606,
					Data:   707,
					Dirty:  808,
				},
			},
			expected: &model.MemoryStat{
				Rss:    101,
				Vms:    202,
				Swap:   303,
				Shared: 404,
				Text:   505,
				Lib:    606,
				Data:   707,
				Dirty:  808,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, formatMemory(test.stats))
		})
	}
}

func TestFormatNetworks(t *testing.T) {
	for _, tc := range []struct {
		connsByPID map[int32][]*model.Connection
		interval   int
		pid        int32
		expected   *model.ProcessNetworks
	}{
		{
			connsByPID: map[int32][]*model.Connection{
				1: yieldConnections(10),
			},
			interval: 2,
			pid:      1,
			expected: &model.ProcessNetworks{ConnectionRate: 5, BytesRate: 150},
		},
		{
			connsByPID: map[int32][]*model.Connection{
				1: yieldConnections(10),
			},
			interval: 10,
			pid:      1,
			expected: &model.ProcessNetworks{ConnectionRate: 1, BytesRate: 30},
		},
		{
			connsByPID: map[int32][]*model.Connection{
				1: yieldConnections(10),
			},
			interval: 20,
			pid:      1,
			expected: &model.ProcessNetworks{ConnectionRate: 0.5, BytesRate: 15},
		},
		{
			connsByPID: nil,
			interval:   20,
			pid:        1,
			expected:   &model.ProcessNetworks{ConnectionRate: 0, BytesRate: 0},
		},
		{
			connsByPID: map[int32][]*model.Connection{
				1: yieldConnections(10),
			},
			interval: 10,
			pid:      2,
			expected: &model.ProcessNetworks{ConnectionRate: 0, BytesRate: 0},
		},
	} {
		result := formatNetworks(tc.connsByPID[tc.pid], tc.interval)
		assert.EqualValues(t, tc.expected, result)
	}
}

func TestFormatCPU(t *testing.T) {
	for name, test := range map[string]struct {
		statsNow   *procutil.Stats
		statsPrev  *procutil.Stats
		timeNow    cpu.TimesStat
		timeBefore cpu.TimesStat
		expected   *model.CPUStat
	}{
		"percent": {
			statsNow: &procutil.Stats{
				CPUPercent: &procutil.CPUPercentStat{
					UserPct:   101.01,
					SystemPct: 202.02,
				},
			},
			expected: &model.CPUStat{
				LastCpu:   "cpu",
				TotalPct:  303.03,
				UserPct:   101.01,
				SystemPct: 202.02,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected,
				formatCPU(test.statsNow, test.statsPrev, test.timeNow, test.timeBefore))
		})
	}
}

func floatEquals(a, b float32) bool {
	var e float32 = 0.00000001 // Difference less than some epsilon
	return a-b < e && b-a < e
}

func yieldConnections(count int) []*model.Connection {
	result := make([]*model.Connection, count)
	for i := 0; i < count; i++ {
		result[i] = &model.Connection{LastBytesReceived: 10, LastBytesSent: 20}
	}
	return result
}
