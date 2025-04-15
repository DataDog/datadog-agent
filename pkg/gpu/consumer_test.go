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

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/nvml"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/cuda"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/nvml/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestConsumerCanStartAndStop(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMock())
	handler := ddebpf.NewRingBufferHandler(consumerChannelSize)
	cfg := config.New()
	ctx := getTestSystemContext(t, withFatbinParsingEnabled(true))
	streamHandlers := newStreamCollection(ctx, testutil.GetTelemetryMock(t), cfg)
	consumer := newCudaEventConsumer(ctx, streamHandlers, handler, cfg, testutil.GetTelemetryMock(t))

	consumer.Start()
	require.Eventually(t, func() bool { return consumer.running.Load() }, 100*time.Millisecond, 10*time.Millisecond)

	consumer.Stop()
	require.Eventually(t, func() bool { return !consumer.running.Load() }, 100*time.Millisecond, 10*time.Millisecond)
}

func TestGetStreamKeyUpdatesCorrectlyWhenChangingDevice(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMock())
	ctx := getTestSystemContext(t, withFatbinParsingEnabled(true))
	handlers := newStreamCollection(ctx, testutil.GetTelemetryMock(t), config.New())
	consumer := newCudaEventConsumer(ctx, handlers, nil, nil, testutil.GetTelemetryMock(t))

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
	require.Equal(t, testutil.GPUUUIDs[0], stream.metadata.gpuUUID)

	// Check the same thing happens with the global stream
	globalStream, err := handlers.getStream(&headerGlobalStream)

	require.NoError(t, err)
	require.NotNil(t, globalStream)
	require.Equal(t, pid, globalStream.metadata.pid)
	require.Equal(t, globalStreamID, globalStream.metadata.streamID)

	// Again, this should be on the default device
	require.Equal(t, testutil.GPUUUIDs[0], globalStream.metadata.gpuUUID)

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
	stream, err = handlers.getStream(&headerStreamSpecific)
	require.NoError(t, err)
	require.NotNil(t, stream)
	require.Equal(t, pid, stream.metadata.pid)
	require.Equal(t, streamID, stream.metadata.streamID)
	require.Equal(t, testutil.GPUUUIDs[0], stream.metadata.gpuUUID)

	// The global stream should change, as the global stream can change devices
	globalStream, err = handlers.getStream(&headerGlobalStream)
	require.NoError(t, err)
	require.NotNil(t, globalStream)
	require.Equal(t, pid, globalStream.metadata.pid)
	require.Equal(t, globalStreamID, globalStream.metadata.streamID)
	require.Equal(t, testutil.GPUUUIDs[1], globalStream.metadata.gpuUUID)
}

// BenchmarkConsumer benchmarks the consumer with a data sample, with and without fatbin parsing enabled
// Note that the NVML library is mocked here, so if some of the API calls are slow in the real implementation
// the results will not reflect that. This benchmark is useful to measure the performance of the event intake,
// such as the event parsing, stream handling, the effect of the fatbin parsing and the related caches, etc
func BenchmarkConsumer(b *testing.B) {
	events := testutil.GetGPUTestEvents(b, testutil.DataSamplePytorchBatchedKernels)
	for _, fatbinParsingEnabled := range []bool{true, false} {
		name := "fatbinParsingDisabled"
		if fatbinParsingEnabled {
			name = "fatbinParsingEnabled"
		}
		b.Run(name, func(b *testing.B) {
			ddnvml.WithMockNVML(b, testutil.GetBasicNvmlMock())
			ctx, err := getSystemContext(
				withProcRoot(kernel.ProcFSRoot()),
				withWorkloadMeta(testutil.GetWorkloadMetaMock(b)),
				withTelemetry(testutil.GetTelemetryMock(b)),
				withFatbinParsingEnabled(fatbinParsingEnabled),
			)
			require.NoError(b, err)
			handlers := newStreamCollection(ctx, testutil.GetTelemetryMock(b), config.New())

			cfg := config.New()
			pid := testutil.DataSampleInfos[testutil.DataSamplePytorchBatchedKernels].ActivePID
			ctx.visibleDevicesCache[pid] = nvmltestutil.GetDDNVMLMocksWithIndexes(b, 0, 1)

			if ctx.cudaKernelCache != nil {
				cuda.AddKernelCacheProcMap(ctx.cudaKernelCache, pid, nil)

				// If we don't start the kernel cache, the request channel will be full and we'll run into
				// errors, falsifying the results of the benchmark
				ctx.cudaKernelCache.Start()
				b.Cleanup(ctx.cudaKernelCache.Stop)
			}

			consumer := newCudaEventConsumer(ctx, handlers, nil, cfg, testutil.GetTelemetryMock(b))
			b.ResetTimer()
			injectEventsToConsumer(b, consumer, events, b.N)
		})
	}
}
