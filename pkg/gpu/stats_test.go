// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package gpu

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func getStatsGeneratorForTest(t *testing.T) (*statsGenerator, map[model.StreamKey]*StreamHandler, int64) {
	sysCtx, err := getSystemContext(testutil.GetBasicNvmlMock())
	require.NoError(t, err)
	require.NotNil(t, sysCtx)

	ktime, err := ddebpf.NowNanoseconds()
	require.NoError(t, err)

	// Align mock time with boot time for consistent time resolution
	streamHandlers := make(map[model.StreamKey]*StreamHandler)
	statsGen := newStatsGenerator(sysCtx, ktime, streamHandlers)
	require.NotNil(t, statsGen)

	return statsGen, streamHandlers, ktime
}

func TestGetStatsWithOnlyCurrentStreamData(t *testing.T) {
	statsGen, streamHandlers, ktime := getStatsGeneratorForTest(t)

	startKtime := ktime + int64(1*time.Second)
	pid := uint32(1)
	streamID := uint64(120)
	pidTgid := uint64(pid)<<32 + uint64(pid)
	skeyKern := model.StreamKey{Pid: pid, Stream: streamID}
	streamHandlers[skeyKern] = &StreamHandler{
		processEnded: false,
		kernelLaunches: []gpuebpf.CudaKernelLaunch{
			{
				Header:          gpuebpf.CudaEventHeader{Ktime_ns: uint64(startKtime), Pid_tgid: pidTgid, Stream_id: streamID},
				Kernel_addr:     0,
				Shared_mem_size: 10,
				Grid_size:       gpuebpf.Dim3{X: 1, Y: 1, Z: 1},
				Block_size:      gpuebpf.Dim3{X: 1, Y: 1, Z: 1},
			},
		},
	}

	allocSize := uint64(10)
	skeyAlloc := model.StreamKey{Pid: pid, Stream: 0}
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
	require.Contains(t, stats.PIDStats, pid)

	pidStats := stats.PIDStats[pid]
	require.Equal(t, allocSize, pidStats.CurrentMemoryBytes)
	require.Equal(t, allocSize, pidStats.MaxMemoryBytes)

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
	skeyKern := model.StreamKey{Pid: pid, Stream: streamID}
	numThreads := uint64(5)
	streamHandlers[skeyKern] = &StreamHandler{
		processEnded: false,
		kernelSpans: []*model.KernelSpan{
			{
				StartKtime:     uint64(startKtime),
				EndKtime:       uint64(endKtime),
				AvgThreadCount: numThreads,
				NumKernels:     10,
			},
		},
	}

	allocSize := uint64(10)
	skeyAlloc := model.StreamKey{Pid: pid, Stream: 0}
	streamHandlers[skeyAlloc] = &StreamHandler{
		processEnded: false,
		allocations: []*model.MemoryAllocation{
			{
				StartKtime: uint64(startKtime),
				EndKtime:   uint64(endKtime),
				Size:       allocSize,
				IsLeaked:   false,
			},
		},
	}

	checkDuration := 10 * time.Second
	checkKtime := ktime + int64(checkDuration)
	stats := statsGen.getStats(checkKtime)
	require.NotNil(t, stats)
	require.Contains(t, stats.PIDStats, pid)

	pidStats := stats.PIDStats[pid]
	require.Equal(t, uint64(0), pidStats.CurrentMemoryBytes)
	require.Equal(t, allocSize, pidStats.MaxMemoryBytes)

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
	skeyKern := model.StreamKey{Pid: pid, Stream: streamID}
	pidTgid := uint64(pid)<<32 + uint64(pid)
	numThreads := uint64(5)
	streamHandlers[skeyKern] = &StreamHandler{
		processEnded: false,
		kernelLaunches: []gpuebpf.CudaKernelLaunch{
			{
				Header:          gpuebpf.CudaEventHeader{Ktime_ns: uint64(startKtime), Pid_tgid: pidTgid, Stream_id: streamID},
				Kernel_addr:     0,
				Shared_mem_size: 10,
				Grid_size:       gpuebpf.Dim3{X: 1, Y: 1, Z: 1},
				Block_size:      gpuebpf.Dim3{X: 1, Y: 1, Z: 1},
			},
		},
		kernelSpans: []*model.KernelSpan{
			{
				StartKtime:     uint64(startKtime),
				EndKtime:       uint64(endKtime),
				AvgThreadCount: numThreads,
				NumKernels:     10,
			},
		},
	}

	allocSize := uint64(10)
	skeyAlloc := model.StreamKey{Pid: pid, Stream: 0}
	streamHandlers[skeyAlloc] = &StreamHandler{
		processEnded: false,
		allocations: []*model.MemoryAllocation{
			{
				StartKtime: uint64(startKtime),
				EndKtime:   uint64(endKtime),
				Size:       allocSize,
				IsLeaked:   false,
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
	require.Contains(t, stats.PIDStats, pid)

	pidStats := stats.PIDStats[pid]
	require.Equal(t, uint64(allocSize), pidStats.CurrentMemoryBytes)
	require.Equal(t, allocSize*2, pidStats.MaxMemoryBytes)

	// numThreads / DefaultGpuCores is the utilization for the
	threadSecondsUsed := float64(numThreads) * float64(endKtime-startKtime) / 1e9
	threadSecondsAvailable := float64(testutil.DefaultGpuCores) * checkDuration.Seconds()
	expectedUtilKern1 := threadSecondsUsed / threadSecondsAvailable
	expectedUtilKern2 := 1.0 / testutil.DefaultGpuCores * 0.9
	expectedUtil := expectedUtilKern1 + expectedUtilKern2
	require.InDelta(t, expectedUtil, pidStats.UtilizationPercentage, 0.001)
}
