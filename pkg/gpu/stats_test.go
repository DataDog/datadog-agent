// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"sync"
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

func getMetricsEntry(key model.ProcessStatsKey, stats *model.GPUStats) *model.UtilizationMetrics {
	for _, entry := range stats.ProcessMetrics {
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

	stream, err := newStreamHandler(metadata, streamHandlers.sysCtx, config.New().StreamConfig, streamHandlers.telemetry)
	require.NoError(t, err)
	streamHandlers.streams.Store(key, stream)

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

	stream, err := newStreamHandler(metadata, streamHandlers.sysCtx, config.New().StreamConfig, streamHandlers.telemetry)
	require.NoError(t, err)
	streamHandlers.globalStreams.Store(key, stream)

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
	stream.kernelLaunches = []*enrichedKernelLaunch{
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

	metricsKey := model.ProcessStatsKey{PID: pid, DeviceUUID: testutil.DefaultGpuUUID}
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
	// Send kernel span to the channel
	stream.pendingKernelSpans <- &kernelSpan{
		startKtime:     uint64(startKtime),
		endKtime:       uint64(endKtime),
		avgThreadCount: numThreads,
		numKernels:     10,
	}

	allocSize := uint64(10)
	stream = addGlobalStream(t, streamHandlers, pid, testutil.DefaultGpuUUID, "")
	stream.ended = false
	// Send allocation to the channel
	stream.pendingMemorySpans <- &memorySpan{
		startKtime: uint64(startKtime),
		endKtime:   uint64(endKtime),
		size:       allocSize,
		isLeaked:   false,
		allocType:  globalMemAlloc,
	}

	checkDuration := 10 * time.Second
	checkKtime := ktime + int64(checkDuration)
	stats, err := statsGen.getStats(checkKtime)
	require.NoError(t, err)
	require.NotNil(t, stats)

	metricsKey := model.ProcessStatsKey{PID: pid, DeviceUUID: testutil.DefaultGpuUUID}
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
	stream.kernelLaunches = []*enrichedKernelLaunch{
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

	// Send kernel span to the channel
	stream.pendingKernelSpans <- &kernelSpan{
		startKtime:     uint64(startKtime),
		endKtime:       uint64(endKtime),
		avgThreadCount: numThreads,
		numKernels:     10,
	}

	allocSize := uint64(10)
	stream = addGlobalStream(t, streamHandlers, pid, testutil.DefaultGpuUUID, "")
	stream.ended = false
	// Send allocation to the channel
	stream.pendingMemorySpans <- &memorySpan{
		startKtime: uint64(startKtime),
		endKtime:   uint64(endKtime),
		size:       allocSize,
		isLeaked:   false,
		allocType:  globalMemAlloc,
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

	metricsKey := model.ProcessStatsKey{PID: pid, DeviceUUID: testutil.DefaultGpuUUID}
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
		// Send kernel span to the channel
		stream.pendingKernelSpans <- &kernelSpan{
			startKtime:     uint64(startKtime),
			endKtime:       uint64(endKtime),
			avgThreadCount: numThreads,
			numKernels:     10,
		}
	}

	checkDuration := 10 * time.Second
	checkKtime := ktime + int64(checkDuration)
	stats, err := statsGen.getStats(checkKtime)
	require.NoError(t, err)
	require.NotNil(t, stats)

	// Check the metrics for each device
	for i, uuid := range testutil.GPUUUIDs {
		metricsKey := model.ProcessStatsKey{PID: pid, DeviceUUID: uuid}
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
	stream.kernelLaunches = []*enrichedKernelLaunch{
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
	streamHandlers.streams = sync.Map{}
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
	numThreads := uint64(testutil.DefaultGpuCores * 8 / 10)
	memSize := uint64(testutil.DefaultTotalMemory * 6 / 10)

	// Add kernels and memory allocations for both processes
	for _, pid := range []uint32{pid1, pid2} {
		streamID := uint64(pid)
		stream := addStream(t, streamHandlers, pid, streamID, testutil.DefaultGpuUUID, "")
		stream.ended = false
		// Send kernel span to the channel
		stream.pendingKernelSpans <- &kernelSpan{
			startKtime:     uint64(startKtime),
			endKtime:       uint64(endKtime),
			avgThreadCount: numThreads,
			numKernels:     10,
		}

		// Add memory allocations
		globalStream := addGlobalStream(t, streamHandlers, pid, testutil.DefaultGpuUUID, "")
		globalStream.ended = false
		// Send allocation to the channel
		globalStream.pendingMemorySpans <- &memorySpan{
			startKtime: uint64(startKtime),
			endKtime:   uint64(endKtime),
			size:       memSize,
			isLeaked:   false,
			allocType:  globalMemAlloc,
		}
	}

	checkKtime := endKtime + 1
	stats, err := statsGen.getStats(checkKtime)
	require.NoError(t, err)
	require.NotNil(t, stats)

	for _, pid := range []uint32{pid1, pid2} {
		metricsKey := model.ProcessStatsKey{PID: pid, DeviceUUID: testutil.DefaultGpuUUID}
		metrics := getMetricsEntry(metricsKey, stats)
		require.NotNil(t, metrics, "cannot find metrics for pid %d", pid)

		require.InDelta(t, testutil.DefaultGpuCores/2, metrics.UsedCores, 0.001, "incorrect utilization for pid %d", pid)
		require.InDelta(t, testutil.DefaultTotalMemory/2, metrics.Memory.MaxBytes, 0.001, "incorrect normalized max memory for pid %d", pid)
	}
}

func TestGetStatsActiveTimePct(t *testing.T) {
	statsGen, streamHandlers, ktime := getStatsGeneratorForTest(t)

	checkDuration := 10 * time.Second
	startKtime := ktime + int64(1*time.Second)
	endKtime := startKtime + int64(5*time.Second) // 5 seconds of activity out of 10 = 50%

	pid := uint32(1)
	streamID := uint64(120)
	stream := addStream(t, streamHandlers, pid, streamID, testutil.DefaultGpuUUID, "")
	stream.ended = false

	// Send kernel span to the channel
	stream.pendingKernelSpans <- &kernelSpan{
		startKtime:     uint64(startKtime),
		endKtime:       uint64(endKtime),
		avgThreadCount: 100,
		numKernels:     1,
	}

	checkKtime := ktime + int64(checkDuration)
	stats, err := statsGen.getStats(checkKtime)
	require.NoError(t, err)
	require.NotNil(t, stats)

	// Check per-process ActiveTimePct
	metricsKey := model.ProcessStatsKey{PID: pid, DeviceUUID: testutil.DefaultGpuUUID}
	metrics := getMetricsEntry(metricsKey, stats)
	require.NotNil(t, metrics)
	expectedActivePct := 50.0 // 5 seconds active out of 10 seconds
	require.InDelta(t, expectedActivePct, metrics.ActiveTimePct, 0.1, "incorrect ActiveTimePct")

	// Check device-level ActiveTimePct
	require.Len(t, stats.DeviceMetrics, 1)
	require.Equal(t, testutil.DefaultGpuUUID, stats.DeviceMetrics[0].DeviceUUID)
	require.InDelta(t, expectedActivePct, stats.DeviceMetrics[0].Metrics.ActiveTimePct, 0.1, "incorrect device-level ActiveTimePct")
}

func TestGetStatsActiveTimePctWithOverlappingSpans(t *testing.T) {
	statsGen, streamHandlers, ktime := getStatsGeneratorForTest(t)

	checkDuration := 10 * time.Second
	// Two processes with overlapping kernel execution
	// Process 1: runs from 1s to 6s (5 seconds)
	// Process 2: runs from 3s to 8s (5 seconds)
	// Overlap: 3s to 6s (3 seconds)
	// Total device active time: 1s to 8s (7 seconds) = 70%

	pid1 := uint32(1)
	pid2 := uint32(2)

	stream1 := addStream(t, streamHandlers, pid1, uint64(1), testutil.DefaultGpuUUID, "")
	stream1.ended = false
	stream1.pendingKernelSpans <- &kernelSpan{
		startKtime:     uint64(ktime + int64(1*time.Second)),
		endKtime:       uint64(ktime + int64(6*time.Second)),
		avgThreadCount: 100,
		numKernels:     1,
	}

	stream2 := addStream(t, streamHandlers, pid2, uint64(2), testutil.DefaultGpuUUID, "")
	stream2.ended = false
	stream2.pendingKernelSpans <- &kernelSpan{
		startKtime:     uint64(ktime + int64(3*time.Second)),
		endKtime:       uint64(ktime + int64(8*time.Second)),
		avgThreadCount: 100,
		numKernels:     1,
	}

	checkKtime := ktime + int64(checkDuration)
	stats, err := statsGen.getStats(checkKtime)
	require.NoError(t, err)
	require.NotNil(t, stats)

	// Check per-process ActiveTimePct
	metrics1 := getMetricsEntry(model.ProcessStatsKey{PID: pid1, DeviceUUID: testutil.DefaultGpuUUID}, stats)
	require.NotNil(t, metrics1)
	require.InDelta(t, 50.0, metrics1.ActiveTimePct, 0.1, "incorrect ActiveTimePct for pid1")

	metrics2 := getMetricsEntry(model.ProcessStatsKey{PID: pid2, DeviceUUID: testutil.DefaultGpuUUID}, stats)
	require.NotNil(t, metrics2)
	require.InDelta(t, 50.0, metrics2.ActiveTimePct, 0.1, "incorrect ActiveTimePct for pid2")

	// Check device-level ActiveTimePct (should merge overlapping intervals)
	require.Len(t, stats.DeviceMetrics, 1)
	require.Equal(t, testutil.DefaultGpuUUID, stats.DeviceMetrics[0].DeviceUUID)
	require.InDelta(t, 70.0, stats.DeviceMetrics[0].Metrics.ActiveTimePct, 0.1, "incorrect device-level ActiveTimePct with overlapping spans")
}

func TestGetStatsWithNoActivity(t *testing.T) {
	statsGen, _, ktime := getStatsGeneratorForTest(t)

	// No streams added, should return empty stats
	checkKtime := ktime + int64(10*time.Second)
	stats, err := statsGen.getStats(checkKtime)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Empty(t, stats.ProcessMetrics)
	require.Empty(t, stats.DeviceMetrics)
}

func TestGetStatsWithContainerID(t *testing.T) {
	statsGen, streamHandlers, ktime := getStatsGeneratorForTest(t)

	pid := uint32(1)
	containerID := "container-abc123"
	streamID := uint64(120)

	stream := addStream(t, streamHandlers, pid, streamID, testutil.DefaultGpuUUID, containerID)
	stream.ended = false
	stream.pendingKernelSpans <- &kernelSpan{
		startKtime:     uint64(ktime + int64(1*time.Second)),
		endKtime:       uint64(ktime + int64(5*time.Second)),
		avgThreadCount: 100,
		numKernels:     1,
	}

	checkKtime := ktime + int64(10*time.Second)
	stats, err := statsGen.getStats(checkKtime)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Len(t, stats.ProcessMetrics, 1)

	// Verify the container ID is included in the stats key
	metricsKey := model.ProcessStatsKey{PID: pid, DeviceUUID: testutil.DefaultGpuUUID, ContainerID: containerID}
	metrics := getMetricsEntry(metricsKey, stats)
	require.NotNil(t, metrics, "metrics should include container ID in key")
}

func TestGetStatsMultiDeviceActiveTime(t *testing.T) {
	statsGen, streamHandlers, ktime := getStatsGeneratorForTest(t)

	checkDuration := 10 * time.Second
	pid := uint32(1)

	// Use first 3 devices only to keep test simple and predictable
	devicesToTest := testutil.GPUUUIDs[:3]

	// Add kernels for multiple devices with different active times
	// Device 0: 2s active (20%), Device 1: 4s (40%), Device 2: 6s (60%)
	for i, uuid := range devicesToTest {
		streamID := uint64(i)
		stream := addStream(t, streamHandlers, pid, streamID, uuid, "")
		stream.ended = false
		// Each device has different active time starting from ktime
		activeTime := time.Duration((i + 1) * 2)
		stream.pendingKernelSpans <- &kernelSpan{
			startKtime:     uint64(ktime),
			endKtime:       uint64(ktime + int64(activeTime*time.Second)),
			avgThreadCount: 100,
			numKernels:     1,
		}
	}

	checkKtime := ktime + int64(checkDuration)
	stats, err := statsGen.getStats(checkKtime)
	require.NoError(t, err)
	require.NotNil(t, stats)

	// Check device-level ActiveTimePct for each device
	require.Len(t, stats.DeviceMetrics, len(devicesToTest))

	deviceMetricsMap := make(map[string]float64)
	for _, dm := range stats.DeviceMetrics {
		deviceMetricsMap[dm.DeviceUUID] = dm.Metrics.ActiveTimePct
	}

	for i, uuid := range devicesToTest {
		activeSeconds := float64((i + 1) * 2)
		expectedPct := (activeSeconds / 10.0) * 100.0
		require.InDelta(t, expectedPct, deviceMetricsMap[uuid], 0.1, "incorrect ActiveTimePct for device %s", uuid)
	}
}

func TestCollectIntervalsClampsBoundaries(t *testing.T) {
	statsGen, _, ktime := getStatsGeneratorForTest(t)

	statsGen.lastGenerationKTime = ktime
	statsGen.currGenerationKTime = ktime + int64(10*time.Second)
	statsGen.deviceIntervals = make(map[string][][2]uint64)

	deviceUUID := testutil.DefaultGpuUUID
	nowKtime := statsGen.currGenerationKTime

	t.Run("span within window is unchanged", func(t *testing.T) {
		statsGen.deviceIntervals = make(map[string][][2]uint64)
		spans := []*kernelSpan{
			{startKtime: uint64(ktime + int64(2*time.Second)), endKtime: uint64(ktime + int64(5*time.Second))},
		}
		statsGen.collectIntervals(spans, deviceUUID, nowKtime)

		require.Len(t, statsGen.deviceIntervals[deviceUUID], 1)
		require.Equal(t, uint64(ktime+int64(2*time.Second)), statsGen.deviceIntervals[deviceUUID][0][0])
		require.Equal(t, uint64(ktime+int64(5*time.Second)), statsGen.deviceIntervals[deviceUUID][0][1])
	})

	t.Run("span start before window is clamped", func(t *testing.T) {
		statsGen.deviceIntervals = make(map[string][][2]uint64)
		spans := []*kernelSpan{
			{startKtime: uint64(ktime - int64(5*time.Second)), endKtime: uint64(ktime + int64(5*time.Second))},
		}
		statsGen.collectIntervals(spans, deviceUUID, nowKtime)

		require.Len(t, statsGen.deviceIntervals[deviceUUID], 1)
		require.Equal(t, uint64(ktime), statsGen.deviceIntervals[deviceUUID][0][0], "start should be clamped")
		require.Equal(t, uint64(ktime+int64(5*time.Second)), statsGen.deviceIntervals[deviceUUID][0][1])
	})

	t.Run("span end after window is clamped", func(t *testing.T) {
		statsGen.deviceIntervals = make(map[string][][2]uint64)
		spans := []*kernelSpan{
			{startKtime: uint64(ktime + int64(5*time.Second)), endKtime: uint64(ktime + int64(15*time.Second))},
		}
		statsGen.collectIntervals(spans, deviceUUID, nowKtime)

		require.Len(t, statsGen.deviceIntervals[deviceUUID], 1)
		require.Equal(t, uint64(ktime+int64(5*time.Second)), statsGen.deviceIntervals[deviceUUID][0][0])
		require.Equal(t, uint64(nowKtime), statsGen.deviceIntervals[deviceUUID][0][1], "end should be clamped")
	})

	t.Run("span completely outside window is excluded", func(t *testing.T) {
		statsGen.deviceIntervals = make(map[string][][2]uint64)
		spans := []*kernelSpan{
			{startKtime: uint64(ktime - int64(10*time.Second)), endKtime: uint64(ktime - int64(5*time.Second))},
		}
		statsGen.collectIntervals(spans, deviceUUID, nowKtime)

		// After clamping, start >= end, so interval is excluded
		require.Empty(t, statsGen.deviceIntervals[deviceUUID])
	})
}

func TestGetStatsReturnsErrorForZeroInterval(t *testing.T) {
	statsGen, streamHandlers, ktime := getStatsGeneratorForTest(t)

	// Add a stream so we have aggregators
	pid := uint32(1)
	stream := addStream(t, streamHandlers, pid, 1, testutil.DefaultGpuUUID, "")
	stream.ended = false
	stream.pendingKernelSpans <- &kernelSpan{
		startKtime:     uint64(ktime),
		endKtime:       uint64(ktime + int64(1*time.Second)),
		avgThreadCount: 100,
		numKernels:     1,
	}

	// Call getStats with the same ktime (zero interval)
	stats, err := statsGen.getStats(ktime)
	require.Error(t, err, "should return error for zero interval")
	require.Nil(t, stats)
	require.Contains(t, err.Error(), "intervalNs is less than or equal to 0")
}

func TestGetStatsDeviceIntervalsResetBetweenCalls(t *testing.T) {
	statsGen, streamHandlers, ktime := getStatsGeneratorForTest(t)

	pid := uint32(1)
	stream := addStream(t, streamHandlers, pid, 1, testutil.DefaultGpuUUID, "")
	stream.ended = false

	// First call with some activity
	stream.pendingKernelSpans <- &kernelSpan{
		startKtime:     uint64(ktime + int64(1*time.Second)),
		endKtime:       uint64(ktime + int64(5*time.Second)),
		avgThreadCount: 100,
		numKernels:     1,
	}

	checkKtime1 := ktime + int64(10*time.Second)
	stats1, err := statsGen.getStats(checkKtime1)
	require.NoError(t, err)
	require.Len(t, stats1.DeviceMetrics, 1)
	require.InDelta(t, 40.0, stats1.DeviceMetrics[0].Metrics.ActiveTimePct, 0.1)

	// Second call without new activity - device metrics should be empty
	checkKtime2 := checkKtime1 + int64(10*time.Second)
	stats2, err := statsGen.getStats(checkKtime2)
	require.NoError(t, err)
	// No new kernel spans, so device intervals should be empty for this period
	require.Empty(t, stats2.DeviceMetrics, "device metrics should be empty when no new activity")
}

func TestGetStatsActiveTimePctCapsAt100(t *testing.T) {
	statsGen, streamHandlers, ktime := getStatsGeneratorForTest(t)

	pid := uint32(1)
	stream := addStream(t, streamHandlers, pid, 1, testutil.DefaultGpuUUID, "")
	stream.ended = false

	// Span that extends beyond the check interval (simulating edge case)
	stream.pendingKernelSpans <- &kernelSpan{
		startKtime:     uint64(ktime - int64(5*time.Second)),  // Started before interval
		endKtime:       uint64(ktime + int64(15*time.Second)), // Ends after interval
		avgThreadCount: 100,
		numKernels:     1,
	}

	checkKtime := ktime + int64(10*time.Second)
	stats, err := statsGen.getStats(checkKtime)
	require.NoError(t, err)
	require.NotNil(t, stats)

	// Device-level ActiveTimePct should be capped at 100%
	require.Len(t, stats.DeviceMetrics, 1)
	require.LessOrEqual(t, stats.DeviceMetrics[0].Metrics.ActiveTimePct, 100.0, "ActiveTimePct should be capped at 100")
	require.InDelta(t, 100.0, stats.DeviceMetrics[0].Metrics.ActiveTimePct, 0.1)
}
