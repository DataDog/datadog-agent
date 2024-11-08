// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func getStatsGeneratorForTest(t *testing.T) (*statsGenerator, map[streamKey]*StreamHandler, int64) {
	sysCtx, err := getSystemContext(testutil.GetBasicNvmlMock(), kernel.ProcFSRoot())
	require.NoError(t, err)
	require.NotNil(t, sysCtx)

	ktime, err := ddebpf.NowNanoseconds()
	require.NoError(t, err)

	streamHandlers := make(map[streamKey]*StreamHandler)
	statsGen := newStatsGenerator(sysCtx, streamHandlers)
	statsGen.lastGenerationKTime = ktime
	statsGen.currGenerationKTime = ktime
	require.NotNil(t, statsGen)

	return statsGen, streamHandlers, ktime
}

func TestGetStatsWithOnlyCurrentStreamData(t *testing.T) {
	statsGen, streamHandlers, ktime := getStatsGeneratorForTest(t)

	startKtime := ktime + int64(1*time.Second)
	pid := uint32(1)
	streamID := uint64(120)
	pidTgid := uint64(pid)<<32 + uint64(pid)
	skeyKern := streamKey{pid: pid, stream: streamID}
	shmemSize := uint64(10)
	streamHandlers[skeyKern] = &StreamHandler{
		processEnded: false,
		kernelLaunches: []enrichedKernelLaunch{
			{
				CudaKernelLaunch: gpuebpf.CudaKernelLaunch{
					Header:          gpuebpf.CudaEventHeader{Ktime_ns: uint64(startKtime), Pid_tgid: pidTgid, Stream_id: streamID},
					Kernel_addr:     0,
					Shared_mem_size: shmemSize,
					Grid_size:       gpuebpf.Dim3{X: 1, Y: 1, Z: 1},
					Block_size:      gpuebpf.Dim3{X: 1, Y: 1, Z: 1},
				},
			},
		},
	}

	allocSize := uint64(10)
	skeyAlloc := streamKey{pid: pid, stream: 0}
	streamHandlers[skeyAlloc] = &StreamHandler{
		processEnded: false,
		memAllocEvents: map[uint64]gpuebpf.CudaMemEvent{
			0: {
				Header: gpuebpf.CudaEventHeader{Ktime_ns: uint64(startKtime), Pid_tgid: pidTgid, Stream_id: streamID},
				Addr:   0,
				Size:   allocSize,
				Type:   gpuebpf.CudaMemAlloc,
			},
		},
	}

	checkDuration := 10 * time.Second
	checkKtime := ktime + int64(checkDuration)
	stats := statsGen.getStats(checkKtime)
	require.NotNil(t, stats)
	require.Contains(t, stats.ProcessStats, pid)

	pidStats := stats.ProcessStats[pid]
	require.Equal(t, allocSize*2, pidStats.Memory.CurrentBytes)
	require.Equal(t, allocSize*2, pidStats.Memory.MaxBytes)

	// defined kernel is using only 1 core for 9 of the 10 seconds
	expectedUtil := 1.0 / testutil.DefaultGpuCores * 0.9
	require.Equal(t, expectedUtil, pidStats.UtilizationPercentage)
}

func TestGetStatsWithOnlyPastStreamData(t *testing.T) {
	statsGen, streamHandlers, ktime := getStatsGeneratorForTest(t)

	startKtime := ktime + int64(1*time.Second)
	endKtime := startKtime + int64(1*time.Second)

	pid := uint32(1)
	streamID := uint64(120)
	skeyKern := streamKey{pid: pid, stream: streamID}
	numThreads := uint64(5)
	streamHandlers[skeyKern] = &StreamHandler{
		processEnded: false,
		kernelSpans: []*kernelSpan{
			{
				startKtime:     uint64(startKtime),
				endKtime:       uint64(endKtime),
				avgThreadCount: numThreads,
				numKernels:     10,
			},
		},
	}

	allocSize := uint64(10)
	skeyAlloc := streamKey{pid: pid, stream: 0}
	streamHandlers[skeyAlloc] = &StreamHandler{
		processEnded: false,
		allocations: []*memoryAllocation{
			{
				startKtime: uint64(startKtime),
				endKtime:   uint64(endKtime),
				size:       allocSize,
				isLeaked:   false,
				allocType:  globalMemAlloc,
			},
		},
	}

	checkDuration := 10 * time.Second
	checkKtime := ktime + int64(checkDuration)
	stats := statsGen.getStats(checkKtime)
	require.NotNil(t, stats)
	require.Contains(t, stats.ProcessStats, pid)

	pidStats := stats.ProcessStats[pid]
	require.Equal(t, uint64(0), pidStats.Memory.CurrentBytes)
	require.Equal(t, allocSize, pidStats.Memory.MaxBytes)

	// numThreads / DefaultGpuCores is the utilization for the
	threadSecondsUsed := float64(numThreads) * float64(endKtime-startKtime) / 1e9
	threadSecondsAvailable := float64(testutil.DefaultGpuCores) * checkDuration.Seconds()
	expectedUtil := threadSecondsUsed / threadSecondsAvailable
	require.InDelta(t, expectedUtil, pidStats.UtilizationPercentage, 0.001)
}

func TestGetStatsWithPastAndCurrentData(t *testing.T) {
	statsGen, streamHandlers, ktime := getStatsGeneratorForTest(t)

	startKtime := ktime + int64(1*time.Second)
	endKtime := startKtime + int64(1*time.Second)

	pid := uint32(1)
	streamID := uint64(120)
	skeyKern := streamKey{pid: pid, stream: streamID}
	pidTgid := uint64(pid)<<32 + uint64(pid)
	numThreads := uint64(5)
	shmemSize := uint64(10)
	streamHandlers[skeyKern] = &StreamHandler{
		processEnded: false,
		kernelLaunches: []enrichedKernelLaunch{
			{
				CudaKernelLaunch: gpuebpf.CudaKernelLaunch{
					Header:          gpuebpf.CudaEventHeader{Ktime_ns: uint64(startKtime), Pid_tgid: pidTgid, Stream_id: streamID},
					Kernel_addr:     0,
					Shared_mem_size: shmemSize,
					Grid_size:       gpuebpf.Dim3{X: 1, Y: 1, Z: 1},
					Block_size:      gpuebpf.Dim3{X: 1, Y: 1, Z: 1},
				},
			},
		},
		kernelSpans: []*kernelSpan{
			{
				startKtime:     uint64(startKtime),
				endKtime:       uint64(endKtime),
				avgThreadCount: numThreads,
				numKernels:     10,
			},
		},
	}

	allocSize := uint64(10)
	skeyAlloc := streamKey{pid: pid, stream: 0}
	streamHandlers[skeyAlloc] = &StreamHandler{
		processEnded: false,
		allocations: []*memoryAllocation{
			{
				startKtime: uint64(startKtime),
				endKtime:   uint64(endKtime),
				size:       allocSize,
				isLeaked:   false,
				allocType:  globalMemAlloc,
			},
		},
		memAllocEvents: map[uint64]gpuebpf.CudaMemEvent{
			0: {
				Header: gpuebpf.CudaEventHeader{Ktime_ns: uint64(startKtime), Pid_tgid: pidTgid, Stream_id: streamID},
				Addr:   0,
				Size:   allocSize,
				Type:   gpuebpf.CudaMemAlloc,
			},
		},
	}

	checkDuration := 10 * time.Second
	checkKtime := ktime + int64(checkDuration)
	stats := statsGen.getStats(checkKtime)
	require.NotNil(t, stats)
	require.Contains(t, stats.ProcessStats, pid)

	pidStats := stats.ProcessStats[pid]
	require.Equal(t, allocSize+shmemSize, pidStats.Memory.CurrentBytes)
	require.Equal(t, allocSize*2+shmemSize, pidStats.Memory.MaxBytes)

	// numThreads / DefaultGpuCores is the utilization for the
	threadSecondsUsed := float64(numThreads) * float64(endKtime-startKtime) / 1e9
	threadSecondsAvailable := float64(testutil.DefaultGpuCores) * checkDuration.Seconds()
	expectedUtilKern1 := threadSecondsUsed / threadSecondsAvailable
	expectedUtilKern2 := 1.0 / testutil.DefaultGpuCores * 0.9
	expectedUtil := expectedUtilKern1 + expectedUtilKern2
	require.InDelta(t, expectedUtil, pidStats.UtilizationPercentage, 0.001)
}
