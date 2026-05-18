// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dogstatsd implements DogStatsD.
//
//nolint:revive // TODO(AML) Fix revive linter
package serverimpl

import (
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/identity"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// interface requiring all functions expected by the dogstatsd server
type dogstatsdBatcher interface {
	appendSample(sample metrics.MetricSample)
	appendSampleWithContext(sample metrics.MetricSample, context identity.HotPathContext)
	appendEvent(event *event.Event)
	appendServiceCheck(serviceCheck *servicecheck.ServiceCheck)
	appendLateSample(sample metrics.MetricSample)
	appendLateSampleWithContext(sample metrics.MetricSample, context identity.HotPathContext)
	appendColumnarV3SampleWithContext(sample metrics.MetricSample, context identity.HotPathContext)
	needsSampleContext() bool
	flush()
}

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

	columnarV3             aggregator.DogStatsDColumnarV3Inserter
	columnarV3Samples      []aggregator.DogStatsDColumnarV3SampleBatch
	columnarV3SamplesCount []int
	columnarV3SamplePool   *aggregator.DogStatsDColumnarV3SamplePool

	demux aggregator.Demultiplexer
	// buffer slice allocated once per contextResolver to combine and sort
	// tags, origin detection tags and k8s tags.
	shardGenerator shardKeyGenerator

	pipelineCount int
	// the batcher has to know if the no-aggregation pipeline is enabled or not:
	// in the case of the no agg pipeline disabled, it would send them as usual to
	// the demux which only choice would be to send them on an arbitrary sampler
	// (i.e. the first one). Being aware that the no agg pipeline is disabled,
	// the batcher can decide to properly distribute these samples on the available
	// pipelines.
	noAggPipelineEnabled bool

	// telemetry
	tlmChannel telemetry.Histogram
}

type shardKeyGenerator struct {
	keyGenerator *ckey.KeyGenerator
	tagsBuffer   *tagset.HashingTagsAccumulator
}

func (s *shardKeyGenerator) Generate(sample metrics.MetricSample, shards int) uint32 {
	// TODO(remy): re-using this tagsBuffer later in the pipeline (by sharing
	// it in the sample?) would reduce CPU usage, avoiding to recompute
	// the tags hashes while generating the context key.
	s.tagsBuffer.Append(sample.Tags...)
	h := s.keyGenerator.Generate(sample.Name, sample.Host, s.tagsBuffer)
	s.tagsBuffer.Reset()
	return fastrange(h, shards)
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
	// return uint32(uint64(key) % uint64(pipelineCount))
	return identity.ShardIndex(key, pipelineCount)
}

func newBatcher(demux aggregator.DemultiplexerWithAggregator, tlmChannel telemetry.Histogram) *batcher {
	_, pipelineCount := aggregator.GetDogStatsDWorkerAndPipelineCount()

	var e chan []*event.Event
	var sc chan []*servicecheck.ServiceCheck

	// the Serverless Agent doesn't have to support service checks nor events so
	// it doesn't run an Aggregator.
	e, sc = demux.GetEventsAndServiceChecksChannels()

	// prepare on-time samples buffers
	samples := make([]metrics.MetricSampleBatch, pipelineCount)
	samplesCount := make([]int, pipelineCount)

	for i := range samples {
		samples[i] = demux.GetMetricSamplePool().GetBatch()
		samplesCount[i] = 0
	}

	// prepare the late samples buffer
	samplesWithTs := demux.GetMetricSamplePool().GetBatch()
	samplesWithTsCount := 0

	var columnarV3 aggregator.DogStatsDColumnarV3Inserter
	var columnarV3Samples []aggregator.DogStatsDColumnarV3SampleBatch
	var columnarV3SamplesCount []int
	var columnarV3SamplePool *aggregator.DogStatsDColumnarV3SamplePool
	if inserter, ok := demux.(aggregator.DogStatsDColumnarV3Inserter); ok && inserter.DogStatsDColumnarV3Enabled() {
		columnarV3SamplePool = inserter.GetDogStatsDColumnarV3SamplePool()
		if columnarV3SamplePool != nil {
			columnarV3 = inserter
			columnarV3Samples = make([]aggregator.DogStatsDColumnarV3SampleBatch, pipelineCount)
			columnarV3SamplesCount = make([]int, pipelineCount)
			for i := range columnarV3Samples {
				columnarV3Samples[i] = columnarV3SamplePool.GetBatch()
			}
		}
	}

	return &batcher{
		samples:            samples,
		samplesCount:       samplesCount,
		samplesWithTs:      samplesWithTs,
		samplesWithTsCount: samplesWithTsCount,
		metricSamplePool:   demux.GetMetricSamplePool(),
		choutEvents:        e,
		choutServiceChecks: sc,

		columnarV3:             columnarV3,
		columnarV3Samples:      columnarV3Samples,
		columnarV3SamplesCount: columnarV3SamplesCount,
		columnarV3SamplePool:   columnarV3SamplePool,

		demux:          demux,
		pipelineCount:  pipelineCount,
		shardGenerator: newShardGenerator(),

		noAggPipelineEnabled: demux.Options().EnableNoAggregationPipeline,
		tlmChannel:           tlmChannel,
	}
}

func newShardGenerator() shardKeyGenerator {
	return shardKeyGenerator{
		keyGenerator: ckey.NewKeyGenerator(),
		tagsBuffer:   tagset.NewHashingTagsAccumulator(),
	}
}

func newServerlessBatcher(demux aggregator.Demultiplexer, tlmChannel telemetry.Histogram) *batcher {
	_, pipelineCount := aggregator.GetDogStatsDWorkerAndPipelineCount()
	samples := make([]metrics.MetricSampleBatch, pipelineCount)
	samplesCount := make([]int, pipelineCount)

	samplesWithTs := demux.GetMetricSamplePool().GetBatch()
	samplesWithTsCount := 0

	for i := range samples {
		samples[i] = demux.GetMetricSamplePool().GetBatch()
		samplesCount[i] = 0
	}

	var columnarV3 aggregator.DogStatsDColumnarV3Inserter
	var columnarV3Samples []aggregator.DogStatsDColumnarV3SampleBatch
	var columnarV3SamplesCount []int
	var columnarV3SamplePool *aggregator.DogStatsDColumnarV3SamplePool
	if inserter, ok := demux.(aggregator.DogStatsDColumnarV3Inserter); ok && inserter.DogStatsDColumnarV3Enabled() {
		columnarV3SamplePool = inserter.GetDogStatsDColumnarV3SamplePool()
		if columnarV3SamplePool != nil {
			columnarV3 = inserter
			columnarV3Samples = make([]aggregator.DogStatsDColumnarV3SampleBatch, pipelineCount)
			columnarV3SamplesCount = make([]int, pipelineCount)
			for i := range columnarV3Samples {
				columnarV3Samples[i] = columnarV3SamplePool.GetBatch()
			}
		}
	}

	return &batcher{
		samples:            samples,
		samplesCount:       samplesCount,
		samplesWithTs:      samplesWithTs,
		samplesWithTsCount: samplesWithTsCount,
		metricSamplePool:   demux.GetMetricSamplePool(),

		columnarV3:             columnarV3,
		columnarV3Samples:      columnarV3Samples,
		columnarV3SamplesCount: columnarV3SamplesCount,
		columnarV3SamplePool:   columnarV3SamplePool,

		demux:          demux,
		pipelineCount:  pipelineCount,
		shardGenerator: newShardGenerator(),
		tlmChannel:     tlmChannel,
	}
}

// Batching data
// -------------

func (b *batcher) appendSample(sample metrics.MetricSample) {
	var shardKey uint32
	if b.pipelineCount > 1 {
		shardKey = b.shardGenerator.Generate(sample, b.pipelineCount)
	}
	b.appendSampleToShard(sample, shardKey)
}

func (b *batcher) appendSampleWithContext(sample metrics.MetricSample, context identity.HotPathContext) {
	var shardKey uint32
	if b.pipelineCount > 1 {
		shardKey = identity.ShardIndex(context.Shard.ContextKey, b.pipelineCount)
	}
	b.appendSampleToShard(sample, shardKey)
}

func (b *batcher) appendSampleToShard(sample metrics.MetricSample, shardKey uint32) {
	if b.samplesCount[shardKey] >= len(b.samples[shardKey]) {
		b.flushSamples(shardKey)
	}

	b.samples[shardKey][b.samplesCount[shardKey]] = sample
	b.samplesCount[shardKey]++
}

func (b *batcher) appendEvent(event *event.Event) {
	b.events = append(b.events, event)
}

func (b *batcher) appendServiceCheck(serviceCheck *servicecheck.ServiceCheck) {
	b.serviceChecks = append(b.serviceChecks, serviceCheck)
}

func (b *batcher) appendLateSample(sample metrics.MetricSample) {
	// if the no aggregation pipeline is not enabled, we fallback on the
	// main pipeline eventually distributing the samples on multiple samplers.
	if !b.noAggPipelineEnabled {
		b.appendSample(sample)
		return
	}

	b.appendLateSampleWithoutAggregation(sample)
}

func (b *batcher) appendLateSampleWithContext(sample metrics.MetricSample, context identity.HotPathContext) {
	// if the no aggregation pipeline is not enabled, we fallback on the
	// main pipeline eventually distributing the samples on multiple samplers.
	if !b.noAggPipelineEnabled {
		b.appendSampleWithContext(sample, context)
		return
	}

	b.appendLateSampleWithoutAggregation(sample)
}

func (b *batcher) appendLateSampleWithoutAggregation(sample metrics.MetricSample) {
	if b.samplesWithTsCount == len(b.samplesWithTs) {
		b.flushSamplesWithTs()
	}

	b.samplesWithTs[b.samplesWithTsCount] = sample
	b.samplesWithTsCount++
}

func (b *batcher) appendColumnarV3SampleWithContext(sample metrics.MetricSample, context identity.HotPathContext) {
	if b.columnarV3 == nil || b.columnarV3SamplePool == nil {
		b.appendSampleWithContext(sample, context)
		return
	}
	var shardKey uint32
	if b.pipelineCount > 1 {
		shardKey = identity.ShardIndex(context.Shard.ContextKey, b.pipelineCount)
	}
	if b.columnarV3SamplesCount[shardKey] >= len(b.columnarV3Samples[shardKey]) {
		b.flushColumnarV3Samples(shardKey)
	}
	b.columnarV3Samples[shardKey][b.columnarV3SamplesCount[shardKey]] = aggregator.DogStatsDColumnarV3Sample{
		ContextKey: context.Shard.ContextKey,
		Sample:     sample,
	}
	b.columnarV3SamplesCount[shardKey]++
}

func (b *batcher) needsSampleContext() bool {
	return b.pipelineCount > 1
}

// Flushing
// --------

func (b *batcher) flushSamples(shard uint32) {
	if b.samplesCount[shard] > 0 {
		t1 := time.Now()
		b.demux.AggregateSamples(aggregator.TimeSamplerID(shard), b.samples[shard][:b.samplesCount[shard]])
		t2 := time.Now()
		b.tlmChannel.Observe(float64(t2.Sub(t1).Nanoseconds()), strconv.Itoa(int(shard)), "metrics")

		b.samplesCount[shard] = 0
		b.samples[shard] = b.metricSamplePool.GetBatch()
	}
}

func (b *batcher) flushSamplesWithTs() {
	if b.samplesWithTsCount > 0 {
		t1 := time.Now()
		b.demux.SendSamplesWithoutAggregation(b.samplesWithTs[:b.samplesWithTsCount])
		t2 := time.Now()
		b.tlmChannel.Observe(float64(t2.Sub(t1).Nanoseconds()), "", "late_metrics")

		b.samplesWithTsCount = 0
		b.samplesWithTs = b.metricSamplePool.GetBatch()
	}
}

func (b *batcher) flushColumnarV3Samples(shard uint32) {
	if b.columnarV3 == nil || b.columnarV3SamplesCount[shard] == 0 {
		return
	}
	t1 := time.Now()
	b.columnarV3.AggregateDogStatsDColumnarV3Samples(aggregator.TimeSamplerID(shard), b.columnarV3Samples[shard][:b.columnarV3SamplesCount[shard]])
	t2 := time.Now()
	b.tlmChannel.Observe(float64(t2.Sub(t1).Nanoseconds()), strconv.Itoa(int(shard)), "columnar_v3_metrics")

	b.columnarV3SamplesCount[shard] = 0
	b.columnarV3Samples[shard] = b.columnarV3SamplePool.GetBatch()
}

// flush pushes all batched metrics to the aggregator.
func (b *batcher) flush() {
	// flush all on-time samples on their respective time sampler
	for i := 0; i < b.pipelineCount; i++ {
		b.flushSamples(uint32(i))
		b.flushColumnarV3Samples(uint32(i))
	}

	// flush all samples with timestamp to the serializer
	b.flushSamplesWithTs()

	// flush events
	if len(b.events) > 0 {
		if b.choutEvents != nil {
			t1 := time.Now()
			b.choutEvents <- b.events
			t2 := time.Now()
			b.tlmChannel.Observe(float64(t2.Sub(t1).Nanoseconds()), "", "events")
		} else {
			log.Debugf("Skipping event flush due to nil channel")
		}

		b.events = []*event.Event{}
	}

	// flush service checks
	if len(b.serviceChecks) > 0 {
		if b.choutServiceChecks != nil {
			t1 := time.Now()
			b.choutServiceChecks <- b.serviceChecks
			t2 := time.Now()
			b.tlmChannel.Observe(float64(t2.Sub(t1).Nanoseconds()), "", "service_checks")
		} else {
			log.Debugf("Skipping service check flush due to nil channel")
		}

		b.serviceChecks = []*servicecheck.ServiceCheck{}
	}
}
