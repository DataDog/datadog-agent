// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// This file contains e2e tests for the consumer + stats generator based on events
// gatherered with the event collector. They are placed in a separate file as they don't match
// with any specific struct but are rather integration tests for the consumer and stats generator.

//go:build linux_bpf

package gpu

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func injectEventsToConsumer(t testing.TB, consumer *cudaEventConsumer, events *testutil.EventCollection) {
	for _, event := range events.Events {
		switch e := event.(type) {
		case *ebpf.CudaKernelLaunch:
			consumer.getStreamHandler(&e.Header).handleKernelLaunch(e)
		case *ebpf.CudaMemEvent:
			consumer.getStreamHandler(&e.Header).handleMemEvent(e)
		case *ebpf.CudaSync:
			consumer.getStreamHandler(&e.Header).handleSync(e)
		case *ebpf.CudaSetDeviceEvent:
			consumer.handleSetDevice(e)
		default:
			t.Fatalf("unsupported event type %T", event)
		}
	}
}

func TestPytorchBatchedKernels(t *testing.T) {
	cfg := config.New()
	ctx, err := getSystemContext(testutil.GetBasicNvmlMock(), kernel.ProcFSRoot(), testutil.GetWorkloadMetaMock(t), testutil.GetTelemetryMock(t))
	require.NoError(t, err)
	consumer := newCudaEventConsumer(ctx, nil, cfg, testutil.GetTelemetryMock(t))
	require.NotNil(t, consumer)

	events := testutil.GetGPUTestEvents(t, "pytorch_batched_kernels.ndjson")

	// Setup the visibleDevicesCache so that we don't get warnings
	// about missing devices
	executingPID := 24920
	ctx.visibleDevicesCache[executingPID] = []nvml.Device{testutil.GetDeviceMock(0), testutil.GetDeviceMock(1)}

	injectEventsToConsumer(t, consumer, events)

	// Check that the consumer has the expected number of streams
	require.Len(t, consumer.streamHandlers, 1)

	// Check the state of those streams. As there's only one we can just get it
	// by iterating over the map
	var stream *StreamHandler
	for _, s := range consumer.streamHandlers {
		stream = s
	}

	// Let's explain the input data here a bit. It consists of a stream of kernel launches,
	// interseded with some cudaStreamSynchronize calls. After all that there's a longer cudaStreamSynchronize
	// and then a batch of synchronizations.

	require.NotNil(t, stream)
	require.Len(t, stream.kernelSpans, 10)   // There are 10 uninterrupted sequences of kernel launches
	require.Len(t, stream.kernelLaunches, 0) // And we should have no kernel launches in the stream pending

	// Check that the kernel launches don't span the entire interval of events
	startTs := testutil.GetEventHeader(events.Events[0]).Ktime_ns
	endTs := testutil.GetEventHeader(events.Events[len(events.Events)-1]).Ktime_ns
	firstSyncAfterLastKernelLaunchTs := testutil.GetEventHeader(events.Events[866]).Ktime_ns

	firstSpanStart := stream.kernelSpans[0].startKtime
	lastSpanEnd := stream.kernelSpans[len(stream.kernelSpans)-1].endKtime

	require.Equal(t, firstSpanStart, startTs)
	require.Equal(t, lastSpanEnd, firstSyncAfterLastKernelLaunchTs)
	require.Less(t, lastSpanEnd, endTs-60e6) // The last span should end at least 60 milliseconds before the end of the interval

	// Now let's check the stats generator and see the output
	statsGen, _, _ := getStatsGeneratorForTest(t)
	statsGen.streamHandlers = consumer.streamHandlers // Replace the streamHandlers with the ones from the consumer

	// Tell the generator the last generation time is before the start of our first event

	statsGen.lastGenerationKTime = int64(startTs - 1)
	statsGen.currGenerationKTime = int64(startTs - 1)

	// And get the stats for the full interval
	stats := statsGen.getStats(int64(endTs + 1))
	require.NotNil(t, stats)
	require.Len(t, stats.Metrics, 1)

	metrics := stats.Metrics[0]

	require.Equal(t, metrics.Key.PID, uint32(executingPID))
	require.Equal(t, 10, metrics.UtilizationMetrics.UtilizationPercentage)
}
