// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// This file contains e2e tests for the consumer + stats generator based on events
// gatherered with the event collector. They are placed in a separate file as they don't match
// with any specific struct but are rather integration tests for the consumer and stats generator.

//go:build linux_bpf && nvml

package gpu

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/ebpf"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/safenvml/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// injectEventsToConsumer injects events to the consumer. If count is 0, all events will be injected. The count can be higher
// than the number of events in the collection, in which case the events will be injected multiple times.
func injectEventsToConsumer(tb testing.TB, consumer *cudaEventConsumer, events *testutil.EventCollection, count int) {
	if count == 0 {
		count = len(events.Events)
	}

	for count > 0 {
		for _, event := range events.Events {
			err := consumer.handleEvent(&event.Header, event.Pointer, event.DataLength)
			require.NoError(tb, err)
			count--
			if count == 0 {
				return
			}
		}
	}
}

func TestPytorchBatchedKernels(t *testing.T) {
	cfg := config.New()
	telemetryMock := testutil.GetTelemetryMock(t)
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled()))
	ctx, err := getSystemContext(
		withProcRoot(kernel.ProcFSRoot()),
		withWorkloadMeta(testutil.GetWorkloadMetaMock(t)),
		withTelemetry(telemetryMock),
	)
	require.NoError(t, err)

	handlers := newStreamCollection(ctx, telemetryMock, cfg)
	consumer := newCudaEventConsumer(ctx, handlers, nil, cfg, telemetryMock)
	require.NotNil(t, consumer)

	events := testutil.GetGPUTestEvents(t, testutil.DataSamplePytorchBatchedKernels)

	// Setup the visibleDevicesCache so that we don't get warnings
	// about missing devices
	executingPID := testutil.DataSampleInfos[testutil.DataSamplePytorchBatchedKernels].ActivePID
	ctx.visibleDevicesCache[executingPID] = nvmltestutil.GetDDNVMLMocksWithIndexes(t, 0, 1)

	injectEventsToConsumer(t, consumer, events, 0)

	// Check that the consumer has the expected number of streams
	require.Equal(t, 1, handlers.streamCount())

	telemetryMetrics, err := telemetryMock.GetCountMetric("gpu__consumer", "events")
	require.NoError(t, err)
	require.Equal(t, int(ebpf.CudaEventTypeCount), len(telemetryMetrics)) // one for each event type
	expectedEventsByType := testutil.DataSampleInfos[testutil.DataSamplePytorchBatchedKernels].EventByType
	for _, metric := range telemetryMetrics {
		eventTypeTag := metric.Tags()["event_type"]
		require.NotEmpty(t, eventTypeTag)
		require.Equal(t, expectedEventsByType[eventTypeTag], int(metric.Value()))
	}

	// Check the state of those streams. As there's only one we can just get it
	// by iterating over the map
	var stream *StreamHandler
	for s := range handlers.allStreams() {
		require.Nil(t, stream) // There should be only one stream, so it should be set to nil at this point
		stream = s
	}

	// Let's explain the input data here a bit. It consists of a stream of kernel launches,
	// interseded with some cudaStreamSynchronize calls. After all that there's a longer cudaStreamSynchronize
	// and then a batch of synchronizations.

	require.NotNil(t, stream)
	require.Len(t, stream.kernelSpans, 10)                             // There are 10 uninterrupted sequences of kernel launches
	require.Len(t, stream.kernelLaunches, 0)                           // And we should have no kernel launches in the stream pending
	require.Equal(t, stream.metadata.gpuUUID, testutil.DefaultGpuUUID) // Ensure the metadata is set correctly
	require.Equal(t, stream.metadata.pid, uint32(executingPID))

	totalThreadSeconds := 0.0
	activeSeconds := 0.0

	for _, span := range stream.kernelSpans {
		totalThreadSeconds += float64(span.avgThreadCount) * float64(span.endKtime-span.startKtime) / 1e9
		activeSeconds += float64(span.endKtime-span.startKtime) / 1e9
	}

	// Manually calculated
	expectedTotalThreadSeconds := 3588668.5
	expectedActiveSeconds := 3.379124306
	require.InDelta(t, totalThreadSeconds, expectedTotalThreadSeconds, 0.1)
	require.InDelta(t, activeSeconds, expectedActiveSeconds, 0.001)

	// Check that the kernel launches don't span the entire interval of events
	startTs := events.Events[0].Header.Ktime_ns
	endTs := events.Events[len(events.Events)-1].Header.Ktime_ns
	firstSyncAfterLastKernelLaunchTs := events.Events[866].Header.Ktime_ns

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
	stats, err := statsGen.getStats(int64(endTs + 1))
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Len(t, stats.Metrics, 1)

	metrics := stats.Metrics[0]

	require.Equal(t, metrics.Key.PID, uint32(executingPID))

	// Because the number of threads reported by the mock devices is very low, lower than the
	// number of cores used by the sample events, the number of cores is going to get normalized
	// to the number of cores available. This makes the used cores numbers computable based just
	// on the number of active seconds
	sampleDurationSeconds := float64(endTs-startTs) / 1e9
	activeFraction := activeSeconds / sampleDurationSeconds
	expectedUsedCores := activeFraction * float64(testutil.DefaultGpuCores)
	require.InDelta(t, expectedUsedCores, metrics.UtilizationMetrics.UsedCores, 0.001)
}
