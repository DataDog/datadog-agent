// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"expvar"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/util"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
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
func newNoAggregationStreamWorker(maxMetricsPerPayload int, metricSamplePool *metrics.MetricSamplePool,
	serializer serializer.MetricSerializer, flushConfig FlushAndSerializeInParallel,
) *noAggregationStreamWorker {
	panic("not called")
}

func (w *noAggregationStreamWorker) addSamples(samples metrics.MetricSampleBatch) {
	panic("not called")
}

func (w *noAggregationStreamWorker) stop(wait bool) {
	panic("not called")
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
	panic("not called")
}

// metricSampleAPIType returns the APIMetricType of the given sample, the second
// return value informs the caller if the input type is supported by
// the no-aggregation pipeline: APIMetricType only supports gauges, counts and rates.
// This method will default on gauges for every other inputs and return false as a second return value.
func metricSampleAPIType(m metrics.MetricSample) (metrics.APIMetricType, bool) {
	panic("not called")
}
