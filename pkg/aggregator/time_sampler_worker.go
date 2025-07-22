// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"io"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

// The timeSamplerWorker runs the process loop for a TimeSampler:
//   - receiving samples for the TimeSampler to process
//   - receiving flush triggers to flush the series from the TimeSampler
//     into a serializer
type timeSamplerWorker struct {
	// parent sampler the timeSamplerWorker is responsible of
	sampler *TimeSampler

	// pointer to the shared MetricSamplePool stored in the Demultiplexer.
	metricSamplePool *metrics.MetricSamplePool

	// flushInterval is the automatic flush interval
	flushInterval time.Duration

	// flushBlocklist is the blocklist used when flushing metrics to the serializer.
	// It's main use-case is to filter out some metrics after their aggregation
	// process, such as histograms which create several metrics.
	flushBlocklist *utilstrings.Blocklist

	// parallel serialization configuration
	parallelSerialization FlushAndSerializeInParallel

	// samplesChan is used to communicate between from the processLoop receiving the
	// samples and the TimeSampler.
	samplesChan chan []metrics.MetricSample
	// use this chan to trigger a flush of the time sampler
	flushChan chan flushTrigger
	// use this chan to trigger a blocklist reconfiguration
	blocklistChan chan *utilstrings.Blocklist
	// use this chan to stop the timeSamplerWorker
	stopChan chan struct{}
	// channel to trigger interactive dump of the context resolver
	dumpChan chan dumpTrigger

	// tagsStore shard used to store tag slices for this worker
	tagsStore *tags.Store
}

type dumpTrigger struct {
	dest io.Writer
	done chan error
}

func newTimeSamplerWorker(sampler *TimeSampler, flushInterval time.Duration, bufferSize int,
	metricSamplePool *metrics.MetricSamplePool,
	parallelSerialization FlushAndSerializeInParallel, tagsStore *tags.Store) *timeSamplerWorker {
	return &timeSamplerWorker{
		sampler: sampler,

		metricSamplePool:      metricSamplePool,
		parallelSerialization: parallelSerialization,

		flushInterval: flushInterval,

		samplesChan:   make(chan []metrics.MetricSample, bufferSize),
		stopChan:      make(chan struct{}),
		flushChan:     make(chan flushTrigger),
		dumpChan:      make(chan dumpTrigger),
		blocklistChan: make(chan *utilstrings.Blocklist),

		tagsStore: tagsStore,
	}
}

// We process all receivend samples in the `select`, but we also process a flush action,
// meaning that the time sampler does not process any sample while flushing.
// Note that it was the same design in the BufferedAggregator (but at the aggregator level,
// not sampler level).
// If we want to move to a design where we can flush while we are processing samples,
// we could consider implementing double-buffering or locking for every sample reception.
func (w *timeSamplerWorker) run() {
	shard := w.sampler.idString

	for {
		tlmChannelSize.Set(float64(len(w.samplesChan)), shard)
		select {
		case <-w.stopChan:
			return
		case ms := <-w.samplesChan:
			aggregatorDogstatsdMetricSample.Add(int64(len(ms)))
			tlmProcessed.Add(float64(len(ms)), shard, "dogstatsd_metrics")
			t := timeNowNano()
			for i := 0; i < len(ms); i++ {
				w.sampler.sample(&ms[i], t)
			}
			w.metricSamplePool.PutBatch(ms)
		case blocklist := <-w.blocklistChan:
			w.flushBlocklist = blocklist
		case trigger := <-w.flushChan:
			w.triggerFlush(trigger)
			w.tagsStore.Shrink()
		case trigger := <-w.dumpChan:
			trigger.done <- w.sampler.dumpContexts(trigger.dest)
		}
	}
}

func (w *timeSamplerWorker) stop() {
	w.stopChan <- struct{}{}
}

func (w *timeSamplerWorker) triggerFlush(trigger flushTrigger) {
	w.sampler.flush(float64(trigger.time.Unix()), trigger.seriesSink, trigger.sketchesSink, w.flushBlocklist, trigger.forceFlushAll)
	trigger.blockChan <- struct{}{}
}

func (w *timeSamplerWorker) dumpContexts(dest io.Writer) error {
	done := make(chan error)
	w.dumpChan <- dumpTrigger{dest: dest, done: done}
	return <-done
}
