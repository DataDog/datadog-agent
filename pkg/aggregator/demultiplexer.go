// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	orch "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// DemultiplexerInstance is a shared global demultiplexer instance
// Could be uninitialized if nobody's created and set a Demultiplexer
// as a global instance.
var demultiplexerInstance Demultiplexer

// Demultiplexer is composed of the samplers and their multiple pipelines,
// the event platform forwarder, orchestrator data buffers and other data
// that need to then be sent to the forwarder.
type Demultiplexer interface {
	// General

	Run()
	Stop(flush bool)

	// Aggregation API

	// TODO(remy): comment me
	AddTimeSamples(sample []metrics.MetricSample)
	// TODO(remy): comment me
	AddCheckSample(sample metrics.MetricSample)
	// FlushAggregatedData flushes all the aggregated data from the samplers to
	// the serialization part.
	FlushAggregatedData(start time.Time, waitForSerializer bool)
	// Aggregator returns an aggregator that anyone can use. This method exists
	// to keep compatibility with existing code while introducing the Demultiplexer,
	// however, the plan is to remove it anytime soon.
	//
	// Deprecated.
	Aggregator() *BufferedAggregator // (remy): remove me
	// Serializer returns a serializer that anyone can use. This method exists
	// to keep compatibility with existing code while introducing the Demultiplexer,
	// however, the plan is to remove it anytime soon.
	//
	// Deprecated.
	Serializer() serializer.MetricSerializer

	// Senders API, mainly used by collectors/checks

	GetSender(id check.ID) (Sender, error)
	SetSender(sender Sender, id check.ID) error
	DestroySender(id check.ID)
	GetDefaultSender() (Sender, error)
	ChangeAllSendersDefaultHostname(hostname string)
	cleanSenders()
}

// AgentDemultiplexer is the demultiplexer implementation for the main Agent
type AgentDemultiplexer struct {
	sync.Mutex

	// options are the options with which the demultiplexer has been created
	options    DemultiplexerOptions
	aggregator *BufferedAggregator
	output
	*senders
}

// DemultiplexerOptions are the options used to initialize a Demultiplexer.
type DemultiplexerOptions struct {
	Forwarder                  *forwarder.Options
	NoopEventPlatformForwarder bool
	NoEventPlatformForwarder   bool
	NoOrchestratorForwarder    bool
	FlushInterval              time.Duration
	StartupTelemetry           string
	StartForwarders            bool // unit tests don't need the forwarders to be instanciated
}

type outputForwarders struct {
	shared        *forwarder.DefaultForwarder
	orchestrator  *forwarder.DefaultForwarder
	eventPlatform epforwarder.EventPlatformForwarder
}

type output struct {
	forwarders outputForwarders

	sharedSerializer serializer.MetricSerializer
}

// DefaultDemultiplexerOptions returns the default options to initialize a Demultiplexer.
func DefaultDemultiplexerOptions(keysPerDomain map[string][]string) DemultiplexerOptions {
	return DemultiplexerOptions{
		Forwarder:        forwarder.NewOptions(keysPerDomain),
		FlushInterval:    DefaultFlushInterval,
		StartupTelemetry: version.AgentVersion,
	}
}

// InitAndStartAgentDemultiplexer creates a new Demultiplexer and runs what's necessary
// in goroutines. As of today, only the embedded BufferedAggregator needs a separate routine.
// In the future, routines will be started for the event platform forwarder and/or orchestrator forwarder.
func InitAndStartAgentDemultiplexer(options DemultiplexerOptions, hostname string) *AgentDemultiplexer {
	// prepare the multiple forwarders
	// -------------------------------

	log.Debugf("Starting forwarders")
	// orchestrator forwarder
	orchestratorForwarder := orch.NewOrchestratorForwarder()

	// event platform forwarder
	var eventPlatformForwarder epforwarder.EventPlatformForwarder
	if !options.NoEventPlatformForwarder && options.NoopEventPlatformForwarder {
		eventPlatformForwarder = epforwarder.NewNoopEventPlatformForwarder()
	} else if !options.NoEventPlatformForwarder {
		eventPlatformForwarder = epforwarder.NewEventPlatformForwarder()
	}

	sharedForwarder := forwarder.NewDefaultForwarder(options.Forwarder)

	// prepare the serializer
	// ----------------------

	sharedSerializer := serializer.NewSerializer(sharedForwarder, orchestratorForwarder)

	// prepare the embedded aggregator
	// --

	agg := InitAggregatorWithFlushInterval(sharedSerializer, eventPlatformForwarder, hostname, options.FlushInterval)

	// --

	if demultiplexerInstance != nil {
		log.Warn("A DemultiplexerInstance is already existing but InitAndStartAgentDemultiplexer has been called again. Current instance will be overridden")
	}

	demux := &AgentDemultiplexer{
		options: options,

		// Input
		aggregator: agg,

		// Processing

		// Output
		output: output{

			forwarders: outputForwarders{
				shared:        sharedForwarder,
				orchestrator:  orchestratorForwarder,
				eventPlatform: eventPlatformForwarder,
			},

			sharedSerializer: sharedSerializer,
		},

		senders: newSenders(agg),
	}

	demultiplexerInstance = demux

	go demux.Run()
	agg.AddAgentStartupTelemetry(options.StartupTelemetry)

	return demux
}

// Run runs all demultiplexer parts
func (d *AgentDemultiplexer) Run() {
	if d.options.StartForwarders {
		if d.forwarders.orchestrator != nil {
			d.forwarders.orchestrator.Start() //nolint:errcheck
		} else {
			log.Debug("not starting the orchestrator forwarder")
		}
		if d.forwarders.eventPlatform != nil {
			d.forwarders.eventPlatform.Start()
		} else {
			log.Debug("not starting the event platform forwarder")
		}
		if d.forwarders.shared != nil {
			d.forwarders.shared.Start() //nolint:errcheck
		} else {
			log.Debug("not starting the shared forwarder")
		}
		log.Debug("Forwarders started")
	}

	d.aggregator.run() // this is the blocking call
}

// Stop stops the demultiplexer
func (d *AgentDemultiplexer) Stop(flush bool) {
	d.Lock()
	defer d.Unlock()

	if d.aggregator != nil {
		d.aggregator.Stop(flush)
	}
	d.aggregator = nil

	if d.options.StartForwarders {
		if d.output.forwarders.orchestrator != nil {
			d.output.forwarders.orchestrator.Stop()
		}
		if d.output.forwarders.eventPlatform != nil {
			d.output.forwarders.eventPlatform.Stop()
		}
		if d.output.forwarders.shared != nil {
			d.output.forwarders.shared.Stop()
		}
	}

	d.output.sharedSerializer = nil
	d.senders = nil

	demultiplexerInstance = nil
}

// FlushAggregatedData flushes all data from the aggregator to the serializer
func (d *AgentDemultiplexer) FlushAggregatedData(start time.Time, waitForSerializer bool) {
	d.Lock()
	defer d.Unlock()

	if d.aggregator != nil {
		d.aggregator.Flush(start, waitForSerializer)
	}
}

// AddTimeSamples adds a sample into the time samplers (DogStatsD) pipelines
// The MetricSamples in samples have to contain their hash already computed.
// TODO (remy): implement me
func (d *AgentDemultiplexer) AddTimeSamples(samples []metrics.MetricSample) {
	// TODO(remy): it may makes sense to embed the `batcher` directly in the Demultiplexer

	// TODO(remy): this is the entry point to the time sampler pipelines, we will probably want to do something
	// TODO(remy): like `pipelines[sample.Key%d.pipelinesCount].samples <- sample`
	// TODO(remy): where all readers of these channels are running in a different routine
}

// AddCheckSample adds a check sample into the check samplers pipeline.
// XXX(remy): implement me
func (d *AgentDemultiplexer) AddCheckSample(sample metrics.MetricSample) {
	// XXX(remy): for now, send it to the aggregator
}

// Serializer returns a serializer that anyone can use. This method exists
// to keep compatibility with existing code while introducing the Demultiplexer,
// however, the plan is to remove it anytime soon.
//
// Deprecated.
func (d *AgentDemultiplexer) Serializer() serializer.MetricSerializer {
	return d.output.sharedSerializer
}

// Aggregator returns an aggregator that anyone can use. This method exists
// to keep compatibility with existing code while introducing the Demultiplexer,
// however, the plan is to remove it anytime soon.
//
// Deprecated.
func (d *AgentDemultiplexer) Aggregator() *BufferedAggregator {
	return d.aggregator
}

// ------------------------------

// ServerlessDemultiplexer is a simple demultiplexer used by the serverless flavor of the Agent
type ServerlessDemultiplexer struct {
	aggregator *BufferedAggregator
	serializer *serializer.Serializer
	forwarder  *forwarder.SyncForwarder
	*senders
}

// InitAndStartServerlessDemultiplexer creates and starts new Demultiplexer for the serverless agent.
func InitAndStartServerlessDemultiplexer(keysPerDomain map[string][]string, hostname string, forwarderTimeout time.Duration) *ServerlessDemultiplexer {
	forwarder := forwarder.NewSyncForwarder(keysPerDomain, forwarderTimeout)
	serializer := serializer.NewSerializer(forwarder, nil)
	// TODO(remy): what about the flush interval here?
	aggregator := InitAggregator(serializer, nil, hostname)

	demux := &ServerlessDemultiplexer{
		aggregator: aggregator,
		serializer: serializer,
		forwarder:  forwarder,
		senders:    newSenders(aggregator),
	}

	demultiplexerInstance = demux

	go demux.Run()

	return demux
}

// Run runs all demultiplexer parts
func (d *ServerlessDemultiplexer) Run() {
	if d.forwarder != nil {
		d.forwarder.Start() //nolint:errcheck
	} else {
		log.Debug("not starting the forwarder")
	}
	log.Debug("Forwarder started")

	d.aggregator.run()
	log.Debug("Aggregator started")
}

// Stop stops the wrapped aggregator and the forwarder.
func (d *ServerlessDemultiplexer) Stop(flush bool) {
	d.aggregator.Stop(flush)

	if d.forwarder != nil {
		d.forwarder.Stop()
	}
}

// FlushAggregatedData flushes all data from the aggregator to the serializer
func (d *ServerlessDemultiplexer) FlushAggregatedData(start time.Time, waitForSerializer bool) {
	d.aggregator.Flush(start, waitForSerializer)
}

// AddTimeSamples adds a sample into the time samplers (DogStatsD) pipelines
// The MetricSamples in samples have to contain their hash already computed.
// TODO (remy): implement me
func (d *ServerlessDemultiplexer) AddTimeSamples(samples []metrics.MetricSample) {
	// TODO(remy): it may makes sense to embed the `batcher` directly in the Demultiplexer

	// TODO(remy): this is the entry point to the time sampler pipelines, we will probably want to do something
	// TODO(remy): like `pipelines[sample.Key%d.pipelinesCount].samples <- sample`
	// TODO(remy): where all readers of these channels are running in a different routine

	// TODO(remy): for now, these are sent directly to the aggregator channels
}

// AddCheckSample adds a check sample into the check samplers pipeline.
// TODO(remy): implement me
func (d *ServerlessDemultiplexer) AddCheckSample(sample metrics.MetricSample) {
	// TODO(remy): for now, these are sent directly to the aggregator channels
}

// Serializer returns the shared serializer
func (d *ServerlessDemultiplexer) Serializer() serializer.MetricSerializer {
	return d.serializer
}

// Aggregator returns the main buffered aggregator
func (d *ServerlessDemultiplexer) Aggregator() *BufferedAggregator {
	return d.aggregator
}
