// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// noAggregationWorker is forwarding metric sample batch received from the DogStatsD batcher to the serializer.
// It uses its own MetricSamplePool in order to control the size of the batches sent to the serializer (i.e. controlling
// the payloads size).
type noAggregationWorker struct {
	currentBatch    metrics.MetricSampleBatch
	currentBatchIdx int
	samplePool      *metrics.MetricSamplePool
	batchSize       int

	m sync.Mutex

	serializer  *serializer.Serializer
	flushConfig FlushAndSerializeInParallel
}

// TODO(remy): mocked serializer to unit test the addSamples with a batchSize of 16, directly test the json
func newNoAggregationWorker(batchSize int, serializer *serializer.Serializer, flushConfig FlushAndSerializeInParallel) *noAggregationWorker {
	samplePool := metrics.NewMetricSamplePool(batchSize)
	return &noAggregationWorker{
		serializer:   serializer,
		flushConfig:  flushConfig,
		batchSize:    batchSize,
		samplePool:   samplePool,
		currentBatch: samplePool.GetBatch(),
	}
}

func (w *noAggregationWorker) addSamples(samples metrics.MetricSampleBatch) {
	w.m.Lock()
	defer w.m.Unlock()

	toProcess := len(samples)
	idx := 0

	// complete batches
	// (for now this should never happen since the MetricSampleBatch from the batcher are of fixed size 32 atm,
	// but this may be revisited for the no aggregation pipeline metrics. I prefer to ship this already implemented
	// for 1. performance reasons if that happens 2. completeness of this function implementation on its own).

	for toProcess >= w.batchSize {
		batch := w.samplePool.GetBatch()
		copy(batch[0:w.batchSize], samples[idx:idx+w.batchSize])
		go w.flush(batch)
		toProcess -= w.batchSize
		idx += w.batchSize

		if toProcess == 0 {
			return
		}

		continue
	}

	// not fitting the space left in current batch, fill it, flush it and get a new empty batch

	leftSpace := cap(w.currentBatch) - w.currentBatchIdx

	if toProcess > leftSpace {
		copy(w.currentBatch[w.currentBatchIdx:cap(w.currentBatch)], samples[idx:idx+leftSpace])
		go w.flush(w.currentBatch)
		toProcess -= leftSpace
		idx += leftSpace
		w.currentBatch = w.samplePool.GetBatch()
		w.currentBatchIdx = 0
	}

	// copy everything left in current batch

	copy(w.currentBatch[w.currentBatchIdx:w.currentBatchIdx+toProcess], samples[idx:idx+toProcess])
	w.currentBatchIdx += toProcess
}

func (w *noAggregationWorker) flush(samples metrics.MetricSampleBatch) {
	if len(samples) == 0 {
		return
	}

	logPayloads := config.Datadog.GetBool("log_payloads")
	series, sketches := createIterableMetrics(w.flushConfig, w.serializer, logPayloads, false)
	start := time.Now()

	metrics.Serialize(
		series,
		sketches,
		func(seriesSink metrics.SerieSink, sketchesSink metrics.SketchesSink) {

			// flush the historical metrics if any
			// -------------------------------------

			if len(samples) > 0 {
				log.Debugf("Flushing %d metrics from the no-aggregation pipeline", len(samples))
				// TODO(remy): we can consider re-using these instead of building them on every flush.
				taggerBuffer := tagset.NewHashlessTagsAccumulator()
				metricBuffer := tagset.NewHashlessTagsAccumulator()
				for _, sample := range samples {
					// enrich metric sample tags
					sample.GetTags(taggerBuffer, metricBuffer)
					metricBuffer.AppendHashlessAccumulator(taggerBuffer)

					// turns this metric sample into a serie
					var serie metrics.Serie
					serie.Name = sample.Name
					// TODO(remy): we may have to sort uniq them? and is this copy actually needed?
					serie.Points = []metrics.Point{{Ts: sample.Timestamp, Value: sample.Value}}
					serie.Tags = tagset.CompositeTagsFromSlice(metricBuffer.Copy())
					serie.Host = sample.Host
					serie.Interval = 0 // TODO(remy): document me
					seriesSink.Append(&serie)

					taggerBuffer.Reset()
					metricBuffer.Reset()
				}

				w.samplePool.PutBatch(samples)
			}
		}, func(serieSource metrics.SerieSource) {
			sendIterableSeries(w.serializer, start, serieSource)
		},
		func(sketches metrics.SketchesSource) {
		})

	addFlushTime("MainFlushTime", int64(time.Since(start)))
	aggregatorNumberOfFlush.Add(1)
}
