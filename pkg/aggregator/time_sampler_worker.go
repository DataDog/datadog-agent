// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"context"
	"io"
	"time"

	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/metricname"
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

	// tagFilterList is used to filter tags for specific metrics during context tracking.
	// It determines which tags should be kept or stripped for distribution metrics.
	tagFilterList filterlist.TagMatcher
	// use this chan to trigger a tagFilterList reconfiguration
	tagFilterListChan chan filterlist.TagMatcher

	// flushFilterList is the filter applied when flushing metrics to the serializer.
	// It's main use-case is to filter out some metrics after their aggregation
	// process, such as histograms which create several metrics.
	flushFilterList metricname.Matcher

	// parallel serialization configuration
	parallelSerialization FlushAndSerializeInParallel

	// samplesChan is used to communicate between from the processLoop receiving the
	// samples and the TimeSampler.
	samplesChan chan []metrics.MetricSample
	// samplesDrained is closed after last sample is read from samplesChan
	samplesDrained chan struct{}
	// drainChan requests a synchronous drain of samplesChan. Unlike
	// shutdown/samplesDrained, it's repeatable and doesn't touch the worker's
	// lifecycle.
	drainChan chan chan struct{}
	// use this chan to trigger a flush of the time sampler
	flushChan chan flushTrigger
	// use this chan to trigger a filterList reconfiguration
	metricFilterListChan chan metricname.Matcher
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
	parallelSerialization FlushAndSerializeInParallel, tagsStore *tags.Store,
	flushFilterList metricname.Matcher, tagFilterList filterlist.TagMatcher) *timeSamplerWorker {
	return &timeSamplerWorker{
		sampler: sampler,

		metricSamplePool:      metricSamplePool,
		parallelSerialization: parallelSerialization,

		flushInterval:   flushInterval,
		tagFilterList:   tagFilterList,
		flushFilterList: flushFilterList,

		samplesChan:          make(chan []metrics.MetricSample, bufferSize),
		samplesDrained:       make(chan struct{}),
		drainChan:            make(chan chan struct{}),
		stopChan:             make(chan struct{}),
		flushChan:            make(chan flushTrigger),
		dumpChan:             make(chan dumpTrigger),
		metricFilterListChan: make(chan metricname.Matcher),
		tagFilterListChan:    make(chan filterlist.TagMatcher),

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
			if ms == nil {
				close(w.samplesDrained)
				continue
			}
			w.processBatch(ms)
		case matcher := <-w.metricFilterListChan:
			w.flushFilterList = matcher
		case matcher := <-w.tagFilterListChan:
			w.setFilterList(matcher)
		case trigger := <-w.flushChan:
			w.triggerFlush(trigger)
			w.tagsStore.Shrink()
		case trigger := <-w.dumpChan:
			trigger.done <- w.sampler.dumpContexts(trigger.dest)
		case done := <-w.drainChan:
			// select doesn't order drainChan against samplesChan, so drain
			// whatever's already buffered before acking.
		drainLoop:
			for {
				select {
				case ms := <-w.samplesChan:
					if ms == nil {
						close(w.samplesDrained)
						continue
					}
					w.processBatch(ms)
				default:
					break drainLoop
				}
			}
			close(done)
		}
	}
}

// processBatch samples ms, called from run()'s normal loop or from a drain.
func (w *timeSamplerWorker) processBatch(ms []metrics.MetricSample) {
	shard := w.sampler.idString
	aggregatorDogstatsdMetricSample.Add(int64(len(ms)))
	tlmProcessed.Add(float64(len(ms)), shard, "dogstatsd_metrics")
	t := timeNowNano()

	for i := 0; i < len(ms); i++ {
		w.sampler.sample(&ms[i], t, w.tagFilterList)
	}
	w.metricSamplePool.PutBatch(ms)
}

// Set a new filterlist, ensuring we also clear the context resolver strip cache
func (w *timeSamplerWorker) setFilterList(matcher filterlist.TagMatcher) {
	w.sampler.contextResolver.resolver.clearTagFilterCache()
	w.tagFilterList = matcher
}

func (w *timeSamplerWorker) stop() {
	w.stopChan <- struct{}{}
}

func (w *timeSamplerWorker) triggerFlush(trigger flushTrigger) {
	w.sampler.flush(float64(trigger.time.Unix()), trigger.seriesSink, trigger.sketchesSink, &w.flushFilterList, trigger.forceFlushAll)
	trigger.blockChan <- struct{}{}
}

func (w *timeSamplerWorker) dumpContexts(dest io.Writer) error {
	done := make(chan error)
	w.dumpChan <- dumpTrigger{dest: dest, done: done}
	return <-done
}

func (w *timeSamplerWorker) addSamples(ms []metrics.MetricSample) {
	if len(ms) == 0 {
		return
	}
	w.samplesChan <- ms
}

func (w *timeSamplerWorker) shutdown(ctx context.Context) {
	// Close would be more idiomatic, but we can't be sure everyone
	// holding a ref to the demultiplexer will be stopped before
	// demultiplexer Stop(), and we don't want panics.
	select {
	case w.samplesChan <- nil:
	case <-ctx.Done():
	}
}

func (w *timeSamplerWorker) waitForShutdown(ctx context.Context) {
	select {
	case <-w.samplesDrained:
	case <-ctx.Done():
	}
}

// waitForPendingSamples blocks until samples enqueued before this call have
// been processed. Safe to call repeatedly, unlike shutdown/waitForShutdown.
func (w *timeSamplerWorker) waitForPendingSamples() {
	done := make(chan struct{})
	w.drainChan <- done
	<-done
}
