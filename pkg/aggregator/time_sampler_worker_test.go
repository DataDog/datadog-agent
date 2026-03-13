// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
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

// TestFlushIncludesPendingSamples verifies that samples buffered in
// samplesChan are processed before a concurrent flush trigger, so that
// metrics submitted just before a flush (e.g. during shutdown) are not lost.
//
// Without the drainSamples() call in the flush path, Go's select picks
// randomly between samplesChan and flushChan when both are ready,
// causing samples to be missed ~50% of the time.
func TestFlushIncludesPendingSamples(t *testing.T) {
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

		// Pre-fill samplesChan so the sample is already buffered when
		// the worker starts. This guarantees that samplesChan and
		// flushChan are both ready in the same select evaluation.
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

		// Start the worker goroutine.
		go worker.run()

		// Send a flush trigger. The worker's select will see both
		// samplesChan (buffered sample) and flushChan (this trigger)
		// as ready simultaneously.
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

	assert.Zero(t, missed, "flush missed buffered samples in %d/%d iterations — "+
		"drainSamples() before flush is needed to prevent this race", missed, iterations)
	require.Zero(t, missed)
}
