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
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	orchestratorforwarder "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	compression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/hosttags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
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
	// a message, not doing anything else. Passing a non-nil trigger will perform
	// a final flush.
	stopChan chan *trigger
	// flushChan receives a trigger to run an internal flush of all
	// samplers (TimeSampler, BufferedAggregator (CheckSampler, Events, ServiceChecks))
	// to the shared serializer.
	flushChan chan trigger

	// options are the options with which the demultiplexer has been created
	options    AgentDemultiplexerOptions
	aggregator *BufferedAggregator
	dataOutputs

	senders *senders

	hostTagProvider *hosttags.HostTagProvider

	filterList filterlist.Component

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
	haAgent haagent.Component,
	compressor compression.Component,
	tagger tagger.Component,
	filterList filterlist.Component,
	hostname string) *AgentDemultiplexer {
	demux := initAgentDemultiplexer(log, sharedForwarder, orchestratorForwarder, options, eventPlatformForwarder, haAgent, compressor, tagger, filterList, hostname)
	go demux.run()
	return demux
}

func initAgentDemultiplexer(log log.Component,
	sharedForwarder forwarder.Forwarder,
	orchestratorForwarder orchestratorforwarder.Component,
	options AgentDemultiplexerOptions,
	eventPlatformForwarder eventplatform.Component,
	haAgent haagent.Component,
	compressor compression.Component,
	tagger tagger.Component,
	filterList filterlist.Component,
	hostname string) *AgentDemultiplexer {
	// prepare the multiple forwarders
	// -------------------------------
	if pkgconfigsetup.Datadog().GetBool("telemetry.enabled") && pkgconfigsetup.Datadog().GetBool("telemetry.dogstatsd_origin") && !pkgconfigsetup.Datadog().GetBool("aggregator_use_tags_store") {
		log.Warn("DogStatsD origin telemetry is not supported when aggregator_use_tags_store is disabled.")
		pkgconfigsetup.Datadog().Set("telemetry.dogstatsd_origin", false, model.SourceAgentRuntime)
	}

	// prepare the serializer
	// ----------------------

	sharedSerializer := serializer.NewSerializer(sharedForwarder, orchestratorForwarder, compressor, pkgconfigsetup.Datadog(), log, hostname)

	// prepare the embedded aggregator
	// --

	agg := NewBufferedAggregator(sharedSerializer, eventPlatformForwarder, haAgent, tagger, hostname, options.FlushInterval)

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
			bufferSize, metricSamplePool, agg.flushAndSerializeInParallel, tagsStore,
			filterList.GetMetricFilterList(), filterList.GetTagFilterList())
	}

	var noAggWorker *noAggregationStreamWorker
	var noAggSerializer serializer.MetricSerializer
	if options.EnableNoAggregationPipeline {
		noAggSerializer = serializer.NewSerializer(sharedForwarder, orchestratorForwarder, compressor, pkgconfigsetup.Datadog(), log, hostname)
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
		stopChan:  make(chan *trigger),
		flushChan: make(chan trigger),

		// Input
		aggregator: agg,

		// Output
		dataOutputs: dataOutputs{
			forwarders: forwarders{},

			sharedSerializer: sharedSerializer,
			noAggSerializer:  noAggSerializer,
		},

		hostTagProvider: hosttags.NewHostTagProvider(),
		senders:         newSenders(agg),

		filterList: filterList,

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

// eventLogView is a minimal observer.LogView implementation for forwarding aggregator events into the observer.
// It is immediately copied by the observer handle, so it must not be retained.
type eventLogView struct {
	content  []byte
	status   string
	tags     []string
	hostname string
	ts       int64
}

func (v *eventLogView) GetContent() []byte  { return v.content }
func (v *eventLogView) GetStatus() string   { return v.status }
func (v *eventLogView) GetTags() []string   { return v.tags }
func (v *eventLogView) GetHostname() string { return v.hostname }

// GetTimestamp is an optional method recognized by the observer handle to set event time.
func (v *eventLogView) GetTimestamp() int64 { return v.ts }

// standardizeEventType normalizes event type names to lowercase_with_underscores format.
// Examples: "Agent Startup" -> "agent_startup", "Container OOM" -> "container_oom"
func standardizeEventType(name string) string {
	// Convert to lowercase
	result := ""
	for _, ch := range name {
		if ch >= 'A' && ch <= 'Z' {
			result += string(ch + 32) // to lowercase
		} else if ch == ' ' || ch == '.' || ch == '-' {
			result += "_"
		} else {
			result += string(ch)
		}
	}
	// Collapse multiple underscores
	for len(result) > 0 && result[0] == '_' {
		result = result[1:]
	}
	for i := 0; i < len(result)-1; {
		if result[i] == '_' && result[i+1] == '_' {
			result = result[:i] + result[i+1:]
		} else {
			i++
		}
	}
	return result
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
				Text:           "Version " + agentVersion,
				SourceTypeName: "System",
				Host:           d.aggregator.hostname,
				EventType:      "Agent Startup",
			}
		}
	}
}

// SetObserver wires an observer component into the demultiplexer.
// This should be called after construction to enable mirroring of check metrics
// and events to the observer for local analysis/correlation.
func (d *AgentDemultiplexer) SetObserver(obs observer.Component) {
	if obs == nil {
		return
	}

	// Only wire if capture_metrics.enabled is enabled
	if !pkgconfigsetup.Datadog().GetBool("observer.analysis.enabled") || !pkgconfigsetup.Datadog().GetBool("observer.recording.enabled") {
		d.log.Debug("Observer metric capture disabled by configuration")
		return
	}

	// Wire all metric paths with a single global handle
	metricsHandle := obs.GetHandle("all-metrics")

	// Metrics: mirror raw check samples into the observer via the CheckSampler hook.
	d.aggregator.SetObserverHandle(metricsHandle)

	// DogStatsD metrics: wire all time sampler workers
	for _, worker := range d.statsd.workers {
		worker.sampler.observerHandle = metricsHandle
	}

	// Timestamped metrics (no-aggregation pipeline)
	if d.statsd.noAggStreamWorker != nil {
		d.statsd.noAggStreamWorker.observerHandle = metricsHandle
	}

	// Events: forward lifecycle events as best-effort log observations (used as event signals for correlation).
	eventsHandle := obs.GetHandle("check-events")
	d.aggregator.SetObserverEventSink(func(e event.Event) {
		// Copy tags to avoid mutating the underlying slice that is also stored in agg.events.
		tags := make([]string, len(e.Tags), len(e.Tags)+3)
		copy(tags, e.Tags)
		// Event signal tags used downstream for correlation/annotation (kept local to observer only).
		tags = append(tags,
			"observer_signal:event",
			fmt.Sprintf("observer_ts:%d", e.Ts),
			fmt.Sprintf("event_type:%s", standardizeEventType(e.EventType)))

		eventsHandle.ObserveLog(&eventLogView{
			content:  []byte(e.String()),
			status:   string(e.AlertType),
			tags:     tags,
			hostname: e.Host,
			ts:       e.Ts,
		})
	})
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

	// It is important to set this up after the statsd workers have been started
	// to make sure they are running to receive the initial filter list and any
	// updates
	d.filterList.OnUpdateMetricFilterList(d.SetSamplersFilterList)
	d.filterList.OnUpdateTagFilterList(d.SetAggregatorTagFilterList)

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
		case trigger, ok := <-d.stopChan:
			if ok && trigger != nil {
				// Final flush requested
				d.flushToSerializer(trigger.time, trigger.waitForSerializer, trigger.forceFlushAll)
				if trigger.blockChan != nil {
					trigger.blockChan <- struct{}{}
				}
			}
			return
		// manual flush sequence
		case trigger := <-d.flushChan:
			d.flushToSerializer(trigger.time, trigger.waitForSerializer, trigger.forceFlushAll)
			if trigger.blockChan != nil {
				trigger.blockChan <- struct{}{}
			}
		// automatic flush sequence
		case t := <-flushTicker:
			d.flushToSerializer(t, false, false)
		}
	}
}

// Stop stops the demultiplexer.
// Resources are released, the instance should not be used after a call to `Stop()`.
func (d *AgentDemultiplexer) Stop(flush bool) {
	timeout := pkgconfigsetup.Datadog().GetDuration("aggregator_stop_timeout") * time.Second
	forceFlushAll := pkgconfigsetup.Datadog().GetBool("dogstatsd_flush_incomplete_buckets")

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
			forceFlushAll:     forceFlushAll,
		}
		timeoutStart := time.Now()

		select {
		case <-time.After(timeout):
			d.log.Errorf("triggering flushing data on Stop() timed out")

		case d.stopChan <- &trigger:
			timeout = timeout - time.Since(timeoutStart)
			select {
			case <-trigger.blockChan:
			case <-time.After(timeout):
				d.log.Errorf("completing flushing data on Stop() timed out")
			}
		}

	} else {
		// stops the flushloop and makes sure no automatic flushes will happen anymore
		select {
		case d.stopChan <- nil:
		case <-time.After(timeout):
			d.log.Debug("unable to guarantee flush loop termination on Stop()")
		}
	}

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
		forceFlushAll:     false,
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
func (d *AgentDemultiplexer) flushToSerializer(start time.Time, waitForSerializer bool, forceFlushAll bool) {
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
						time:          start,
						blockChan:     make(chan struct{}),
						forceFlushAll: forceFlushAll,
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
						forceFlushAll:     forceFlushAll,
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

func (d *AgentDemultiplexer) SetAggregatorTagFilterList(tagmatcher filterlist.TagMatcher) {
	d.m.RLock()
	defer d.m.RUnlock()

	if d.aggregator == nil {
		// The demultiplexer has stopped and the workers and aggregator are no longer available
		// to receive updates.
		return
	}

	d.aggregator.tagfilterListChan <- tagmatcher

	for _, worker := range d.statsd.workers {
		worker.tagFilterListChan <- tagmatcher
	}
}

// SetSamplersFilterList triggers a reconfiguration of the filter list
// applied in the samplers.
func (d *AgentDemultiplexer) SetSamplersFilterList(filterList utilstrings.Matcher, histoFilterList utilstrings.Matcher) {
	d.m.RLock()
	defer d.m.RUnlock()

	if d.aggregator == nil {
		// The demultiplexer has stopped and the workers and aggregator are no longer available
		// to receive updates.
		return
	}

	// Most metrics coming from dogstatsd will have already been filtered in the listeners.
	// Histogram metrics need aggregating before we determine the correct name to be filtered.
	for _, worker := range d.statsd.workers {
		worker.metricFilterListChan <- histoFilterList
	}

	// Metrics from checks are only filtered here, so we need the full filter list.
	d.aggregator.filterListChan <- filterList
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
