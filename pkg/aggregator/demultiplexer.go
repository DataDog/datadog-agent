// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DemultiplexerInstance is a shared global demultiplexer instance.
// Initialized by InitAndStartAgentDemultiplexer or InitAndStartServerlessDemultiplexer,
// could be nil otherwise. TODO(remy): remove this global instance in the future.
//
// Deprecated.
var demultiplexerInstance Demultiplexer

// Demultiplexer is composed of multiple samplers (check and time/dogstatsd)
// a shared forwarder, the event platform forwarder, orchestrator data buffers
// and other data that need to then be sent to the forwarders.
// DemultiplexerOptions let you configure which forwarders have to be started.
// They are not started automatically if `options.StartForwarders` is not
// explicitly set to `true`.
type Demultiplexer interface {
	// General

	Run()
	Stop(flush bool)

	// Aggregation API

	// AddTimeSamples adds time samples processed by the DogStatsD server into a time sampler pipeline.
	// The MetricSamples should have their hash computed.
	// TODO(remy): not implemented yet.
	AddTimeSamples(sample []metrics.MetricSample)
	// AddCheckSample adds check sample sent by a check from one of the collectors into a check sampler pipeline.
	// TODO(remy): not implemented yet.
	AddCheckSample(sample metrics.MetricSample)
	// FlushAggregatedData flushes all the aggregated data from the samplers to
	// the serialization/forwarding parts.
	FlushAggregatedData(start time.Time, waitForSerializer bool)
	// Aggregator returns an aggregator that anyone can use. This method exists
	// to keep compatibility with existing code while introducing the Demultiplexer,
	// however, the plan is to remove it anytime soon.
	//
	// Deprecated.
	Aggregator() *BufferedAggregator
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
// Note that forwarders are started automatically only if `StartForwarders`
// is set to `true`.
type DemultiplexerOptions struct {
	ForwarderOptions           *forwarder.Options
	NoopEventPlatformForwarder bool
	NoEventPlatformForwarder   bool
	NoOrchestratorForwarder    bool
	FlushInterval              time.Duration
	StartForwarders            bool // unit tests don't need the forwarders to be instanciated
}

type forwarders struct {
	shared        *forwarder.DefaultForwarder
	orchestrator  *forwarder.DefaultForwarder
	eventPlatform epforwarder.EventPlatformForwarder
}

type output struct {
	forwarders       forwarders
	sharedSerializer serializer.MetricSerializer
}

// DefaultDemultiplexerOptions returns the default options to initialize a Demultiplexer.
func DefaultDemultiplexerOptions(options *forwarder.Options) DemultiplexerOptions {
	if options == nil {
		options = forwarder.NewOptions(nil)
	}

	return DemultiplexerOptions{
		ForwarderOptions: options,
		FlushInterval:    DefaultFlushInterval,
	}
}

// InitAndStartAgentDemultiplexer creates a new Demultiplexer and runs what's necessary
// in goroutines. As of today, only the embedded BufferedAggregator needs a separate goroutine.
// In the future, goroutines will be started for the event platform forwarder and/or orchestrator forwarder.
func InitAndStartAgentDemultiplexer(options DemultiplexerOptions, hostname string) *AgentDemultiplexer {
	// prepare the multiple forwarders
	// -------------------------------

	log.Debugf("Starting forwarders")
	// orchestrator forwarder
	var orchestratorForwarder *forwarder.DefaultForwarder
	if !options.NoOrchestratorForwarder {
		orchestratorForwarder = buildOrchestratorForwarder()
	}

	// event platform forwarder
	var eventPlatformForwarder epforwarder.EventPlatformForwarder
	if !options.NoEventPlatformForwarder && options.NoopEventPlatformForwarder {
		eventPlatformForwarder = epforwarder.NewNoopEventPlatformForwarder()
	} else if !options.NoEventPlatformForwarder {
		eventPlatformForwarder = epforwarder.NewEventPlatformForwarder()
	}

	sharedForwarder := forwarder.NewDefaultForwarder(options.ForwarderOptions)

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

		// Output
		output: output{

			forwarders: forwarders{
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

// AddAgentStartupTelemetry adds a startup event and count to be sent on the next flush
func (d *AgentDemultiplexer) AddAgentStartupTelemetry(str string) {
	if str != "" {
		d.aggregator.AddAgentStartupTelemetry(str)
	}
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
// FIXME(remy): document thread-safety once aggregated API has been implemented
func (d *AgentDemultiplexer) FlushAggregatedData(start time.Time, waitForSerializer bool) {
	d.Lock()
	defer d.Unlock()

	if d.aggregator != nil {
		d.aggregator.Flush(start, waitForSerializer)
	}
}

// AddTimeSamples adds time samples processed by the DogStatsD server into a time sampler pipeline.
// The MetricSamples should have their hash computed.
// TODO(remy): not implemented yet.
func (d *AgentDemultiplexer) AddTimeSamples(samples []metrics.MetricSample) {
	// TODO(remy): it may makes sense to embed the `batcher` directly in the Demultiplexer

	// TODO(remy): this is the entry point to the time sampler pipelines, we will probably want to do something
	// TODO(remy): like `pipelines[sample.Key%d.pipelinesCount].samples <- sample`
	// TODO(remy): where all readers of these channels are running in a different routine
	panic("not implemented yet.")
}

// AddCheckSample adds check sample sent by a check from one of the collectors into a check sampler pipeline.
// TODO(remy): not implemented yet.
func (d *AgentDemultiplexer) AddCheckSample(sample metrics.MetricSample) {
	panic("not implemented yet.")
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
func InitAndStartServerlessDemultiplexer(domainResolvers map[string]resolver.DomainResolver, hostname string, forwarderTimeout time.Duration) *ServerlessDemultiplexer {
	forwarder := forwarder.NewSyncForwarder(domainResolvers, forwarderTimeout)
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

// AddTimeSamples adds time samples processed by the DogStatsD server into a time sampler pipeline.
// The MetricSamples should have their hash computed.
// TODO(remy): not implemented yet.
func (d *ServerlessDemultiplexer) AddTimeSamples(samples []metrics.MetricSample) {
	// TODO(remy): it may makes sense to embed the `batcher` directly in the Demultiplexer

	// TODO(remy): this is the entry point to the time sampler pipelines, we will probably want to do something
	// TODO(remy): like `pipelines[sample.Key%d.pipelinesCount].samples <- sample`
	// TODO(remy): where all readers of these channels are running in a different routine

	// TODO(remy): for now, these are sent directly to the aggregator channels
	panic("not implemented yet.")
}

// AddCheckSample doesn't do anything in the Serverless Agent implementation.
func (d *ServerlessDemultiplexer) AddCheckSample(sample metrics.MetricSample) {
	panic("not implemented yet.")
}

// Serializer returns the shared serializer
func (d *ServerlessDemultiplexer) Serializer() serializer.MetricSerializer {
	return d.serializer
}

// Aggregator returns the main buffered aggregator
func (d *ServerlessDemultiplexer) Aggregator() *BufferedAggregator {
	return d.aggregator
}
