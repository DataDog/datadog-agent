// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/safenvml/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestStreamKeyUpdatesCorrectlyWhenChangingDevice(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	ctx := getTestSystemContext(t)
	handlers := newStreamCollection(ctx, testutil.GetTelemetryMock(t), config.New())

	pid := uint32(1)
	pidTgid := uint64(pid)<<32 + uint64(pid)

	streamID := uint64(120)
	headerStreamSpecific := gpuebpf.CudaEventHeader{
		Pid_tgid:  pidTgid,
		Stream_id: streamID,
	}

	globalStreamID := uint64(0)
	headerGlobalStream := gpuebpf.CudaEventHeader{
		Pid_tgid:  pidTgid,
		Stream_id: globalStreamID,
	}

	// Configure the visible devices for our process
	ctx.visibleDevicesCache[int(pid)] = nvmltestutil.GetDDNVMLMocksWithIndexes(t, 0, 1)

	stream, err := handlers.getStream(&headerStreamSpecific)
	require.NoError(t, err)
	require.NotNil(t, stream)
	require.Equal(t, pid, stream.metadata.pid)
	require.Equal(t, streamID, stream.metadata.streamID)

	// The stream should have the default selected device, which is 0
	defaultDevice := 0
	require.Equal(t, testutil.GPUUUIDs[defaultDevice], stream.metadata.gpuUUID)

	// Check the same thing happens with the global stream
	globalStream, err := handlers.getStream(&headerGlobalStream)
	require.NoError(t, err)
	require.NotNil(t, globalStream)
	require.Equal(t, pid, globalStream.metadata.pid)
	require.Equal(t, globalStreamID, globalStream.metadata.streamID)

	// Again, this should be on the default device
	require.Equal(t, testutil.GPUUUIDs[defaultDevice], globalStream.metadata.gpuUUID)

	// Last, check that the same applies when retrieving all active streams on the device.
	// we use a dummy stream ID to make sure the function does not rely on it
	dummyHeader := gpuebpf.CudaEventHeader{Pid_tgid: pidTgid, Stream_id: math.MaxUint64}
	streams, err := handlers.getActiveDeviceStreams(&dummyHeader)
	require.NoError(t, err)
	require.Len(t, streams, 2)
	for _, s := range streams {
		require.Equal(t, pid, s.metadata.pid)
		require.True(t, s.metadata.streamID == streamID || s.metadata.streamID == globalStreamID, s.metadata.streamID)
		require.Equal(t, testutil.GPUUUIDs[defaultDevice], s.metadata.gpuUUID)
	}

	// Now we change the device for the specific stream
	selectedDevice := 1
	ctx.selectedDeviceByPIDAndTID[int(pid)] = map[int]int32{int(pid): int32(selectedDevice)}

	// Again, retrieve all streams for the current device. This time we haven't added any stream yet,
	// so we expect only the returned list to be empty
	streams, err = handlers.getActiveDeviceStreams(&dummyHeader)
	require.NoError(t, err)
	require.Len(t, streams, 0)

	// The stream key for the specific stream should not change, as streams are per-device
	// and cannot change devices during its lifetime
	stream, err = handlers.getStream(&headerStreamSpecific)
	require.NoError(t, err)
	require.NotNil(t, stream)
	require.Equal(t, pid, stream.metadata.pid)
	require.Equal(t, streamID, stream.metadata.streamID)
	require.Equal(t, testutil.GPUUUIDs[defaultDevice], stream.metadata.gpuUUID)

	// The global stream should change, as the global stream can change devices
	globalStream, err = handlers.getStream(&headerGlobalStream)
	require.NoError(t, err)
	require.NotNil(t, globalStream)
	require.Equal(t, pid, globalStream.metadata.pid)
	require.Equal(t, globalStreamID, globalStream.metadata.streamID)
	require.Equal(t, testutil.GPUUUIDs[selectedDevice], globalStream.metadata.gpuUUID)

	// The list of all streams should change too, and this time should contain
	// only the global stream on the selected device
	streams, err = handlers.getActiveDeviceStreams(&dummyHeader)
	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.Equal(t, pid, streams[0].metadata.pid)
	require.Equal(t, globalStreamID, streams[0].metadata.streamID)
	require.Equal(t, testutil.GPUUUIDs[selectedDevice], streams[0].metadata.gpuUUID)
}

func TestStreamCollectionCleanRemovesInactiveStreams(t *testing.T) {
	// Set a fake procfs to avoid system interactions
	pid1, pid2 := uint32(1), uint32(2)
	fakeProcFS := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{
			Pid:     pid1,
			Cmdline: "test-process",
			Command: "test-process",
			Exe:     "/usr/bin/test-process",
		},
		{
			Pid:     pid2,
			Cmdline: "test-process",
			Command: "test-process",
			Exe:     "/usr/bin/test-process",
		},
	}, kernel.WithRealUptime(), kernel.WithRealStat())
	kernel.WithFakeProcFS(t, fakeProcFS)

	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	ctx := getTestSystemContext(t)
	cfg := config.New()
	cfg.StreamConfig.Timeout = 1 * time.Second // Set inactivity threshold to 1 second
	handlers := newStreamCollection(ctx, testutil.GetTelemetryMock(t), cfg)

	// Create two streams
	streamID1, streamID2 := uint64(1), uint64(2)

	header1 := &gpuebpf.CudaEventHeader{
		Pid_tgid:  uint64(pid1)<<32 + uint64(pid1),
		Stream_id: streamID1,
	}
	header2 := &gpuebpf.CudaEventHeader{
		Pid_tgid:  uint64(pid2)<<32 + uint64(pid2),
		Stream_id: streamID2,
	}

	// Create both streams
	stream1, err := handlers.getStream(header1)
	require.NoError(t, err)
	require.NotNil(t, stream1)
	stream2, err := handlers.getStream(header2)
	require.NoError(t, err)
	require.NotNil(t, stream2)

	// Add an events to both streams to make them active
	ktimeLaunch1 := uint64(1000)
	launch := &gpuebpf.CudaKernelLaunch{
		Header: gpuebpf.CudaEventHeader{
			Type:      uint32(gpuebpf.CudaEventTypeKernelLaunch),
			Pid_tgid:  header1.Pid_tgid,
			Ktime_ns:  ktimeLaunch1,
			Stream_id: streamID1,
		},
		Kernel_addr:     42,
		Grid_size:       gpuebpf.Dim3{X: 10, Y: 10, Z: 10},
		Block_size:      gpuebpf.Dim3{X: 2, Y: 2, Z: 1},
		Shared_mem_size: 100,
	}
	stream1.handleKernelLaunch(launch)

	// Add an event to stream1 to make it active
	ktimeLaunch2 := uint64(2000)
	launch2 := &gpuebpf.CudaKernelLaunch{
		Header: gpuebpf.CudaEventHeader{
			Type:      uint32(gpuebpf.CudaEventTypeKernelLaunch),
			Pid_tgid:  header1.Pid_tgid,
			Ktime_ns:  ktimeLaunch2,
			Stream_id: streamID1,
		},
		Kernel_addr:     42,
		Grid_size:       gpuebpf.Dim3{X: 10, Y: 10, Z: 10},
		Block_size:      gpuebpf.Dim3{X: 2, Y: 2, Z: 1},
		Shared_mem_size: 100,
	}
	stream2.handleKernelLaunch(launch2)

	// Clean at a time when stream2 should still be active but stream1 should be inactive
	endTime := ktimeLaunch1 + uint64(cfg.StreamConfig.Timeout.Nanoseconds()+1)
	handlers.clean(int64(endTime))

	// Verify stream1 is not present (inactive)
	streamKey1 := streamKey{
		pid:    pid1,
		stream: streamID1,
	}

	// Can't use require.NotContains with sync.Map
	_, ok := handlers.streams.Load(streamKey1)
	require.False(t, ok)

	// Verify stream2 is still present (active)
	streamKey2 := streamKey{
		pid:    pid2,
		stream: streamID2,
	}
	_, ok = handlers.streams.Load(streamKey2)
	require.True(t, ok)
}

func TestStreamCollectionCleanReleasesPoolItems(t *testing.T) {
	// This test verifies that when a stream is cleaned due to inactivity,
	// the enrichedKernelLaunch pool items are properly released back to the pool.
	pid := uint32(1)
	fakeProcFS := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{
			Pid:     pid,
			Cmdline: "test-process",
			Command: "test-process",
			Exe:     "/usr/bin/test-process",
		},
	}, kernel.WithRealUptime(), kernel.WithRealStat())
	kernel.WithFakeProcFS(t, fakeProcFS)

	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))

	telemetryMock := testutil.GetTelemetryMock(t)
	withTelemetryEnabledPools(t, telemetryMock)

	ctx := getTestSystemContext(t)
	cfg := config.New()
	cfg.StreamConfig.Timeout = 1 * time.Second
	handlers := newStreamCollection(ctx, telemetryMock, cfg)

	streamID := uint64(1)
	header := &gpuebpf.CudaEventHeader{
		Pid_tgid:  uint64(pid)<<32 + uint64(pid),
		Stream_id: streamID,
	}

	stream, err := handlers.getStream(header)
	require.NoError(t, err)
	require.NotNil(t, stream)

	// Add kernel launches WITHOUT syncing them
	ktimeLaunch := uint64(1000)
	numLaunches := 5
	for i := 0; i < numLaunches; i++ {
		launch := &gpuebpf.CudaKernelLaunch{
			Header: gpuebpf.CudaEventHeader{
				Type:      uint32(gpuebpf.CudaEventTypeKernelLaunch),
				Pid_tgid:  header.Pid_tgid,
				Ktime_ns:  ktimeLaunch,
				Stream_id: streamID,
			},
			Kernel_addr:     42,
			Grid_size:       gpuebpf.Dim3{X: 10, Y: 10, Z: 10},
			Block_size:      gpuebpf.Dim3{X: 2, Y: 2, Z: 1},
			Shared_mem_size: 100,
		}
		stream.handleKernelLaunch(launch)
		ktimeLaunch++
	}

	// Verify that we have active items in the pool
	stats := getPoolStats(t, telemetryMock, "enrichedKernelLaunch")
	require.Equal(t, numLaunches, stats.active, "should have %d active items before cleanup", numLaunches)
	require.Equal(t, numLaunches, stats.get)
	require.Equal(t, 0, stats.put)

	// Clean at a time when the stream should be inactive (no sync was done)
	endTime := ktimeLaunch + uint64(cfg.StreamConfig.Timeout.Nanoseconds()+1)
	require.True(t, stream.isInactive(int64(endTime), cfg.StreamConfig.Timeout))
	handlers.clean(int64(endTime))

	// Verify stream was removed
	streamKey := streamKey{pid: pid, stream: streamID}
	_, ok := handlers.streams.Load(streamKey)
	require.False(t, ok, "stream should have been removed")

	// Verify that all pool items were released
	stats = getPoolStats(t, telemetryMock, "enrichedKernelLaunch")
	require.Equal(t, 0, stats.active, "all enrichedKernelLaunch items should be released after cleanup")
	require.Equal(t, numLaunches, stats.get)
	require.Equal(t, numLaunches, stats.put, "put count should match get count after cleanup")
}

func TestGetExistingStreamNoAllocs(t *testing.T) {
	res := testing.Benchmark(BenchmarkGetExistingStream)
	require.Zero(t, res.AllocsPerOp())
}

func BenchmarkGetExistingStream(b *testing.B) {
	ddnvml.WithMockNVML(b, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	ctx := getTestSystemContext(b)
	cfg := config.New()
	handlers := newStreamCollection(ctx, testutil.GetTelemetryMock(b), cfg)

	pid := uint32(1)
	pidTgid := uint64(pid)<<32 + uint64(pid)

	run := func(b *testing.B, streamID uint64) {
		header := &gpuebpf.CudaEventHeader{
			Pid_tgid:  pidTgid,
			Stream_id: streamID,
		}
		ctx.visibleDevicesCache[int(pid)] = nvmltestutil.GetDDNVMLMocksWithIndexes(b, 0, 1)

		b.ResetTimer()
		b.ReportAllocs()

		// Retrieve the header to ensure all allocations for a new stream are
		// done here
		handlers.getStream(header)

		for b.Loop() {
			stream, err := handlers.getStream(header)
			if err != nil {
				b.Fatalf("getStream failed: %v", err)
			}
			if stream == nil {
				b.Fatal("getStream returned nil stream")
			}
		}
	}

	b.Run("GlobalStream", func(b *testing.B) {
		run(b, 0)
	})

	b.Run("NonGlobalStream", func(b *testing.B) {
		run(b, 120)
	})
}
