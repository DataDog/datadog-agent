// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

// TestWaitForPendingSamplesThenFlush verifies that waiting for
// samplesChan to drain before flushing guarantees all samples are
// included. This is the mechanism behind WaitForPendingSamples used
// during serverless-init shutdown.
//
// Without the wait (removing the for-loop below), Go's select picks
// randomly between samplesChan and flushChan when both are ready,
// causing samples to be missed ~50% of the time.
func TestWaitForPendingSamplesThenFlush(t *testing.T) {
	const iterations = 100
	missed := 0

	for i := 0; i < iterations; i++ {
		store := tags.NewStore(true, "test")
		sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "")
		pool := metrics.NewMetricSamplePool(MetricSamplePoolBatchSize, false)
		tagMatcher := filterlistimpl.NewNoopTagMatcher()

		worker := newTimeSamplerWorker(
			sampler,
			10*time.Second,
			10, // buffered samplesChan
			pool,
			FlushAndSerializeInParallel{},
			store,
			utilstrings.NewMatcher(nil, false),
			tagMatcher,
		)

		// Start the worker goroutine.
		go worker.run()

		// Submit a sample via the channel.
		batch := pool.GetBatch()
		batch[0] = metrics.MetricSample{
			Name:       "test.metric",
			Value:      1,
			Mtype:      metrics.GaugeType,
			Tags:       []string{},
			SampleRate: 1,
			Timestamp:  1000.0,
		}
		worker.samplesChan <- batch[:1]

		// Wait for the sample to be consumed — this is the pattern
		// that WaitForPendingSamples / PendingSamples implements.
		// Removing this loop causes ~50% of iterations to miss the
		// sample, which is the race condition this fix addresses.
		for len(worker.samplesChan) > 0 {
			runtime.Gosched()
		}

		// flushChan is unbuffered, so this send blocks until the
		// worker is back in select — which is after sample()
		// completes. The metric is therefore in a bucket.
		var series metrics.Series
		var sketches metrics.SketchSeriesList
		blockChan := make(chan struct{}, 1)

		worker.flushChan <- flushTrigger{
			trigger: trigger{
				time:          time.Unix(1010, 0), // after the sample timestamp
				blockChan:     blockChan,
				forceFlushAll: true,
			},
			seriesSink:   &series,
			sketchesSink: &sketches,
		}

		// Wait for flush to complete.
		<-blockChan
		worker.stop()

		if len(series) == 0 {
			missed++
		}
	}

	assert.Zero(t, missed, "WaitForPendingSamples pattern should ensure all samples are flushed, "+
		"but missed %d/%d iterations", missed, iterations)
	require.Zero(t, missed)
}
