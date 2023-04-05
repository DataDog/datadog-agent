// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ServerlessDemultiplexer is a simple demultiplexer used by the serverless flavor of the Agent
type ServerlessDemultiplexer struct {
	// shared metric sample pool between the dogstatsd server & the time sampler
	metricSamplePool *metrics.MetricSamplePool

	serializer    *serializer.Serializer
	forwarder     *forwarder.SyncForwarder
	statsdSampler *TimeSampler
	statsdWorker  *timeSamplerWorker

	flushLock *sync.Mutex

	flushAndSerializeInParallel FlushAndSerializeInParallel

	*senders
}

// InitAndStartServerlessDemultiplexer creates and starts new Demultiplexer for the serverless agent.
func InitAndStartServerlessDemultiplexer(domainResolvers map[string]resolver.DomainResolver, forwarderTimeout time.Duration) *ServerlessDemultiplexer {
	bufferSize := config.Datadog.GetInt("aggregator_buffer_size")
	forwarder := forwarder.NewSyncForwarder(domainResolvers, forwarderTimeout)
	serializer := serializer.NewSerializer(forwarder, nil)
	metricSamplePool := metrics.NewMetricSamplePool(MetricSamplePoolBatchSize)
	tagsStore := tags.NewStore(config.Datadog.GetBool("aggregator_use_tags_store"), "timesampler")

	statsdSampler := NewTimeSampler(TimeSamplerID(0), bucketSize, tagsStore, "")
	flushAndSerializeInParallel := NewFlushAndSerializeInParallel(config.Datadog)
	statsdWorker := newTimeSamplerWorker(statsdSampler, DefaultFlushInterval, bufferSize, metricSamplePool, flushAndSerializeInParallel, tagsStore)

	demux := &ServerlessDemultiplexer{
		forwarder:        forwarder,
		statsdSampler:    statsdSampler,
		statsdWorker:     statsdWorker,
		serializer:       serializer,
		metricSamplePool: metricSamplePool,
		flushLock:        &sync.Mutex{},

		flushAndSerializeInParallel: flushAndSerializeInParallel,
	}

	// set the global instance
	demultiplexerInstance = demux

	// start routines
	go demux.Run()

	// we're done with the initialization
	return demux
}

// Run runs all demultiplexer parts
func (d *ServerlessDemultiplexer) Run() {
	if d.forwarder != nil {
		d.forwarder.Start() //nolint:errcheck
		log.Debug("Forwarder started")
	} else {
		log.Debug("not starting the forwarder")
	}

	log.Debug("Demultiplexer started")
	d.statsdWorker.run()
}

// Stop stops the wrapped aggregator and the forwarder.
func (d *ServerlessDemultiplexer) Stop(flush bool) {
	if flush {
		d.ForceFlushToSerializer(time.Now(), true)
	}

	d.statsdWorker.stop()

	if d.forwarder != nil {
		d.forwarder.Stop()
	}
}

// ForceFlushToSerializer flushes all data from the time sampler to the serializer.
func (d *ServerlessDemultiplexer) ForceFlushToSerializer(start time.Time, waitForSerializer bool) {
	d.flushLock.Lock()
	defer d.flushLock.Unlock()

	logPayloads := config.Datadog.GetBool("log_payloads")
	series, sketches := createIterableMetrics(d.flushAndSerializeInParallel, d.serializer, logPayloads, true)

	metrics.Serialize(
		series,
		sketches,
		func(seriesSink metrics.SerieSink, sketchesSink metrics.SketchesSink) {
			trigger := flushTrigger{
				trigger: trigger{
					time:              start,
					blockChan:         make(chan struct{}),
					waitForSerializer: waitForSerializer,
				},
				sketchesSink: sketchesSink,
				seriesSink:   seriesSink,
			}

			d.statsdWorker.flushChan <- trigger
			<-trigger.blockChan
		}, func(serieSource metrics.SerieSource) {
			sendIterableSeries(d.serializer, start, serieSource)
		}, func(sketches metrics.SketchesSource) {
			// Don't send empty sketches payloads
			if sketches.WaitForValue() {
				d.serializer.SendSketch(sketches) //nolint:errcheck
			}
		})
}

// AggregateSample send a MetricSample to the TimeSampler.
func (d *ServerlessDemultiplexer) AggregateSample(sample metrics.MetricSample) {
	d.flushLock.Lock()
	defer d.flushLock.Unlock()
	batch := d.GetMetricSamplePool().GetBatch()
	batch[0] = sample
	d.statsdWorker.samplesChan <- batch[:1]
}

// AggregateSamples send a MetricSampleBatch to the TimeSampler.
// The ServerlessDemultiplexer is not using sharding in its DogStatsD pipeline,
// the `shard` parameter is ignored.
// In the Serverless Agent, consider using `AggregateSample` instead.
func (d *ServerlessDemultiplexer) AggregateSamples(shard TimeSamplerID, samples metrics.MetricSampleBatch) {
	d.flushLock.Lock()
	defer d.flushLock.Unlock()
	d.statsdWorker.samplesChan <- samples
}

// SendSamplesWithoutAggregation is not supported in the Serverless Agent implementation.
func (d *ServerlessDemultiplexer) SendSamplesWithoutAggregation(samples metrics.MetricSampleBatch) {
	panic("not implemented.")
}

// Serializer returns the shared serializer
func (d *ServerlessDemultiplexer) Serializer() serializer.MetricSerializer {
	return d.serializer
}

// GetMetricSamplePool returns a shared resource used in the whole DogStatsD
// pipeline to re-use metric samples slices: the server is getting a slice
// and filling it with samples, the rest of the pipeline process them the
// end of line (the time sampler) is putting back the slice in the pool.
// Main idea is to reduce the garbage generated by slices allocation.
func (d *ServerlessDemultiplexer) GetMetricSamplePool() *metrics.MetricSamplePool {
	return d.metricSamplePool
}
