// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"testing"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/nvml"
	"github.com/stretchr/testify/require"

	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/nvml/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestStreamKeyUpdatesCorrectlyWhenChangingDevice(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMock())
	ctx, err := getSystemContext(kernel.ProcFSRoot(), testutil.GetWorkloadMetaMock(t), testutil.GetTelemetryMock(t))
	require.NoError(t, err)

	handlers := newStreamCollection(ctx, testutil.GetTelemetryMock(t))

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
