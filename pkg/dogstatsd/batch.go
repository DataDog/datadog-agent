// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// batcher batches multiple metrics before submission
// this struct is not safe for concurrent use
type batcher struct {
	samples      [][]metrics.MetricSample
	samplesCount []int

	events        []*metrics.Event
	serviceChecks []*metrics.ServiceCheck

	// output channels
	choutEvents        chan<- []*metrics.Event
	choutServiceChecks chan<- []*metrics.ServiceCheck

	metricSamplePool *metrics.MetricSamplePool

	demux aggregator.Demultiplexer
	// buffer slice allocated once per contextResolver to combine and sort
	// tags, origin detection tags and k8s tags.
	tagsBuffer    *tagset.HashingTagsAccumulator
	keyGenerator  *ckey.KeyGenerator
	pipelineCount int
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
	return uint32((uint64(key>>32) * uint64(pipelineCount)) >> 32)
}

func newBatcher(demux aggregator.Demultiplexer) *batcher {
	agg := demux.Aggregator()

	pipelineCount := demux.GetDogStatsDPipelinesCount()

	e, sc := agg.GetBufferedChannels()
	samples := make([][]metrics.MetricSample, pipelineCount)
	samplesCount := make([]int, pipelineCount)

	for i := range samples {
		samples[i] = demux.GetMetricSamplePool().GetBatch()
		samplesCount[i] = 0
	}

	return &batcher{
		samples:            samples,
		samplesCount:       samplesCount,
		metricSamplePool:   demux.GetMetricSamplePool(),
		choutEvents:        e,
		choutServiceChecks: sc,

		demux:         demux,
		pipelineCount: pipelineCount,
		tagsBuffer:    tagset.NewHashingTagsAccumulator(),
		keyGenerator:  ckey.NewKeyGenerator(),
	}
}

func (b *batcher) appendSample(sample metrics.MetricSample) {
	var shardKey uint32
	if b.pipelineCount > 1 {
		// TODO(remy): re-using this tagsBuffer later in the pipeline (by sharing
		// it in the sample?) would reduce CPU usage, avoiding to recompute
		// the tags hashes while generating the context key.
		b.tagsBuffer.Append(sample.Tags...)
		h := b.keyGenerator.Generate(sample.Name, sample.Host, b.tagsBuffer)
		b.tagsBuffer.Reset()
		shardKey = fastrange(h, b.pipelineCount)
	}

	if b.samplesCount[shardKey] == len(b.samples[shardKey]) {
		b.flushSamples(shardKey)
	}

	b.samples[shardKey][b.samplesCount[shardKey]] = sample
	b.samplesCount[shardKey]++
}

func (b *batcher) appendEvent(event *metrics.Event) {
	b.events = append(b.events, event)
}

func (b *batcher) appendServiceCheck(serviceCheck *metrics.ServiceCheck) {
	b.serviceChecks = append(b.serviceChecks, serviceCheck)
}

func (b *batcher) flushSamples(shard uint32) {
	if b.samplesCount[shard] > 0 {
		t1 := time.Now()
		b.demux.AddTimeSampleBatch(aggregator.TimeSamplerID(shard), b.samples[shard][:b.samplesCount[shard]])
		t2 := time.Now()
		tlmChannel.Observe(float64(t2.Sub(t1).Nanoseconds()), "metrics")

		b.samplesCount[shard] = 0
		b.samples[shard] = b.metricSamplePool.GetBatch()
	}
}

// flush pushes all batched metrics to the aggregator.
func (b *batcher) flush() {
	for i := 0; i < b.pipelineCount; i++ {
		b.flushSamples(uint32(i))
	}

	if len(b.events) > 0 {
		t1 := time.Now()
		b.choutEvents <- b.events
		t2 := time.Now()
		tlmChannel.Observe(float64(t2.Sub(t1).Nanoseconds()), "events")

		b.events = []*metrics.Event{}
	}
	if len(b.serviceChecks) > 0 {
		t1 := time.Now()
		b.choutServiceChecks <- b.serviceChecks
		t2 := time.Now()
		tlmChannel.Observe(float64(t2.Sub(t1).Nanoseconds()), "service_checks")

		b.serviceChecks = []*metrics.ServiceCheck{}
	}
}
