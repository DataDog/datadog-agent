// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// noAggregationStreamWorker is strreaming received metrics from the DogStatsD batcher
// to the serializer.
//
// While streaming metrics to the serializer, the serializer should be responsible of sending the payloads
// to the forwarder once one is generated (because full), even if it is still receiving metrics.
// It's not how it works today, it will pile up the payloads, wait until the streaming is ending and then
// flush the payloads.
// In order to make sure it flushes data to the forwarder, we stop the streaming once a given amount of
// samples have been sent to the serializer. A timer is also triggering a flush if nothing has been
// received for a while.
// In an ideal future, we would not have to implement this mechanism in this part
// of the code (i.e. the serializer should), especially since it may create
// really small payloads (that could have potentially been filled).
type noAggregationStreamWorker struct {
	serializer           serializer.MetricSerializer
	flushConfig          FlushAndSerializeInParallel
	maxMetricsPerPayload int

	seriesSink   *metrics.IterableSeries
	sketchesSink *metrics.IterableSketches

	taggerBuffer *tagset.HashlessTagsAccumulator
	metricBuffer *tagset.HashlessTagsAccumulator

	samplesChan chan metrics.MetricSampleBatch
	stopChan    chan trigger
	flushChan   chan trigger
}

func newNoAggregationStreamWorker(maxMetricsPerPayload int, serializer serializer.MetricSerializer, flushConfig FlushAndSerializeInParallel) *noAggregationStreamWorker {
	return &noAggregationStreamWorker{
		serializer:           serializer,
		flushConfig:          flushConfig,
		maxMetricsPerPayload: maxMetricsPerPayload,

		seriesSink:   nil,
		sketchesSink: nil,

		taggerBuffer: tagset.NewHashlessTagsAccumulator(),
		metricBuffer: tagset.NewHashlessTagsAccumulator(),

		stopChan:    make(chan trigger),
		flushChan:   make(chan trigger),
		samplesChan: make(chan metrics.MetricSampleBatch, config.Datadog.GetInt("dogstatsd_queue_size")),
	}
}

func (w *noAggregationStreamWorker) addSamples(samples metrics.MetricSampleBatch) {
	if len(samples) == 0 {
		return
	}
	w.samplesChan <- samples
}

func (w *noAggregationStreamWorker) stop(wait bool) {
	var blockChan chan struct{}
	if wait {
		blockChan = make(chan struct{})
	}

	trigger := trigger{
		time:      time.Now(),
		blockChan: blockChan,
	}

	w.stopChan <- trigger

	if wait {
		<-blockChan
	}
}

func (w *noAggregationStreamWorker) flush(wait bool) {
	var blockChan chan struct{}
	if wait {
		blockChan = make(chan struct{})
	}

	trigger := trigger{
		time:      time.Now(),
		blockChan: blockChan,
	}

	w.flushChan <- trigger

	if wait {
		<-blockChan
	}
}

// mainloop of the no aggregation stream worker:
//   * it receives samples and counts how much it has sent to the serializer, if it has more than a given amount it stops
//     streaming for the serializer to start sending the payloads to the forwarder, and then starts the streaming
//     mainloop again
//   * it also checks every 2 seconds if it has stopped receiving samples, if so, it stops streaming to the
//     the serializer for a while in order to let the serializer sends the payloads up to the forwarder, and starts
//     the streaming mainloop again
//   * listens for a stop signal
//   * listens for a flush signal
// This is not ideal since the serializer should automatically takes the decision when to flush payloads to
// the serializer but that's not how it works today, see noAggregationStreamWorker comment.
func (w *noAggregationStreamWorker) run() {
	log.Debugf("Starting streaming routine for the no-aggregation pipeline")

	ticker := time.NewTicker(time.Second * 2)
	defer ticker.Stop()
	lastStream := time.Now().Add(time.Hour * 24)

	logPayloads := config.Datadog.GetBool("log_payloads")
	w.seriesSink, w.sketchesSink = createIterableMetrics(w.flushConfig, w.serializer, logPayloads, false)

	stopped := false
	var flushBlockChan chan struct{}
	var stopBlockChan chan struct{}

	for !stopped {
		start := time.Now()
		serializedSamples := 0

		metrics.Serialize(
			w.seriesSink,
			w.sketchesSink,
			func(seriesSink metrics.SerieSink, sketchesSink metrics.SketchesSink) {
			mainloop:
				for {
					select {
					// stop signal
					case trigger := <-w.stopChan:
						stopped = true
						stopBlockChan = trigger.blockChan
						break mainloop

					// ticker regularly producing a flush signal if necessary
					case <-ticker.C:
						n := time.Now()
						if lastStream.Before(n.Add(-time.Second*1)) && serializedSamples > 0 {
							log.Debug("noAggregationStreamWorker: triggering an automatic payloads flush to the forwarder (no traffic since 1s)")
							go w.flush(true)
							lastStream = n.Add(time.Hour * 24)
						}

					// flush signal
					case trigger := <-w.flushChan:
						flushBlockChan = trigger.blockChan
						break mainloop

					// receiving samples
					case samples := <-w.samplesChan:
						log.Debugf("Streaming %d metrics from the no-aggregation pipeline", len(samples))
						for _, sample := range samples {
							// enrich metric sample tags
							sample.GetTags(w.taggerBuffer, w.metricBuffer)
							w.metricBuffer.AppendHashlessAccumulator(w.taggerBuffer)

							// turns this metric sample into a serie
							var serie metrics.Serie
							serie.Name = sample.Name
							// TODO(remy): we may have to sort uniq them? and is this copy actually needed?
							serie.Points = []metrics.Point{{Ts: sample.Timestamp, Value: sample.Value}}
							serie.Tags = tagset.CompositeTagsFromSlice(w.metricBuffer.Copy())
							serie.Host = sample.Host
							serie.Interval = 0 // TODO(remy): document me
							w.seriesSink.Append(&serie)

							w.taggerBuffer.Reset()
							w.metricBuffer.Reset()
						}

						lastStream = time.Now()

						serializedSamples += len(samples)
						if serializedSamples > w.maxMetricsPerPayload {
							go w.flush(true)
						}
					}
				}
			}, func(serieSource metrics.SerieSource) {
				sendIterableSeries(w.serializer, start, serieSource)
				// the flush trigger may have set this flushBlockChan to a channel on which
				// we need to send a signal to indicate the end of the flush.
				if flushBlockChan != nil {
					flushBlockChan <- struct{}{}
					flushBlockChan = nil
				}
			}, func(sketches metrics.SketchesSource) {
				// noop: we do not support sketches in the no-agg pipeline.
			})

		if stopped {
			break
		}

		w.seriesSink, w.sketchesSink = createIterableMetrics(w.flushConfig, w.serializer, logPayloads, false)
	}

	if stopBlockChan != nil {
		stopBlockChan <- struct{}{}
	}
}
