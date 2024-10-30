// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	orchestratorforwarder "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

// DemultiplexerWithAggregator is a Demultiplexer running an Aggregator.
// This flavor uses a AgentDemultiplexerOptions struct for startup configuration.
type DemultiplexerWithAggregator interface {
	Demultiplexer
	Aggregator() *BufferedAggregator
	// AggregateCheckSample adds check sample sent by a check from one of the collectors into a check sampler pipeline.
	AggregateCheckSample(sample metrics.MetricSample)
	Options() AgentDemultiplexerOptions
	GetEventPlatformForwarder() (eventplatform.Forwarder, error)
	GetEventsAndServiceChecksChannels() (chan []*event.Event, chan []*servicecheck.ServiceCheck)
	DumpDogstatsdContexts(io.Writer) error
}

// AgentDemultiplexer is the demultiplexer implementation for the main Agent.
type AgentDemultiplexer struct {
	log log.Component

	m sync.RWMutex

	// stopChan completely stops the flushLoop of the Demultiplexer when receiving
	// a message, not doing anything else.
	stopChan chan struct{}
	// flushChan receives a trigger to run an internal flush of all
	// samplers (TimeSampler, BufferedAggregator (CheckSampler, Events, ServiceChecks))
	// to the shared serializer.
	flushChan chan trigger

	// options are the options with which the demultiplexer has been created
	options    AgentDemultiplexerOptions
	aggregator *BufferedAggregator
	dataOutputs

	senders *senders

	hostTagProvider *HostTagProvider

	// sharded statsd time samplers
	statsd
}

// AgentDemultiplexerOptions are the options used to initialize a Demultiplexer.
type AgentDemultiplexerOptions struct {
	FlushInterval time.Duration

	EnableNoAggregationPipeline bool

	DontStartForwarders bool // unit tests don't need the forwarders to be instanciated

	UseDogstatsdContextLimiter bool
	DogstatsdMaxMetricsTags    int
}

// DefaultAgentDemultiplexerOptions returns the default options to initialize an AgentDemultiplexer.
func DefaultAgentDemultiplexerOptions() AgentDemultiplexerOptions {
	return AgentDemultiplexerOptions{
		FlushInterval: DefaultFlushInterval,
		// the different agents/binaries enable it on a per-need basis
		EnableNoAggregationPipeline: false,
	}
}

type statsd struct {
	// how many sharded statsdSamplers exists.
	// len(workers) would return the same result but having it stored
	// it will provide more explicit visiblility / no extra function call for
	// every metric to distribute.
	pipelinesCount int
	workers        []*timeSamplerWorker
	// shared metric sample pool between the dogstatsd server & the time sampler
	metricSamplePool *metrics.MetricSamplePool

	// the noAggregationStreamWorker is the one dealing with metrics that don't need to
	// be aggregated/sampled.
	noAggStreamWorker *noAggregationStreamWorker
}

type forwarders struct {
	containerLifecycle *forwarder.DefaultForwarder
}

type dataOutputs struct {
	forwarders       forwarders
	sharedSerializer serializer.MetricSerializer
	noAggSerializer  serializer.MetricSerializer
}

// InitAndStartAgentDemultiplexer creates a new Demultiplexer and runs what's necessary
// in goroutines. As of today, only the embedded BufferedAggregator needs a separate goroutine.
// In the future, goroutines will be started for the event platform forwarder and/or orchestrator forwarder.
func InitAndStartAgentDemultiplexer(
	log log.Component,
	sharedForwarder forwarder.Forwarder,
	orchestratorForwarder orchestratorforwarder.Component,
	options AgentDemultiplexerOptions,
	eventPlatformForwarder eventplatform.Component,
	compressor compression.Component,
	tagger tagger.Component,
	hostname string) *AgentDemultiplexer {
	demux := initAgentDemultiplexer(log, sharedForwarder, orchestratorForwarder, options, eventPlatformForwarder, compressor, tagger, hostname)
	go demux.run()
	return demux
}

func initAgentDemultiplexer(
	log log.Component,
	sharedForwarder forwarder.Forwarder,
	orchestratorForwarder orchestratorforwarder.Component,
	options AgentDemultiplexerOptions,
	eventPlatformForwarder eventplatform.Component,
	compressor compression.Component,
	tagger tagger.Component,
	hostname string) *AgentDemultiplexer {
	// prepare the multiple forwarders
	// -------------------------------
	if pkgconfigsetup.Datadog().GetBool("telemetry.enabled") && pkgconfigsetup.Datadog().GetBool("telemetry.dogstatsd_origin") && !pkgconfigsetup.Datadog().GetBool("aggregator_use_tags_store") {
		log.Warn("DogStatsD origin telemetry is not supported when aggregator_use_tags_store is disabled.")
		pkgconfigsetup.Datadog().Set("telemetry.dogstatsd_origin", false, model.SourceAgentRuntime)
	}

	// prepare the serializer
	// ----------------------

	sharedSerializer := serializer.NewSerializer(sharedForwarder, orchestratorForwarder, compressor, pkgconfigsetup.Datadog(), hostname)

	// prepare the embedded aggregator
	// --

	agg := NewBufferedAggregator(sharedSerializer, eventPlatformForwarder, tagger, hostname, options.FlushInterval)

	// statsd samplers
	// ---------------

	bufferSize := pkgconfigsetup.Datadog().GetInt("aggregator_buffer_size")
	metricSamplePool := metrics.NewMetricSamplePool(MetricSamplePoolBatchSize, utils.IsTelemetryEnabled(pkgconfigsetup.Datadog()))
	_, statsdPipelinesCount := GetDogStatsDWorkerAndPipelineCount()
	log.Debug("the Demultiplexer will use", statsdPipelinesCount, "pipelines")

	statsdWorkers := make([]*timeSamplerWorker, statsdPipelinesCount)

	for i := 0; i < statsdPipelinesCount; i++ {
		// the sampler
		tagsStore := tags.NewStore(pkgconfigsetup.Datadog().GetBool("aggregator_use_tags_store"), fmt.Sprintf("timesampler #%d", i))

		statsdSampler := NewTimeSampler(TimeSamplerID(i), bucketSize, tagsStore, tagger, agg.hostname)

		// its worker (process loop + flush/serialization mechanism)

		statsdWorkers[i] = newTimeSamplerWorker(statsdSampler, options.FlushInterval,
			bufferSize, metricSamplePool, agg.flushAndSerializeInParallel, tagsStore)
	}

	var noAggWorker *noAggregationStreamWorker
	var noAggSerializer serializer.MetricSerializer
	if options.EnableNoAggregationPipeline {
		noAggSerializer = serializer.NewSerializer(sharedForwarder, orchestratorForwarder, compressor, pkgconfigsetup.Datadog(), hostname)
		noAggWorker = newNoAggregationStreamWorker(
			pkgconfigsetup.Datadog().GetInt("dogstatsd_no_aggregation_pipeline_batch_size"),
			metricSamplePool,
			noAggSerializer,
			agg.flushAndSerializeInParallel,
			tagger,
		)
	}

	// --
	demux := &AgentDemultiplexer{
		log:       log,
		options:   options,
		stopChan:  make(chan struct{}),
		flushChan: make(chan trigger),

		// Input
		aggregator: agg,

		// Output
		dataOutputs: dataOutputs{
			forwarders: forwarders{},

			sharedSerializer: sharedSerializer,
			noAggSerializer:  noAggSerializer,
		},

		hostTagProvider: NewHostTagProvider(),
		senders:         newSenders(agg),

		// statsd time samplers
		statsd: statsd{
			pipelinesCount:    statsdPipelinesCount,
			workers:           statsdWorkers,
			metricSamplePool:  metricSamplePool,
			noAggStreamWorker: noAggWorker,
		},
	}

	return demux
}

// Options returns options used during the demux initialization.
func (d *AgentDemultiplexer) Options() AgentDemultiplexerOptions {
	return d.options
}

// AddAgentStartupTelemetry adds a startup event and count (in a DSD time sampler)
// to be sent on the next flush.
func (d *AgentDemultiplexer) AddAgentStartupTelemetry(agentVersion string) {
	if agentVersion != "" {
		d.AggregateSample(metrics.MetricSample{
			Name:       fmt.Sprintf("datadog.%s.started", d.aggregator.agentName),
			Value:      1,
			Tags:       d.aggregator.tags(true),
			Host:       d.aggregator.hostname,
			Mtype:      metrics.CountType,
			SampleRate: 1,
			Timestamp:  0,
		})

		if d.aggregator.hostname != "" {
			// Send startup event only when we have a valid hostname
			d.aggregator.eventIn <- event.Event{
				Text:           fmt.Sprintf("Version %s", agentVersion),
				SourceTypeName: "System",
				Host:           d.aggregator.hostname,
				EventType:      "Agent Startup",
			}
		}
	}
}

// run runs all demultiplexer parts
func (d *AgentDemultiplexer) run() {
	if !d.options.DontStartForwarders {
		d.log.Debugf("Starting forwarders")

		// container lifecycle forwarder
		if d.forwarders.containerLifecycle != nil {
			if err := d.forwarders.containerLifecycle.Start(); err != nil {
				d.log.Errorf("error starting container lifecycle forwarder: %v", err)
			}
		} else {
			d.log.Debug("not starting the container lifecycle forwarder")
		}

		d.log.Debug("Forwarders started")
	}

	for _, w := range d.statsd.workers {
		go w.run()
	}

	go d.aggregator.run()

	if d.noAggStreamWorker != nil {
		go d.noAggStreamWorker.run()
	}

	d.flushLoop() // this is the blocking call
}

func (d *AgentDemultiplexer) flushLoop() {
	var flushTicker <-chan time.Time
	if d.options.FlushInterval > 0 {
		flushTicker = time.NewTicker(d.options.FlushInterval).C
	} else {
		d.log.Debug("flushInterval set to 0: will never flush automatically")
	}

	for {
		select {
		// stop sequence
		case <-d.stopChan:
			return
		// manual flush sequence
		case trigger := <-d.flushChan:
			d.flushToSerializer(trigger.time, trigger.waitForSerializer)
			if trigger.blockChan != nil {
				trigger.blockChan <- struct{}{}
			}
		// automatic flush sequence
		case t := <-flushTicker:
			d.flushToSerializer(t, false)
		}
	}
}

// Stop stops the demultiplexer.
// Resources are released, the instance should not be used after a call to `Stop()`.
func (d *AgentDemultiplexer) Stop(flush bool) {
	timeout := pkgconfigsetup.Datadog().GetDuration("aggregator_stop_timeout") * time.Second

	if d.noAggStreamWorker != nil {
		d.noAggStreamWorker.stop(flush)
	}

	// do a manual complete flush then stop
	// stop all automatic flush & the mainloop,
	if flush {
		trigger := trigger{
			time:              time.Now(),
			blockChan:         make(chan struct{}),
			waitForSerializer: flush,
		}

		d.flushChan <- trigger
		select {
		case <-trigger.blockChan:
		case <-time.After(timeout):
			d.log.Errorf("flushing data on Stop() timed out")
		}
	}

	// stops the flushloop and makes sure no automatic flushes will happen anymore
	d.stopChan <- struct{}{}

	d.m.Lock()
	defer d.m.Unlock()

	// aggregated data
	for _, worker := range d.statsd.workers {
		worker.stop()
	}
	if d.aggregator != nil {
		d.aggregator.Stop()
	}
	d.aggregator = nil

	// forwarders

	if !d.options.DontStartForwarders {
		if d.dataOutputs.forwarders.containerLifecycle != nil {
			d.dataOutputs.forwarders.containerLifecycle.Stop()
			d.dataOutputs.forwarders.containerLifecycle = nil
		}
	}

	// misc

	d.dataOutputs.sharedSerializer = nil
	d.senders = nil
}

// ForceFlushToSerializer triggers the execution of a flush from all data of samplers
// and the BufferedAggregator to the serializer.
// Safe to call from multiple threads.
func (d *AgentDemultiplexer) ForceFlushToSerializer(start time.Time, waitForSerializer bool) {
	trigger := trigger{
		time:              start,
		waitForSerializer: waitForSerializer,
		blockChan:         make(chan struct{}),
	}
	d.flushChan <- trigger
	<-trigger.blockChan
}

// flushToSerializer flushes all data from the aggregator and time samplers
// to the serializer.
//
// Best practice is that this method is *only* called by the flushLoop routine.
// It technically works if called from outside of this routine, but beware of
// deadlocks with the parallel stream series implementation.
//
// This implementation is not flushing the TimeSampler and the BufferedAggregator
// concurrently because the IterableSeries is not thread safe / supporting concurrent usage.
// If one day a better (faster?) solution is needed, we could either consider:
// - to have an implementation of SendIterableSeries listening on multiple sinks in parallel, or,
// - to have a thread-safe implementation of the underlying `util.BufferedChan`.
func (d *AgentDemultiplexer) flushToSerializer(start time.Time, waitForSerializer bool) {
	d.m.RLock()
	defer d.m.RUnlock()

	if d.aggregator == nil {
		// NOTE(remy): we could consider flushing only the time samplers
		return
	}

	logPayloads := pkgconfigsetup.Datadog().GetBool("log_payloads")
	series, sketches := createIterableMetrics(d.aggregator.flushAndSerializeInParallel, d.sharedSerializer, logPayloads, false, d.hostTagProvider)
	metrics.Serialize(
		series,
		sketches,
		func(seriesSink metrics.SerieSink, sketchesSink metrics.SketchesSink) {
			// flush DogStatsD pipelines (statsd/time samplers)
			// ------------------------------------------------

			for _, worker := range d.statsd.workers {
				// order the flush to the time sampler, and wait, in a different routine
				t := flushTrigger{
					trigger: trigger{
						time:      start,
						blockChan: make(chan struct{}),
					},
					sketchesSink: sketchesSink,
					seriesSink:   seriesSink,
				}

				worker.flushChan <- t
				<-t.trigger.blockChan
			}

			// flush the aggregator (check samplers)
			// -------------------------------------

			if d.aggregator != nil {
				t := flushTrigger{
					trigger: trigger{
						time:              start,
						blockChan:         make(chan struct{}),
						waitForSerializer: waitForSerializer,
					},
					sketchesSink: sketchesSink,
					seriesSink:   seriesSink,
				}

				d.aggregator.flushChan <- t
				<-t.trigger.blockChan
			}
		}, func(serieSource metrics.SerieSource) {
			sendIterableSeries(d.sharedSerializer, start, serieSource)
		},
		func(sketches metrics.SketchesSource) {
			// Don't send empty sketches payloads
			if sketches.WaitForValue() {
				err := d.sharedSerializer.SendSketch(sketches)
				sketchesCount := sketches.Count()
				d.log.Debugf("Flushing %d sketches to the serializer", sketchesCount)
				updateSketchTelemetry(start, sketchesCount, err)
				addFlushCount("Sketches", int64(sketchesCount))
			}
		})

	addFlushTime("MainFlushTime", int64(time.Since(start)))
	aggregatorNumberOfFlush.Add(1)
}

// GetEventsAndServiceChecksChannels returneds underlying events and service checks channels.
func (d *AgentDemultiplexer) GetEventsAndServiceChecksChannels() (chan []*event.Event, chan []*servicecheck.ServiceCheck) {
	return d.aggregator.GetBufferedChannels()
}

// GetEventPlatformForwarder returns underlying events and service checks channels.
func (d *AgentDemultiplexer) GetEventPlatformForwarder() (eventplatform.Forwarder, error) {
	return d.aggregator.GetEventPlatformForwarder()
}

// SendSamplesWithoutAggregation buffers a bunch of metrics with timestamp. This data will be directly
// transmitted "as-is" (i.e. no aggregation, no sampling) to the serializer.
func (d *AgentDemultiplexer) SendSamplesWithoutAggregation(samples metrics.MetricSampleBatch) {
	// safe-guard: if for some reasons we are receiving some metrics here despite
	// having the no-aggregation pipeline disabled, they are redirected to the first
	// time sampler.
	if !d.options.EnableNoAggregationPipeline {
		d.AggregateSamples(TimeSamplerID(0), samples)
		return
	}

	tlmProcessed.Add(float64(len(samples)), "", "late_metrics")
	d.statsd.noAggStreamWorker.addSamples(samples)
}

// AggregateSamples adds a batch of MetricSample into the given DogStatsD time sampler shard.
// If you have to submit a single metric sample see `AggregateSample`.
func (d *AgentDemultiplexer) AggregateSamples(shard TimeSamplerID, samples metrics.MetricSampleBatch) {
	// distribute the samples on the different statsd samplers using a channel
	// (in the time sampler implementation) for latency reasons:
	// its buffering + the fact that it is another goroutine processing the samples,
	// it should get back to the caller as fast as possible once the samples are
	// in the channel.
	d.statsd.workers[shard].samplesChan <- samples
}

// AggregateSample adds a MetricSample in the first DogStatsD time sampler.
func (d *AgentDemultiplexer) AggregateSample(sample metrics.MetricSample) {
	batch := d.GetMetricSamplePool().GetBatch()
	batch[0] = sample
	d.statsd.workers[0].samplesChan <- batch[:1]
}

// AggregateCheckSample adds check sample sent by a check from one of the collectors into a check sampler pipeline.
//
//nolint:revive // TODO(AML) Fix revive linter
func (d *AgentDemultiplexer) AggregateCheckSample(_ metrics.MetricSample) {
	panic("not implemented yet.")
}

// GetDogStatsDPipelinesCount returns how many sampling pipeline are running for
// the DogStatsD samples.
func (d *AgentDemultiplexer) GetDogStatsDPipelinesCount() int {
	return d.statsd.pipelinesCount
}

// Serializer returns a serializer that anyone can use. This method exists
// to keep compatibility with existing code while introducing the Demultiplexer,
// however, the plan is to remove it anytime soon.
//
// Deprecated.
func (d *AgentDemultiplexer) Serializer() serializer.MetricSerializer {
	return d.dataOutputs.sharedSerializer
}

// Aggregator returns an aggregator that anyone can use. This method exists
// to keep compatibility with existing code while introducing the Demultiplexer,
// however, the plan is to remove it anytime soon.
//
// Deprecated.
func (d *AgentDemultiplexer) Aggregator() *BufferedAggregator {
	return d.aggregator
}

// GetMetricSamplePool returns a shared resource used in the whole DogStatsD
// pipeline to re-use metric samples slices: the server is getting a slice
// and filling it with samples, the rest of the pipeline process them the
// end of line (the time sampler) is putting back the slice in the pool.
// Main idea is to reduce the garbage generated by slices allocation.
func (d *AgentDemultiplexer) GetMetricSamplePool() *metrics.MetricSamplePool {
	return d.statsd.metricSamplePool
}

// DumpDogstatsdContexts writes the current state of the context resolver to dest.
//
// This blocks metrics processing, so dest is expected to be reasonably fast and not block for too
// long.
func (d *AgentDemultiplexer) DumpDogstatsdContexts(dest io.Writer) error {
	for _, w := range d.statsd.workers {
		err := w.dumpContexts(dest)
		if err != nil {
			return err
		}
	}

	return nil
}

// GetSender returns a sender.Sender with passed ID, properly registered with the aggregator
// If no error is returned here, DestroySender must be called with the same ID
// once the sender is not used anymore
func (d *AgentDemultiplexer) GetSender(id checkid.ID) (sender.Sender, error) {
	d.m.RLock()
	defer d.m.RUnlock()

	if d.senders == nil {
		return nil, errors.New("demultiplexer is stopped")
	}

	return d.senders.GetSender(id)
}

// SetSender returns the passed sender with the passed ID.
// This is largely for testing purposes
func (d *AgentDemultiplexer) SetSender(s sender.Sender, id checkid.ID) error {
	d.m.RLock()
	defer d.m.RUnlock()
	if d.senders == nil {
		return errors.New("demultiplexer is stopped")
	}

	return d.senders.SetSender(s, id)
}

// DestroySender frees up the resources used by the sender with passed ID (by deregistering it from the aggregator)
// Should be called when no sender with this ID is used anymore
// The metrics of this (these) sender(s) that haven't been flushed yet will be lost
func (d *AgentDemultiplexer) DestroySender(id checkid.ID) {
	d.m.RLock()
	defer d.m.RUnlock()

	if d.senders == nil {
		return
	}

	d.senders.DestroySender(id)
}

// GetDefaultSender returns a default sender.
func (d *AgentDemultiplexer) GetDefaultSender() (sender.Sender, error) {
	d.m.RLock()
	defer d.m.RUnlock()

	if d.senders == nil {
		return nil, errors.New("demultiplexer is stopped")
	}

	return d.senders.GetDefaultSender()
}
