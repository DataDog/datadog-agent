// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/cuda"
	gpuebpf "github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/safenvml/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

type mockFlusher struct {
}

func (m *mockFlusher) Flush() {
}

type mockProcessMonitor struct {
}

func (m *mockProcessMonitor) SubscribeExit(_ func(uint32)) func() {
	return func() {}
}

func (m *mockProcessMonitor) SubscribeExec(_ func(uint32)) func() {
	return func() {}
}

type testConsumerOpts struct {
	eventHandler ddebpf.EventHandler
	telemetry    telemetry.Component
}

type testConsumerOpt func(*testConsumerOpts)

func withEventHandler(handler ddebpf.EventHandler) testConsumerOpt {
	return func(o *testConsumerOpts) {
		o.eventHandler = handler
	}
}

func withTelemetryMock(tm telemetry.Component) testConsumerOpt {
	return func(o *testConsumerOpts) {
		o.telemetry = tm
	}
}

// newTestCudaEventConsumer creates a cudaEventConsumer with test mocks for ProcessMonitor and RingFlusher.
func newTestCudaEventConsumer(t testing.TB, ctx *systemContext, cfg *config.Config, handlers *streamCollection, opts ...testConsumerOpt) *cudaEventConsumer {
	options := &testConsumerOpts{
		telemetry: testutil.GetTelemetryMock(t),
	}
	for _, opt := range opts {
		opt(options)
	}

	return newCudaEventConsumer(cudaEventConsumerDependencies{
		sysCtx:         ctx,
		cfg:            cfg,
		telemetry:      options.telemetry,
		processMonitor: &mockProcessMonitor{},
		streamHandlers: handlers,
		eventHandler:   options.eventHandler,
		ringFlusher:    &mockFlusher{},
	})
}

func TestConsumerCanStartAndStop(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	handler := ddebpf.NewRingBufferHandler(consumerChannelSize)
	cfg := config.New()
	ctx := getTestSystemContext(t, withFatbinParsingEnabled(true))
	streamHandlers := newStreamCollection(ctx, testutil.GetTelemetryMock(t), cfg)
	consumer := newTestCudaEventConsumer(t, ctx, cfg, streamHandlers, withEventHandler(handler))

	consumer.Start()
	require.Eventually(t, func() bool { return consumer.running.Load() }, 100*time.Millisecond, 10*time.Millisecond)

	consumer.Stop()
	require.Eventually(t, func() bool { return !consumer.running.Load() }, 100*time.Millisecond, 10*time.Millisecond)
}

func TestGetStreamKeyUpdatesCorrectlyWhenChangingDevice(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	ctx := getTestSystemContext(t, withFatbinParsingEnabled(true))
	cfg := config.New()
	handlers := newStreamCollection(ctx, testutil.GetTelemetryMock(t), cfg)
	consumer := newTestCudaEventConsumer(t, ctx, cfg, handlers)

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
			ddnvml.WithMockNVML(b, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
			ctx, err := getSystemContext(
				withProcRoot(kernel.ProcFSRoot()),
				withWorkloadMeta(testutil.GetWorkloadMetaMock(b)),
				withTelemetry(testutil.GetTelemetryMock(b)),
				withFatbinParsingEnabled(fatbinParsingEnabled),
			)
			require.NoError(b, err)

			cfg := config.New()
			handlers := newStreamCollection(ctx, testutil.GetTelemetryMock(b), cfg)

			pid := testutil.DataSampleInfos[testutil.DataSamplePytorchBatchedKernels].ActivePID
			ctx.visibleDevicesCache[pid] = nvmltestutil.GetDDNVMLMocksWithIndexes(b, 0, 1)

			if ctx.cudaKernelCache != nil {
				cuda.AddKernelCacheProcMap(ctx.cudaKernelCache, pid, nil)

				// If we don't start the kernel cache, the request channel will be full and we'll run into
				// errors, falsifying the results of the benchmark
				ctx.cudaKernelCache.Start()
				b.Cleanup(ctx.cudaKernelCache.Stop)
			}

			consumer := newTestCudaEventConsumer(b, ctx, cfg, handlers)
			b.ResetTimer()
			injectEventsToConsumer(b, consumer, events, b.N)
		})
	}
}

func TestConsumerProcessExitChannel(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	handler := ddebpf.NewRingBufferHandler(consumerChannelSize)

	// Create fake procfs
	pid := uint32(5001)
	streamID := uint64(1)
	fakeProcFS := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{
			Pid:     pid,
			Cmdline: "test-process",
			Command: "test-process",
			Exe:     "/usr/bin/test-process",
			Maps:    "00400000-00401000 r-xp 00000000 08:01 123456 /usr/bin/test-process",
			Env:     map[string]string{"PATH": "/usr/bin", "HOME": "/home/test"},
		}},
		kernel.WithRealUptime(), // Required for the ktime resolver to work
		kernel.WithRealStat(),
	)

	// Set up the fake procfs
	kernel.WithFakeProcFS(t, fakeProcFS)

	cfg := config.New()
	ctx := getTestSystemContext(t, withFatbinParsingEnabled(true))
	streamHandlers := newStreamCollection(ctx, testutil.GetTelemetryMock(t), cfg)
	consumer := newTestCudaEventConsumer(t, ctx, cfg, streamHandlers, withEventHandler(handler))

	// Start the consumer
	consumer.Start()
	require.Eventually(t, func() bool { return consumer.running.Load() }, 100*time.Millisecond, 10*time.Millisecond)

	// Create a test stream
	header := &gpuebpf.CudaEventHeader{
		Pid_tgid:  uint64(pid)<<32 + uint64(pid),
		Stream_id: streamID,
	}

	stream, err := streamHandlers.getStream(header)
	require.NoError(t, err)
	require.NotNil(t, stream)
	require.False(t, stream.ended)

	// Send process exit event through the channel
	consumer.processExitChannel <- pid

	// Wait for the stream to be marked as ended
	require.Eventually(t, func() bool { return stream.ended }, 100*time.Millisecond, 10*time.Millisecond)

	// Stop the consumer
	consumer.Stop()
	require.Eventually(t, func() bool { return !consumer.running.Load() }, 100*time.Millisecond, 10*time.Millisecond)
}

func TestConsumerProcessExitViaCheckClosedProcesses(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	handler := ddebpf.NewRingBufferHandler(consumerChannelSize)

	// Create fake procfs with a process that we will remove later
	pid := uint32(6001)
	streamID := uint64(1)
	fakeProcFS := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{
			Pid:     pid,
			Cmdline: "test-process",
			Command: "test-process",
			Exe:     "/usr/bin/test-process",
			Maps:    "00400000-00401000 r-xp 00000000 08:01 123456 /usr/bin/test-process",
			Env:     map[string]string{"PATH": "/usr/bin", "HOME": "/home/test"},
		}},
		kernel.WithRealUptime(), // Required for the ktime resolver to work
		kernel.WithRealStat(),
	)

	// Set up the fake procfs
	kernel.WithFakeProcFS(t, fakeProcFS)

	cfg := config.New()
	cfg.ScanProcessesInterval = 100 * time.Millisecond // don't wait too long
	ctx := getTestSystemContext(t, withFatbinParsingEnabled(true))
	streamHandlers := newStreamCollection(ctx, testutil.GetTelemetryMock(t), cfg)
	consumer := newTestCudaEventConsumer(t, ctx, cfg, streamHandlers, withEventHandler(handler))

	// Start the consumer
	consumer.Start()
	require.Eventually(t, func() bool { return consumer.running.Load() }, 100*time.Millisecond, 10*time.Millisecond)

	// Create a stream for the process
	header := &gpuebpf.CudaEventHeader{
		Pid_tgid:  uint64(pid)<<32 + uint64(pid),
		Stream_id: streamID,
	}

	stream, err := streamHandlers.getStream(header)
	require.NoError(t, err)
	require.NotNil(t, stream)
	require.False(t, stream.ended)

	// Remove the process from fake procfs (simulate process exit) by just deleting its folder
	os.RemoveAll(filepath.Join(fakeProcFS, strconv.Itoa(int(pid))))

	// Wait for the background process checker to discover the closed process and send it through
	// the processExitChannel, which will then be handled by the main consumer loop
	require.Eventually(t, func() bool { return stream.ended }, 5*cfg.ScanProcessesInterval, 50*time.Millisecond)

	// Stop the consumer
	consumer.Stop()
	require.Eventually(t, func() bool { return !consumer.running.Load() }, 100*time.Millisecond, 10*time.Millisecond)
}

func TestHandleStreamEventHandlesGetStreamError(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	handler := ddebpf.NewRingBufferHandler(consumerChannelSize)
	cfg := config.New()
	cfg.StreamConfig.MaxActiveStreams = 0 // This will ensure that no streams are created and we will get an error when trying to get the stream
	ctx := getTestSystemContext(t, withFatbinParsingEnabled(true))
	streamHandlers := newStreamCollection(ctx, testutil.GetTelemetryMock(t), cfg)
	consumer := newTestCudaEventConsumer(t, ctx, cfg, streamHandlers, withEventHandler(handler))

	pid := 25
	streamID := uint64(1)

	event := gpuebpf.CudaKernelLaunch{
		Header: gpuebpf.CudaEventHeader{
			Pid_tgid:  uint64(pid)<<32 + uint64(pid),
			Stream_id: streamID,
		},
		Kernel_addr:     uint64(1),
		Shared_mem_size: uint64(1),
		Grid_size:       gpuebpf.Dim3{X: 1, Y: 1, Z: 1},
		Block_size:      gpuebpf.Dim3{X: 1, Y: 1, Z: 1},
	}

	err := consumer.handleStreamEvent(&event.Header, unsafe.Pointer(&event), gpuebpf.SizeofCudaKernelLaunch)
	require.Error(t, err)
}

func TestConsumerHandlesUnknownEventTypes(t *testing.T) {
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	handler := ddebpf.NewRingBufferHandler(consumerChannelSize)
	cfg := config.New()
	ctx := getTestSystemContext(t, withFatbinParsingEnabled(true))
	streamHandlers := newStreamCollection(ctx, testutil.GetTelemetryMock(t), cfg)
	consumer := newTestCudaEventConsumer(t, ctx, cfg, streamHandlers, withEventHandler(handler))

	// Send an unknown event type
	event := gpuebpf.CudaEventHeader{
		Type: uint32(18967123),
	}

	err := consumer.handleEvent(&event, nil, 0)
	require.Error(t, err)
}
