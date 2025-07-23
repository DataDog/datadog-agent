// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package checks

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	wmimpl "github.com/DataDog/datadog-agent/comp/core/workloadmeta/impl"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	probemocks "github.com/DataDog/datadog-agent/pkg/process/procutil/mocks"
	"github.com/stretchr/testify/assert"
)

func wlmProcessWithCreateTime(pid int32, spaceSeparatedCmdline string, creationTime int64) *wmdef.Process {
	return &wmdef.Process{
		EntityID: wmdef.EntityID{
			ID:   fmt.Sprintf("%d", pid),
			Kind: wmdef.KindProcess,
		},
		Pid:          pid,
		Cmdline:      strings.Split(spaceSeparatedCmdline, " "),
		CreationTime: time.Unix(creationTime, 0),
	}
}

// randRange returns a random number between min and max inclusive [min, max]
func randRange(min, max int) int {
	return rand.IntN(max+1-min) + min
}

// randomUnprivilegedProcessStats returns process stats with reasonable randomized data
func randomProcessStats(createTime int64, withPriviledgedData bool) *procutil.Stats {
	proc := &procutil.Stats{CreateTime: createTime, // 1 second to 1 day ago
		Status:     []string{"U", "D", "R", "S", "T", "W", "Z"}[rand.IntN(7)], // Valid process statuses
		Nice:       int32(randRange(-20, 19)),                                 // -20 (highest priority) to 19 (lowest)
		NumThreads: int32(randRange(1, 500)),                                  // Most processes use <100 threads, upper bound for heavy apps

		CPUPercent: &procutil.CPUPercentStat{
			UserPct:   rand.Float64() * 100, // Simulate 0-100% user CPU usage
			SystemPct: rand.Float64() * 100, // Simulate 0-100% system CPU usage
		},

		CPUTime: &procutil.CPUTimesStat{
			User:      rand.Float64() * 10000, // Seconds spent in user mode
			System:    rand.Float64() * 5000,  // Seconds spent in kernel mode
			Idle:      rand.Float64() * 20000, // Idle time on thread pools or waiting
			Nice:      rand.Float64() * 1000,  // Niceness-adjusted CPU time
			Iowait:    rand.Float64() * 1000,  // Waiting on IO
			Irq:       rand.Float64() * 500,   // Time servicing IRQs
			Softirq:   rand.Float64() * 500,   // Time servicing soft IRQs
			Steal:     rand.Float64() * 100,   // Time stolen by hypervisor
			Guest:     rand.Float64() * 50,    // Guest VM time (if applicable)
			GuestNice: rand.Float64() * 50,    // Guest time with nice value
			Stolen:    rand.Float64() * 10,    // Time stolen from a virtual CPU
			Timestamp: time.Now().Unix(),      // Capture time
		},

		MemInfo: &procutil.MemoryInfoStat{
			RSS:  uint64(randRange(1<<20, 500<<20)), // 1MB–500MB resident memory
			VMS:  uint64(randRange(10<<20, 5<<30)),  // 10MB–5GB virtual memory
			Swap: uint64(randRange(0, 1<<30)),       // 0–1GB swap
		},

		MemInfoEx: &procutil.MemoryInfoExStat{
			RSS:    uint64(randRange(1<<20, 500<<20)), // should be the same as MemInfo.RSS
			VMS:    uint64(randRange(10<<20, 5<<30)),  // should be the same as MemInfo.VMS
			Shared: uint64(randRange(0, 100<<20)),     // Shared memory (e.g. mmap)
			Text:   uint64(randRange(0, 50<<20)),      // Executable code
			Lib:    uint64(randRange(0, 50<<20)),      // Shared libraries
			Data:   uint64(randRange(1<<20, 2<<30)),   // Heap/stack/data
			Dirty:  uint64(randRange(0, 10<<20)),      // Pages waiting to be written
		},

		CtxSwitches: &procutil.NumCtxSwitchesStat{
			Voluntary:   int64(randRange(0, 1_000_000)), // Caused by blocking or waiting
			Involuntary: int64(randRange(0, 500_000)),   // Caused by CPU scheduler preemption
		},
	}
	if withPriviledgedData {
		proc.IOStat = &procutil.IOCountersStat{
			ReadCount:  int64(randRange(0, 1_000_000)), // Number of read syscalls
			WriteCount: int64(randRange(0, 1_000_000)), // Number of write syscalls
			ReadBytes:  int64(randRange(0, 10<<30)),    // Up to 10GB read
			WriteBytes: int64(randRange(0, 10<<30)),    // Up to 10GB written
		}
		// IORateStat is never populated. TODO: we should probably remove it
		//proc.IORateStat = &procutil.IOCountersRateStat{}
		proc.OpenFdCount = int32(randRange(0, 5000)) // 3 minimum (stdin/out/err) to thousands for busy daemons
	}
	return proc
}

func createTestWLMProcessStats(wlmProcs []*wmdef.Process, elevatedPermissions bool) map[int32]*procutil.Stats {
	statsByPid := make(map[int32]*procutil.Stats, len(wlmProcs))
	for _, wlmProc := range wlmProcs {
		statsByPid[wlmProc.Pid] = randomProcessStats(wlmProc.CreationTime.Unix(), elevatedPermissions)
	}
	return statsByPid
}

// TestWLMProcessesByPID tests processesByPID map creation when WLM collection is ON
func TestWLMCollectedProcessesByPIDOn(t *testing.T) {
	for _, tc := range []struct {
		description  string
		collectStats bool
	}{
		{
			description:  "wlm collection ENABLED, with stats ENABLED",
			collectStats: true,
		},
		{
			description:  "wlm collection ENABLED, with stats DISABLED",
			collectStats: false,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			// INITIALIZATION
			mockProbe := probemocks.NewProbe(t)
			mockWLM := wmimpl.NewMockWLM(t)
			mockConstantClock := constantMockClock(time.Now())
			processCheck := &ProcessCheck{
				wmeta:                   mockWLM,
				probe:                   mockProbe,
				useWLMProcessCollection: true,
				clock:                   mockConstantClock,
			}

			// MOCKING
			nowSeconds := mockConstantClock.Now().Unix()
			proc1 := wlmProcessWithCreateTime(1, "git clone google.com", nowSeconds)
			proc2 := wlmProcessWithCreateTime(2, "mine-bitcoins -all -x", nowSeconds-1)
			proc3 := wlmProcessWithCreateTime(3, "datadog-agent --cfgpath datadog.conf", nowSeconds+2)
			proc4 := wlmProcessWithCreateTime(4, "/bin/bash/usr/local/bin/cilium-agent-bpf-map-metrics.sh", nowSeconds-3)
			procs := []*wmdef.Process{proc1, proc2, proc3, proc4}
			statsByPid := make(map[int32]*procutil.Stats)
			mockWLM.EXPECT().ListProcesses().Return(procs).Once()
			if tc.collectStats {
				// elevatedPermissions is irrelevant since we are mocking the probe so no internal logic is tested
				statsByPid = createTestWLMProcessStats([]*wmdef.Process{proc1, proc2, proc3, proc4}, true)
				mockProbe.EXPECT().StatsForPIDs([]int32{proc1.Pid, proc2.Pid, proc3.Pid, proc4.Pid}, mockConstantClock.Now()).Return(statsByPid, nil).Once()
			}

			// EXPECTED
			expected := map[int32]*procutil.Process{
				proc1.Pid: mapWLMProcToProc(proc1, statsByPid[proc1.Pid]),
				proc2.Pid: mapWLMProcToProc(proc2, statsByPid[proc2.Pid]),
				proc3.Pid: mapWLMProcToProc(proc3, statsByPid[proc3.Pid]),
				proc4.Pid: mapWLMProcToProc(proc4, statsByPid[proc4.Pid]),
			}

			// TESTING
			actual, err := processCheck.processesByPID(tc.collectStats)
			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
		})
	}
}

// TestWLMProcessesByPID tests processesByPID normal probe usage when WLM collection is OFF
func TestWLMCollectedProcessesByPIDOff(t *testing.T) {
	for _, tc := range []struct {
		description  string
		collectStats bool
	}{
		{
			description:  "wlm collection DISABLED, with stats ENABLED",
			collectStats: true,
		},
		{
			description:  "wlm collection DISABLED, with stats DISABLED",
			collectStats: false,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			// INITIALIZATION
			mockProbe := probemocks.NewProbe(t)
			mockWLM := wmimpl.NewMockWLM(t)
			mockConstantClock := constantMockClock(time.Now())
			processCheck := &ProcessCheck{
				wmeta:                   mockWLM,
				probe:                   mockProbe,
				useWLMProcessCollection: false,
				clock:                   mockConstantClock,
			}

			// MOCKING
			mockWLM.AssertNotCalled(t, "ListProcesses")
			mockProbe.AssertNotCalled(t, "StatsForPIDs")
			mockProbe.EXPECT().ProcessesByPID(mockConstantClock.Now(), tc.collectStats).Return(nil, nil).Once()

			// TESTING
			_, err := processCheck.processesByPID(tc.collectStats)
			assert.NoError(t, err)
		})
	}
}
