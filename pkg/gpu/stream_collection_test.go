// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/safenvml/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
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

	// Now we change the device for the specific stream
	selectedDevice := 1
	ctx.selectedDeviceByPIDAndTID[int(pid)] = map[int]int32{int(pid): int32(selectedDevice)}

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
}

func TestStreamCollectionCleanRemovesInactiveStreams(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	ctx := getTestSystemContext(t)
	cfg := config.New()
	cfg.MaxStreamInactivity = 1 * time.Second // Set inactivity threshold to 1 second
	handlers := newStreamCollection(ctx, testutil.GetTelemetryMock(t), cfg)

	// Create two streams
	pid1, pid2 := uint32(1), uint32(2)
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
	endTime := ktimeLaunch1 + uint64(cfg.MaxStreamInactivity.Nanoseconds()+1)
	handlers.clean(int64(endTime))

	// Verify stream1 is not present (inactive)
	streamKey1 := streamKey{
		pid:    pid1,
		stream: streamID1,
	}
	require.NotContains(t, handlers.streams, streamKey1)

	// Verify stream2 is still present (active)
	streamKey2 := streamKey{
		pid:    pid2,
		stream: streamID2,
	}
	require.Contains(t, handlers.streams, streamKey2)
}
