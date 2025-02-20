// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestConsumerCanStartAndStop(t *testing.T) {
	handler := ddebpf.NewRingBufferHandler(consumerChannelSize)
	cfg := config.New()
	ctx, err := getSystemContext(testutil.GetBasicNvmlMock(), kernel.ProcFSRoot(), testutil.GetWorkloadMetaMock(t), testutil.GetTelemetryMock(t))
	require.NoError(t, err)
	consumer := newCudaEventConsumer(ctx, handler, cfg, testutil.GetTelemetryMock(t))

	consumer.Start()
	require.Eventually(t, func() bool { return consumer.running.Load() }, 100*time.Millisecond, 10*time.Millisecond)

	consumer.Stop()
	require.Eventually(t, func() bool { return !consumer.running.Load() }, 100*time.Millisecond, 10*time.Millisecond)
}

func TestGetStreamKeyUpdatesCorrectlyWhenChangingDevice(t *testing.T) {
	ctx, err := getSystemContext(testutil.GetBasicNvmlMock(), kernel.ProcFSRoot(), testutil.GetWorkloadMetaMock(t), testutil.GetTelemetryMock(t))
	require.NoError(t, err)

	consumer := newCudaEventConsumer(ctx, nil, nil, testutil.GetTelemetryMock(t))

	pid := uint32(1)
	pidTgid := uint64(pid)<<32 + uint64(pid)

	streamID := uint64(120)
	headerStreamSpecific := gpuebpf.CudaEventHeader{
		Pid_tgid:  pidTgid,
		Stream_id: streamID,
	}

	globalStream := uint64(0)
	headerGlobalStream := gpuebpf.CudaEventHeader{
		Pid_tgid:  pidTgid,
		Stream_id: globalStream,
	}

	// Configure the visible devices for our process
	ctx.visibleDevicesCache[int(pid)] = []nvml.Device{testutil.GetDeviceMock(0), testutil.GetDeviceMock(1)}

	streamKey, err := consumer.getStreamKey(&headerStreamSpecific)
	require.NoError(t, err)
	require.NotNil(t, streamKey)
	require.Equal(t, pid, streamKey.pid)
	require.Equal(t, streamID, streamKey.stream)

	// The stream should have the default selected device, which is 0
	require.Equal(t, testutil.GPUUUIDs[0], streamKey.gpuUUID)

	// Check the same thing happens with the global stream
	globalStreamKey, err := consumer.getStreamKey(&headerGlobalStream)

	require.NoError(t, err)
	require.NotNil(t, globalStreamKey)
	require.Equal(t, pid, globalStreamKey.pid)
	require.Equal(t, globalStream, globalStreamKey.stream)

	// Again, this should be on the default device
	require.Equal(t, testutil.GPUUUIDs[0], globalStreamKey.gpuUUID)

	// Now we trigger a setDevice event on this process
	setDev := gpuebpf.CudaSetDeviceEvent{
		Header: gpuebpf.CudaEventHeader{
			Pid_tgid: pidTgid,
		},
		Device: 1,
	}
	consumer.handleSetDevice(&setDev)

	// The stream key for the specific stream should not change, as streams are per-device
	// and cannot change devices during its lifetime
	streamKey, err = consumer.getStreamKey(&headerStreamSpecific)
	require.NoError(t, err)
	require.NotNil(t, streamKey)
	require.Equal(t, pid, streamKey.pid)
	require.Equal(t, streamID, streamKey.stream)
	require.Equal(t, testutil.GPUUUIDs[0], streamKey.gpuUUID)

	// The global stream should change, as the global stream can change devices
	globalStreamKey, err = consumer.getStreamKey(&headerGlobalStream)
	require.NoError(t, err)
	require.NotNil(t, globalStreamKey)
	require.Equal(t, pid, globalStreamKey.pid)
	require.Equal(t, globalStream, globalStreamKey.stream)
	require.Equal(t, testutil.GPUUUIDs[1], globalStreamKey.gpuUUID)
}
