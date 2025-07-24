// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func getMetricsEntry(key model.StatsKey, stats *model.GPUStats) *model.UtilizationMetrics {
	for _, entry := range stats.Metrics {
		if entry.Key == key {
			return &entry.UtilizationMetrics
		}
	}

	return nil
}

func getStatsGeneratorForTest(t *testing.T) (*statsGenerator, *streamCollection, int64) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	sysCtx := getTestSystemContext(t)

	ktime, err := ddebpf.NowNanoseconds()
	require.NoError(t, err)

	streamHandlers := newStreamCollection(sysCtx, testutil.GetTelemetryMock(t), config.New())
	statsGen := newStatsGenerator(sysCtx, streamHandlers, testutil.GetTelemetryMock(t))
	statsGen.lastGenerationKTime = ktime
	statsGen.currGenerationKTime = ktime
	require.NotNil(t, statsGen)

	return statsGen, streamHandlers, ktime
}

func addStream(t *testing.T, streamHandlers *streamCollection, pid uint32, streamID uint64, gpuUUID string, containerID string) *StreamHandler {
	key := streamKey{pid: pid, stream: streamID}
	metadata := streamMetadata{
		pid:         pid,
		streamID:    streamID,
		containerID: containerID,
		gpuUUID:     gpuUUID,
	}

	stream, err := newStreamHandler(metadata, streamHandlers.sysCtx, getStreamLimits(config.New()), streamHandlers.telemetry)
	require.NoError(t, err)
	streamHandlers.streams[key] = stream

	return stream
}

func addGlobalStream(t *testing.T, streamHandlers *streamCollection, pid uint32, gpuUUID string, containerID string) *StreamHandler {
	streamID := uint64(0)
	key := globalStreamKey{pid: pid, gpuUUID: gpuUUID}
	metadata := streamMetadata{
		pid:         pid,
		streamID:    streamID,
		containerID: containerID,
		gpuUUID:     gpuUUID,
	}

	stream, err := newStreamHandler(metadata, streamHandlers.sysCtx, getStreamLimits(config.New()), streamHandlers.telemetry)
	require.NoError(t, err)
	streamHandlers.globalStreams[key] = stream

	return stream
}

func TestGetStatsWithOnlyCurrentStreamData(t *testing.T) {
	statsGen, streamHandlers, ktime := getStatsGeneratorForTest(t)

	startKtime := ktime + int64(1*time.Second)
	pid := uint32(1)
	streamID := uint64(120)
	pidTgid := uint64(pid)<<32 + uint64(pid)
	shmemSize := uint64(10)
	stream := addStream(t, streamHandlers, pid, streamID, testutil.DefaultGpuUUID, "")
	stream.ended = false
	stream.kernelLaunches = []enrichedKernelLaunch{
		{
			CudaKernelLaunch: gpuebpf.CudaKernelLaunch{
				Header:          gpuebpf.CudaEventHeader{Ktime_ns: uint64(startKtime), Pid_tgid: pidTgid, Stream_id: streamID},
				Kernel_addr:     0,
				Shared_mem_size: shmemSize,
				Grid_size:       gpuebpf.Dim3{X: 1, Y: 1, Z: 1},
				Block_size:      gpuebpf.Dim3{X: 1, Y: 1, Z: 1},
			},
			stream: stream,
		},
	}

	allocSize := uint64(10)
	stream = addGlobalStream(t, streamHandlers, pid, testutil.DefaultGpuUUID, "")
	stream.ended = false
	stream.memAllocEvents.Add(0, gpuebpf.CudaMemEvent{
		Header: gpuebpf.CudaEventHeader{Ktime_ns: uint64(startKtime), Pid_tgid: pidTgid, Stream_id: streamID},
		Addr:   0,
		Size:   allocSize,
		Type:   gpuebpf.CudaMemAlloc,
	})

	checkDuration := 10 * time.Second
	checkKtime := ktime + int64(checkDuration)
	stats, err := statsGen.getStats(checkKtime)
	require.NoError(t, err)
	require.NotNil(t, stats)

	metricsKey := model.StatsKey{PID: pid, DeviceUUID: testutil.DefaultGpuUUID}
	metrics := getMetricsEntry(metricsKey, stats)
	require.NotNil(t, metrics, "did not find metrics for key %+v", metricsKey)
	require.Equal(t, allocSize*2, metrics.Memory.CurrentBytes)
	require.Equal(t, allocSize*2, metrics.Memory.MaxBytes)

	// defined kernel is using only 1 core for 9 of the 10 seconds
	expectedUtil := 1.0 * 9 / 10
	require.Equal(t, expectedUtil, metrics.UsedCores)
}

func TestGetStatsWithOnlyPastStreamData(t *testing.T) {
	statsGen, streamHandlers, ktime := getStatsGeneratorForTest(t)

	startKtime := ktime + int64(1*time.Second)
	endKtime := startKtime + int64(1*time.Second)

	pid := uint32(1)
	streamID := uint64(120)
	numThreads := uint64(5)
	stream := addStream(t, streamHandlers, pid, streamID, testutil.DefaultGpuUUID, "")
	stream.ended = false
	stream.kernelSpans = []*kernelSpan{
		{
			startKtime:     uint64(startKtime),
			endKtime:       uint64(endKtime),
			avgThreadCount: numThreads,
			numKernels:     10,
		},
	}

	allocSize := uint64(10)
	stream = addGlobalStream(t, streamHandlers, pid, testutil.DefaultGpuUUID, "")
	stream.ended = false
	stream.allocations = []*memorySpan{
		{
			startKtime: uint64(startKtime),
			endKtime:   uint64(endKtime),
			size:       allocSize,
			isLeaked:   false,
			allocType:  globalMemAlloc,
		},
	}

	checkDuration := 10 * time.Second
	checkKtime := ktime + int64(checkDuration)
	stats, err := statsGen.getStats(checkKtime)
	require.NoError(t, err)
	require.NotNil(t, stats)

	metricsKey := model.StatsKey{PID: pid, DeviceUUID: testutil.DefaultGpuUUID}
	metrics := getMetricsEntry(metricsKey, stats)
	require.NotNil(t, metrics)
	require.Equal(t, uint64(0), metrics.Memory.CurrentBytes)
	require.Equal(t, allocSize, metrics.Memory.MaxBytes)

	threadSecondsUsed := float64(numThreads) * float64(endKtime-startKtime) / 1e9
	expectedCores := threadSecondsUsed / checkDuration.Seconds()
	require.InDelta(t, expectedCores, metrics.UsedCores, 0.001)
}

func TestGetStatsWithPastAndCurrentData(t *testing.T) {
	statsGen, streamHandlers, ktime := getStatsGeneratorForTest(t)

	startKtime := ktime + int64(1*time.Second)
	endKtime := startKtime + int64(1*time.Second)

	pid := uint32(1)
	streamID := uint64(120)
	pidTgid := uint64(pid)<<32 + uint64(pid)
	numThreads := uint64(5)
	shmemSize := uint64(10)
	stream := addStream(t, streamHandlers, pid, streamID, testutil.DefaultGpuUUID, "")
	stream.ended = false
	stream.kernelLaunches = []enrichedKernelLaunch{
		{
			CudaKernelLaunch: gpuebpf.CudaKernelLaunch{
				Header:          gpuebpf.CudaEventHeader{Ktime_ns: uint64(startKtime), Pid_tgid: pidTgid, Stream_id: streamID},
				Kernel_addr:     0,
				Shared_mem_size: shmemSize,
				Grid_size:       gpuebpf.Dim3{X: 1, Y: 1, Z: 1},
				Block_size:      gpuebpf.Dim3{X: 1, Y: 1, Z: 1},
			},
			stream: stream,
		},
	}

	stream.kernelSpans = []*kernelSpan{
		{
			startKtime:     uint64(startKtime),
			endKtime:       uint64(endKtime),
			avgThreadCount: numThreads,
			numKernels:     10,
		},
	}

	allocSize := uint64(10)
	stream = addGlobalStream(t, streamHandlers, pid, testutil.DefaultGpuUUID, "")
	stream.ended = false
	stream.allocations = []*memorySpan{
		{
			startKtime: uint64(startKtime),
			endKtime:   uint64(endKtime),
			size:       allocSize,
			isLeaked:   false,
			allocType:  globalMemAlloc,
		},
	}

	stream.memAllocEvents.Add(0, gpuebpf.CudaMemEvent{
		Header: gpuebpf.CudaEventHeader{Ktime_ns: uint64(startKtime), Pid_tgid: pidTgid, Stream_id: streamID},
		Addr:   0,
		Size:   allocSize,
		Type:   gpuebpf.CudaMemAlloc,
	})

	checkDuration := 10 * time.Second
	checkKtime := ktime + int64(checkDuration)
	stats, err := statsGen.getStats(checkKtime)
	require.NoError(t, err)
	require.NotNil(t, stats)

	metricsKey := model.StatsKey{PID: pid, DeviceUUID: testutil.DefaultGpuUUID}
	metrics := getMetricsEntry(metricsKey, stats)
	require.NotNil(t, metrics)
	require.Equal(t, allocSize+shmemSize, metrics.Memory.CurrentBytes)
	require.Equal(t, allocSize*2+shmemSize, metrics.Memory.MaxBytes)

	threadSecondsUsed := float64(numThreads) * float64(endKtime-startKtime) / 1e9
	expectedUtilKern1 := threadSecondsUsed / checkDuration.Seconds()
	expectedUtilKern2 := 1.0 * 0.9
	expectedUtil := expectedUtilKern1 + expectedUtilKern2
	require.InDelta(t, expectedUtil, metrics.UsedCores, 0.001)
}

func TestGetStatsMultiGPU(t *testing.T) {
	statsGen, streamHandlers, ktime := getStatsGeneratorForTest(t)

	startKtime := ktime + int64(1*time.Second)
	endKtime := startKtime + int64(1*time.Second)

	pid := uint32(1)
	numThreads := uint64(5)

	// Add kernels for all devices
	for i, uuid := range testutil.GPUUUIDs {
		streamID := uint64(i)
		stream := addStream(t, streamHandlers, pid, streamID, uuid, "")
		stream.ended = false
		stream.kernelSpans = []*kernelSpan{
			{
				startKtime:     uint64(startKtime),
				endKtime:       uint64(endKtime),
				avgThreadCount: numThreads,
				numKernels:     10,
			},
		}
	}

	checkDuration := 10 * time.Second
	checkKtime := ktime + int64(checkDuration)
	stats, err := statsGen.getStats(checkKtime)
	require.NoError(t, err)
	require.NotNil(t, stats)

	// Check the metrics for each device
	for i, uuid := range testutil.GPUUUIDs {
		metricsKey := model.StatsKey{PID: pid, DeviceUUID: uuid}
		metrics := getMetricsEntry(metricsKey, stats)
		require.NotNil(t, metrics, "cannot find metrics for key %+v", metricsKey)

		threadSecondsUsed := float64(numThreads) * float64(endKtime-startKtime) / 1e9
		expectedCores := threadSecondsUsed / checkDuration.Seconds()

		require.InDelta(t, expectedCores, metrics.UsedCores, 0.001, "invalid utilization for device %d (uuid=%s)", i, uuid)
	}
}

func TestCleanupInactiveAggregators(t *testing.T) {
	statsGen, streamHandlers, ktime := getStatsGeneratorForTest(t)

	// Add a stream and get stats to create an aggregator
	pid := uint32(1)
	streamID := uint64(120)
	stream := addStream(t, streamHandlers, pid, streamID, testutil.DefaultGpuUUID, "")
	stream.kernelLaunches = []enrichedKernelLaunch{
		{
			CudaKernelLaunch: gpuebpf.CudaKernelLaunch{
				Header:          gpuebpf.CudaEventHeader{Ktime_ns: uint64(ktime), Pid_tgid: uint64(pid)<<32 + uint64(pid), Stream_id: streamID},
				Kernel_addr:     0,
				Shared_mem_size: 10,
				Grid_size:       gpuebpf.Dim3{X: 1, Y: 1, Z: 1},
				Block_size:      gpuebpf.Dim3{X: 1, Y: 1, Z: 1},
			},
			stream: stream,
		},
	}

	// First getStats call should create an aggregator
	stats, err := statsGen.getStats(ktime + int64(10*time.Second))
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Len(t, statsGen.aggregators, 1)

	// We should not cleanup the aggregator yet, as it is still active
	statsGen.cleanupFinishedAggregators()
	require.Len(t, statsGen.aggregators, 1)

	// If we remove the stream, the aggregator should be marked as inactive in the next getStats call
	streamHandlers.streams = make(map[streamKey]*StreamHandler)
	stats, err = statsGen.getStats(ktime + int64(20*time.Second))
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Len(t, statsGen.aggregators, 1) // no cleanup done yet, here we just mark the aggregator as inactive

	statsGen.cleanupFinishedAggregators()
	require.Len(t, statsGen.aggregators, 0)
}

func TestGetStatsNormalization(t *testing.T) {
	statsGen, streamHandlers, ktime := getStatsGeneratorForTest(t)

	checkDuration := 10 * time.Second
	startKtime := ktime
	endKtime := startKtime + int64(checkDuration)

	// Create two processes that each use 80% of GPU cores and 60% of memory
	pid1 := uint32(1)
	pid2 := uint32(2)
	numThreads := uint64(testutil.DefaultGpuCores * 0.8)
	memSize := uint64(float64(testutil.DefaultTotalMemory) * 0.6)

	// Add kernels and memory allocations for both processes
	for _, pid := range []uint32{pid1, pid2} {
		streamID := uint64(pid)
		stream := addStream(t, streamHandlers, pid, streamID, testutil.DefaultGpuUUID, "")
		stream.ended = false
		stream.kernelSpans = []*kernelSpan{
			{
				startKtime:     uint64(startKtime),
				endKtime:       uint64(endKtime),
				avgThreadCount: numThreads,
				numKernels:     10,
			},
		}

		// Add memory allocations
		globalStream := addGlobalStream(t, streamHandlers, pid, testutil.DefaultGpuUUID, "")
		globalStream.ended = false
		globalStream.allocations = []*memorySpan{
			{
				startKtime: uint64(startKtime),
				endKtime:   uint64(endKtime),
				size:       memSize,
				isLeaked:   false,
				allocType:  globalMemAlloc,
			},
		}
	}

	checkKtime := endKtime + 1
	stats, err := statsGen.getStats(checkKtime)
	require.NoError(t, err)
	require.NotNil(t, stats)

	for _, pid := range []uint32{pid1, pid2} {
		metricsKey := model.StatsKey{PID: pid, DeviceUUID: testutil.DefaultGpuUUID}
		metrics := getMetricsEntry(metricsKey, stats)
		require.NotNil(t, metrics, "cannot find metrics for pid %d", pid)

		require.InDelta(t, testutil.DefaultGpuCores/2, metrics.UsedCores, 0.001, "incorrect utilization for pid %d", pid)
		require.InDelta(t, testutil.DefaultTotalMemory/2, metrics.Memory.MaxBytes, 0.001, "incorrect normalized max memory for pid %d", pid)
	}
}
