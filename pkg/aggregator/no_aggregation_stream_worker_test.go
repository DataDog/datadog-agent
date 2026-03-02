// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"testing"
	"time"
)

// TestNoAggStreamWorkerSeriesDisabled is a regression test for a nil pointer
// dereference that occurs when AreSeriesEnabled() returns false. In that case,
// createIterableMetrics returns a nil *IterableSeries, and the worker's
// producer callback was calling w.seriesSink.Append() directly instead of
// using the nil-safe SerieSink parameter provided by Serialize().
func TestNoAggStreamWorkerSeriesDisabled(t *testing.T) {
	noAggWorkerStreamCheckFrequency = 100 * time.Millisecond

	opts := demuxTestOptions()
	opts.EnableNoAggregationPipeline = true

	mockSerializer := &MockSerializerIterableSerie{}
	mockSerializer.On("AreSeriesEnabled").Return(false)
	mockSerializer.On("AreSketchesEnabled").Return(false)

	deps := createDemultiplexerAgentTestDeps(t)
	demux := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")
	demux.statsd.noAggStreamWorker.serializer = mockSerializer

	go demux.run()

	batch := testDemuxSamples(t)
	demux.SendSamplesWithoutAggregation(batch)

	// Give time for the worker to process the samples. If the bug is present,
	// the worker goroutine will panic with a nil pointer dereference, crashing
	// the test process.
	time.Sleep(200 * time.Millisecond)
	demux.Stop(true)
}
