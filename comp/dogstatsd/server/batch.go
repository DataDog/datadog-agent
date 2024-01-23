// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dogstatsd implements DogStatsD.
//
//nolint:revive // TODO(AML) Fix revive linter
package server

import (
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// batcher batches multiple metrics before submission
// this struct is not safe for concurrent use
type batcher struct {
	// slice of MetricSampleBatch (one entry per running sampling pipeline)
	samples []metrics.MetricSampleBatch
	// offset while writing into samples entries (i.e.  samples currently stored per pipeline)
	samplesCount []int

	// MetricSampleBatch used for metrics with timestamp
	// no multi-pipelines use for metrics with timestamp since we don't process them and we directly
	// send them to the serializer
	samplesWithTs metrics.MetricSampleBatch
	// offset while writing into the sample with timestampe slice (i.e. count of samples
	// with timestamp currently stored)
	samplesWithTsCount int

	events        []*event.Event
	serviceChecks []*servicecheck.ServiceCheck

	// output channels
	choutEvents        chan<- []*event.Event
	choutServiceChecks chan<- []*servicecheck.ServiceCheck

	metricSamplePool *metrics.MetricSamplePool

	demux aggregator.Demultiplexer
	// buffer slice allocated once per contextResolver to combine and sort
	// tags, origin detection tags and k8s tags.
	tagsBuffer    *tagset.HashingTagsAccumulator
	keyGenerator  *ckey.KeyGenerator
	pipelineCount int
	// the batcher has to know if the no-aggregation pipeline is enabled or not:
	// in the case of the no agg pipeline disabled, it would send them as usual to
	// the demux which only choice would be to send them on an arbitrary sampler
	// (i.e. the first one). Being aware that the no agg pipeline is disabled,
	// the batcher can decide to properly distribute these samples on the available
	// pipelines.
	noAggPipelineEnabled bool
}

// Use fastrange instead of a modulo for better performance.
// See http://lemire.me/blog/2016/06/27/a-fast-alternative-to-the-modulo-reduction/.
//
// Note that we shift the context key because it is an actual 64 bits, and
// the fast range has to operate on 32 bits values, so, we shift it in order
// to "reduce" its size to 32 bits (i.e. the `key>>32`), we don't mind using
// only half of the context key for the shard key, it will be unique enough
// for such purpose.
func fastrange(key ckey.ContextKey, pipelineCount int) uint32 {
	panic("not called")
}

func newBatcher(demux aggregator.DemultiplexerWithAggregator) *batcher {
	panic("not called")
}

func newServerlessBatcher(demux aggregator.Demultiplexer) *batcher {
	_, pipelineCount := aggregator.GetDogStatsDWorkerAndPipelineCount()
	samples := make([]metrics.MetricSampleBatch, pipelineCount)
	samplesCount := make([]int, pipelineCount)

	samplesWithTs := demux.GetMetricSamplePool().GetBatch()
	samplesWithTsCount := 0

	for i := range samples {
		samples[i] = demux.GetMetricSamplePool().GetBatch()
		samplesCount[i] = 0
	}

	return &batcher{
		samples:            samples,
		samplesCount:       samplesCount,
		samplesWithTs:      samplesWithTs,
		samplesWithTsCount: samplesWithTsCount,
		metricSamplePool:   demux.GetMetricSamplePool(),

		demux:         demux,
		pipelineCount: pipelineCount,
		tagsBuffer:    tagset.NewHashingTagsAccumulator(),
		keyGenerator:  ckey.NewKeyGenerator(),
	}
}

// Batching data
// -------------

func (b *batcher) appendSample(sample metrics.MetricSample) {
	var shardKey uint32
	if b.pipelineCount > 1 {
		// TODO(remy): re-using this tagsBuffer later in the pipeline (by sharing
		// it in the sample?) would reduce CPU usage, avoiding to recompute
		// the tags hashes while generating the context key.
		b.tagsBuffer.Append(sample.Tags...)
		// TODO: for better isolation we might want an option to user listenerID or origin here.
		// TODO: shuffle-sharding might be interesting too
		h := b.keyGenerator.Generate(sample.Name, sample.Host, b.tagsBuffer)
		b.tagsBuffer.Reset()
		shardKey = fastrange(h, b.pipelineCount)
	}

	if b.samplesCount[shardKey] >= len(b.samples[shardKey]) {
		b.flushSamples(shardKey)
	}

	b.samples[shardKey][b.samplesCount[shardKey]] = sample
	b.samplesCount[shardKey]++
}

func (b *batcher) appendEvent(event *event.Event) {
	panic("not called")
}

func (b *batcher) appendServiceCheck(serviceCheck *servicecheck.ServiceCheck) {
	panic("not called")
}

func (b *batcher) appendLateSample(sample metrics.MetricSample) {
	panic("not called")
}

// Flushing
// --------

func (b *batcher) flushSamples(shard uint32) {
	if b.samplesCount[shard] > 0 {
		t1 := time.Now()
		b.demux.AggregateSamples(aggregator.TimeSamplerID(shard), b.samples[shard][:b.samplesCount[shard]])
		t2 := time.Now()
		tlmChannel.Observe(float64(t2.Sub(t1).Nanoseconds()), strconv.Itoa(int(shard)), "metrics")

		b.samplesCount[shard] = 0
		b.samples[shard] = b.metricSamplePool.GetBatch()
	}
}

func (b *batcher) flushSamplesWithTs() {
	if b.samplesWithTsCount > 0 {
		t1 := time.Now()
		b.demux.SendSamplesWithoutAggregation(b.samplesWithTs[:b.samplesWithTsCount])
		t2 := time.Now()
		tlmChannel.Observe(float64(t2.Sub(t1).Nanoseconds()), "", "late_metrics")

		b.samplesWithTsCount = 0
		b.samplesWithTs = b.metricSamplePool.GetBatch()
	}
}

// flush pushes all batched metrics to the aggregator.
func (b *batcher) flush() {
	// flush all on-time samples on their respective time sampler
	for i := 0; i < b.pipelineCount; i++ {
		b.flushSamples(uint32(i))
	}

	// flush all samples with timestamp to the serializer
	b.flushSamplesWithTs()

	// flush events
	if len(b.events) > 0 {
		t1 := time.Now()
		b.choutEvents <- b.events
		t2 := time.Now()
		tlmChannel.Observe(float64(t2.Sub(t1).Nanoseconds()), "", "events")

		b.events = []*event.Event{}
	}

	// flush service checks
	if len(b.serviceChecks) > 0 {
		t1 := time.Now()
		b.choutServiceChecks <- b.serviceChecks
		t2 := time.Now()
		tlmChannel.Observe(float64(t2.Sub(t1).Nanoseconds()), "", "service_checks")

		b.serviceChecks = []*servicecheck.ServiceCheck{}
	}
}
