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
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
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
	configmock.New(t)

	const iterations = 100
	missed := 0

	for i := 0; i < iterations; i++ {
		demux, err := InitAndStartServerlessDemultiplexer(nil, time.Second, nooptagger.NewComponent(), false)
		require.NoError(t, err)

		// Submit via the real API — same path as metric.Add → Shutdown.
		demux.AggregateSample(metrics.MetricSample{
			Name:       "test.metric",
			Value:      1,
			Mtype:      metrics.GaugeType,
			Tags:       []string{},
			SampleRate: 1,
			Timestamp:  1000.0,
		})

		// Wait via PendingSamples — same as WaitForPendingSamples.
		// Removing this loop causes ~50% of iterations to miss the
		// sample, which is the race condition this fix addresses.
		for demux.PendingSamples() > 0 {
			runtime.Gosched()
		}

		// Flush via statsdWorker.flushChan — same mechanism
		// ForceFlushToSerializer uses. We send directly so we can
		// capture the series output without a real forwarder.
		var series metrics.Series
		var sketches metrics.SketchSeriesList
		blockChan := make(chan struct{}, 1)

		demux.statsdWorker.flushChan <- flushTrigger{
			trigger: trigger{
				time:          time.Unix(1010, 0), // after the sample timestamp
				blockChan:     blockChan,
				forceFlushAll: true,
			},
			seriesSink:   &series,
			sketchesSink: &sketches,
		}

		<-blockChan
		demux.Stop(false)

		if len(series) == 0 {
			missed++
		} else {
			assert.Equal(t, "test.metric", series[0].Name)
			assert.Len(t, series[0].Points, 1)
			assert.Equal(t, float64(1), series[0].Points[0].Value)
		}
	}

	assert.Zero(t, missed, "WaitForPendingSamples pattern should ensure all samples are flushed, "+
		"but missed %d/%d iterations", missed, iterations)
	require.Zero(t, missed)
}
