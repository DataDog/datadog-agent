// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"context"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logimpl "github.com/DataDog/datadog-agent/comp/core/log/impl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

// ServerlessDemultiplexer is a simple demultiplexer used by the serverless flavor of the Agent
type ServerlessDemultiplexer struct {
	log log.Component
	// shared metric sample pool between the dogstatsd server & the time sampler
	metricSamplePool *metrics.MetricSamplePool

	serializer    *serializer.Serializer
	forwarder     *forwarder.SyncForwarder
	statsdSampler *TimeSampler
	statsdWorker  *timeSamplerWorker

	flushLock *sync.Mutex

	flushAndSerializeInParallel FlushAndSerializeInParallel

	hostTagProvider *HostTagProvider

	*senders
}

// InitAndStartServerlessDemultiplexer creates and starts new Demultiplexer for the serverless agent.
func InitAndStartServerlessDemultiplexer(keysPerDomain map[string][]string, forwarderTimeout time.Duration, tagger tagger.Component) *ServerlessDemultiplexer {
	bufferSize := pkgconfigsetup.Datadog().GetInt("aggregator_buffer_size")
	logger := logimpl.NewTemporaryLoggerWithoutInit()
	forwarder := forwarder.NewSyncForwarder(pkgconfigsetup.Datadog(), logger, keysPerDomain, forwarderTimeout)
	h, _ := hostname.Get(context.Background())
	serializer := serializer.NewSerializer(forwarder, nil, compressionimpl.NewCompressor(pkgconfigsetup.Datadog()), pkgconfigsetup.Datadog(), h)
	metricSamplePool := metrics.NewMetricSamplePool(MetricSamplePoolBatchSize, utils.IsTelemetryEnabled(pkgconfigsetup.Datadog()))
	tagsStore := tags.NewStore(pkgconfigsetup.Datadog().GetBool("aggregator_use_tags_store"), "timesampler")

	statsdSampler := NewTimeSampler(TimeSamplerID(0), bucketSize, tagsStore, tagger, "")
	flushAndSerializeInParallel := NewFlushAndSerializeInParallel(pkgconfigsetup.Datadog())
	statsdWorker := newTimeSamplerWorker(statsdSampler, DefaultFlushInterval, bufferSize, metricSamplePool, flushAndSerializeInParallel, tagsStore)

	demux := &ServerlessDemultiplexer{
		log:                         logger,
		forwarder:                   forwarder,
		statsdSampler:               statsdSampler,
		statsdWorker:                statsdWorker,
		serializer:                  serializer,
		metricSamplePool:            metricSamplePool,
		flushLock:                   &sync.Mutex{},
		hostTagProvider:             NewHostTagProvider(),
		flushAndSerializeInParallel: flushAndSerializeInParallel,
	}

	// start routines
	go demux.Run()

	// we're done with the initialization
	return demux
}

// Run runs all demultiplexer parts
func (d *ServerlessDemultiplexer) Run() {
	if d.forwarder != nil {
		d.forwarder.Start() //nolint:errcheck
		d.log.Debug("Forwarder started")
	} else {
		d.log.Debug("not starting the forwarder")
	}

	d.log.Debug("Demultiplexer started")
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

	logPayloads := pkgconfigsetup.Datadog().GetBool("log_payloads")
	series, sketches := createIterableMetrics(d.flushAndSerializeInParallel, d.serializer, logPayloads, true, d.hostTagProvider)

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
//
//nolint:revive // TODO(AML) Fix revive linter
func (d *ServerlessDemultiplexer) AggregateSamples(_ TimeSamplerID, samples metrics.MetricSampleBatch) {
	d.flushLock.Lock()
	defer d.flushLock.Unlock()
	d.statsdWorker.samplesChan <- samples
}

// SendSamplesWithoutAggregation is not supported in the Serverless Agent implementation.
//
//nolint:revive // TODO(AML) Fix revive linter
func (d *ServerlessDemultiplexer) SendSamplesWithoutAggregation(_ metrics.MetricSampleBatch) {
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
