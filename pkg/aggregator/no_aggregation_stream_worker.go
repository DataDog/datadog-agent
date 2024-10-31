// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"expvar"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/util"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// noAggregationStreamWorker is streaming received metrics from the DogStatsD batcher
// to the serializer.
//
// While streaming metrics to the serializer, the serializer should be responsible of sending the payloads
// to the forwarder once one is generated (because full), even while still receiving metrics.
// However, it's not how the serializer works today: it piles up the payloads, wait until the end of the
// streamings ending and then flushes the payloads.
// In order to make sure the serializer flushes data to the forwarder, we stop the streaming once a given
// amount of samples have been sent to the serializer.
// A timer is also triggering a serializer flush if nothing has been received for a while.
// In an ideal future, we would not have to implement this mechanism in this part
// of the code (i.e. the serializer should), especially since it may create
// really small payloads (that could have potentially been filled).
type noAggregationStreamWorker struct {
	serializer           serializer.MetricSerializer
	flushConfig          FlushAndSerializeInParallel
	maxMetricsPerPayload int

	// pointer to the shared MetricSamplePool stored in the Demultiplexer.
	metricSamplePool *metrics.MetricSamplePool

	seriesSink   *metrics.IterableSeries
	sketchesSink *metrics.IterableSketches

	taggerBuffer *tagset.HashlessTagsAccumulator
	metricBuffer *tagset.HashlessTagsAccumulator

	samplesChan chan metrics.MetricSampleBatch
	stopChan    chan trigger

	hostTagProvider *HostTagProvider
	tagger          tagger.Component

	logThrottling util.SimpleThrottler
}

// noAggWorkerStreamCheckFrequency is the frequency at which the no agg worker
// is checking if it has some samples to flush. It triggers this flush only
// if it not still receiving samples.
var noAggWorkerStreamCheckFrequency = time.Second * 2

// Telemetry vars
var (
	noaggExpvars                               = expvar.NewMap("no_aggregation")
	expvarNoAggSamplesProcessedOk              = expvar.Int{}
	expvarNoAggSamplesProcessedUnsupportedType = expvar.Int{}
	expvarNoAggFlush                           = expvar.Int{}

	tlmNoAggSamplesProcessed                = telemetry.NewCounter("no_aggregation", "processed", []string{"state"}, "Count the number of samples processed by the no-aggregation pipeline worker")
	tlmNoAggSamplesProcessedOk              = tlmNoAggSamplesProcessed.WithValues("ok")
	tlmNoAggSamplesProcessedUnsupportedType = tlmNoAggSamplesProcessed.WithValues("unsupported_type")

	tlmNoAggFlush = telemetry.NewSimpleCounter("no_aggregation", "flush", "Count the number of flushes done by the no-aggregation pipeline worker")
)

func init() {
	noaggExpvars.Set("ProcessedOk", &expvarNoAggSamplesProcessedOk)
	noaggExpvars.Set("ProcessedUnsupportedType", &expvarNoAggSamplesProcessedUnsupportedType)
	noaggExpvars.Set("Flush", &expvarNoAggFlush)
}

//nolint:revive // TODO(AML) Fix revive linter
func newNoAggregationStreamWorker(maxMetricsPerPayload int, _ *metrics.MetricSamplePool,
	serializer serializer.MetricSerializer, flushConfig FlushAndSerializeInParallel,
	tagger tagger.Component,
) *noAggregationStreamWorker {
	return &noAggregationStreamWorker{
		serializer:           serializer,
		flushConfig:          flushConfig,
		maxMetricsPerPayload: maxMetricsPerPayload,

		seriesSink:   nil,
		sketchesSink: nil,

		taggerBuffer: tagset.NewHashlessTagsAccumulator(),
		metricBuffer: tagset.NewHashlessTagsAccumulator(),

		stopChan:    make(chan trigger),
		samplesChan: make(chan metrics.MetricSampleBatch, pkgconfigsetup.Datadog().GetInt("dogstatsd_queue_size")),

		hostTagProvider: NewHostTagProvider(),
		// warning for the unsupported metric types should appear maximum 200 times
		// every 5 minutes.
		logThrottling: util.NewSimpleThrottler(200, 5*time.Minute, "Pausing the unsupported metric type warning message for 5m"),

		tagger: tagger,
	}
}

func (w *noAggregationStreamWorker) addSamples(samples metrics.MetricSampleBatch) {
	if len(samples) == 0 {
		return
	}
	// FIXME: instrument
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

// mainloop of the no aggregation stream worker:
//   - it receives samples and counts how much it has sent to the serializer, if it has more than a given amount it stops
//     streaming for the serializer to start sending the payloads to the forwarder, and then starts the streaming
//     mainloop again
//   - it also checks every 2 seconds if it has stopped receiving samples, if so, it stops streaming to the
//     the serializer for a while in order to let the serializer sends the payloads up to the forwarder, and starts
//     the streaming mainloop again
//   - listens for a stop signal
//   - listens for a flush signal
//
// This is not ideal since the serializer should automatically takes the decision when to flush payloads to
// the serializer but that's not how it works today, see noAggregationStreamWorker comment.
func (w *noAggregationStreamWorker) run() {
	log.Debugf("Starting streaming routine for the no-aggregation pipeline")

	ticker := time.NewTicker(noAggWorkerStreamCheckFrequency)
	defer ticker.Stop()
	logPayloads := pkgconfigsetup.Datadog().GetBool("log_payloads")
	w.seriesSink, w.sketchesSink = createIterableMetrics(w.flushConfig, w.serializer, logPayloads, false, w.hostTagProvider)

	stopped := false
	var stopBlockChan chan struct{}
	var lastStream time.Time

	for !stopped {
		start := time.Now()
		serializedSamples := 0

		metrics.Serialize(
			w.seriesSink,
			w.sketchesSink,
			func(_ metrics.SerieSink, _ metrics.SketchesSink) {
			mainloop:
				for {
					select {

					// stop signal
					case trigger := <-w.stopChan:
						stopped = true
						stopBlockChan = trigger.blockChan
						break mainloop // end `Serialize` call and trigger a flush to the forwarder

					case <-ticker.C:
						n := time.Now()
						if serializedSamples > 0 && lastStream.Before(n.Add(-time.Second*1)) {
							log.Debug("noAggregationStreamWorker: triggering an automatic payloads flush to the forwarder (no traffic since 1s)")
							tlmNoAggFlush.Add(1)
							expvarNoAggFlush.Add(1)
							break mainloop // end `Serialize` call and trigger a flush to the forwarder
						}

					// receiving samples
					case samples := <-w.samplesChan:
						log.Tracef("Streaming %d metrics from the no-aggregation pipeline", len(samples))
						countProcessed := 0
						countUnsupportedType := 0

						for _, sample := range samples {
							mtype, supported := metricSampleAPIType(sample)

							if !supported {
								if !w.logThrottling.ShouldThrottle() {
									log.Warnf("Discarding unsupported metric sample in the no-aggregation pipeline for sample '%s', sample type '%s'", sample.Name, sample.Mtype.String())
								}
								countUnsupportedType++
								continue
							}

							// enrich metric sample tags
							sample.GetTags(w.taggerBuffer, w.metricBuffer, w.tagger.EnrichTags)
							w.metricBuffer.AppendHashlessAccumulator(w.taggerBuffer)

							// if the value is a rate, we have to account for the 10s interval
							if mtype == metrics.APIRateType {
								sample.Value /= bucketSize
							}

							// turns this metric sample into a serie
							var serie metrics.Serie
							serie.Name = sample.Name
							serie.Points = []metrics.Point{{Ts: sample.Timestamp, Value: sample.Value}}
							serie.Tags = tagset.CompositeTagsFromSlice(w.metricBuffer.Copy())
							serie.Host = sample.Host
							serie.MType = mtype
							serie.Interval = bucketSize
							w.seriesSink.Append(&serie)

							w.taggerBuffer.Reset()
							w.metricBuffer.Reset()
							countProcessed++
						}

						lastStream = time.Now()

						serializedSamples += countProcessed

						tlmNoAggSamplesProcessedOk.Add(float64(countProcessed))
						expvarNoAggSamplesProcessedOk.Add(int64(countProcessed))
						tlmNoAggSamplesProcessedUnsupportedType.Add(float64(countUnsupportedType))
						expvarNoAggSamplesProcessedUnsupportedType.Add(int64(countUnsupportedType))

						w.metricSamplePool.PutBatch(samples) // return the sample batch back to the pool for reuse

						if serializedSamples > w.maxMetricsPerPayload {
							tlmNoAggFlush.Add(1)
							break mainloop // end `Serialize` call and trigger a flush to the forwarder
						}
					}
				}
			}, func(serieSource metrics.SerieSource) {
				sendIterableSeries(w.serializer, start, serieSource)
			}, func(_ metrics.SketchesSource) {
				// noop: we do not support sketches in the no-agg pipeline.
			})

		if stopped {
			break
		}

		w.seriesSink, w.sketchesSink = createIterableMetrics(w.flushConfig, w.serializer, logPayloads, false, w.hostTagProvider)
	}

	if stopBlockChan != nil {
		close(stopBlockChan)
	}
}

// metricSampleAPIType returns the APIMetricType of the given sample, the second
// return value informs the caller if the input type is supported by
// the no-aggregation pipeline: APIMetricType only supports gauges, counts and rates.
// This method will default on gauges for every other inputs and return false as a second return value.
func metricSampleAPIType(m metrics.MetricSample) (metrics.APIMetricType, bool) {
	switch m.Mtype {
	case metrics.GaugeType:
		return metrics.APIGaugeType, true
	case metrics.CounterType:
		return metrics.APIRateType, true
	case metrics.RateType:
		return metrics.APIRateType, true
	default:
		return metrics.APIGaugeType, false
	}
}
