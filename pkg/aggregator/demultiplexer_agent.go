// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"io"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	orchestratorforwarder "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
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
	GetEventPlatformForwarder() (epforwarder.EventPlatformForwarder, error)
	GetEventsAndServiceChecksChannels() (chan []*event.Event, chan []*servicecheck.ServiceCheck)
	DumpDogstatsdContexts(io.Writer) error
}

// AgentDemultiplexer is the demultiplexer implementation for the main Agent.
type AgentDemultiplexer struct {
	log log.Component

	m sync.Mutex

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

	// sharded statsd time samplers
	statsd
}

// AgentDemultiplexerOptions are the options used to initialize a Demultiplexer.
type AgentDemultiplexerOptions struct {
	UseNoopEventPlatformForwarder bool
	UseEventPlatformForwarder     bool
	FlushInterval                 time.Duration

	EnableNoAggregationPipeline bool

	DontStartForwarders bool // unit tests don't need the forwarders to be instanciated

	UseDogstatsdContextLimiter bool
	DogstatsdMaxMetricsTags    int
}

// DefaultAgentDemultiplexerOptions returns the default options to initialize an AgentDemultiplexer.
func DefaultAgentDemultiplexerOptions() AgentDemultiplexerOptions {
	panic("not called")
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
	shared             forwarder.Forwarder
	orchestrator       orchestratorforwarder.Component
	eventPlatform      epforwarder.EventPlatformForwarder
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
func InitAndStartAgentDemultiplexer(log log.Component, sharedForwarder forwarder.Forwarder, orchestratorForwarder orchestratorforwarder.Component, options AgentDemultiplexerOptions, hostname string) *AgentDemultiplexer {
	panic("not called")
}

func initAgentDemultiplexer(log log.Component, sharedForwarder forwarder.Forwarder, orchestratorForwarder orchestratorforwarder.Component, options AgentDemultiplexerOptions, hostname string) *AgentDemultiplexer {
	panic("not called")
}

// Options returns options used during the demux initialization.
func (d *AgentDemultiplexer) Options() AgentDemultiplexerOptions {
	panic("not called")
}

// AddAgentStartupTelemetry adds a startup event and count (in a DSD time sampler)
// to be sent on the next flush.
func (d *AgentDemultiplexer) AddAgentStartupTelemetry(agentVersion string) {
	panic("not called")
}

// Run runs all demultiplexer parts
func (d *AgentDemultiplexer) Run() {
	panic("not called")
}

func (d *AgentDemultiplexer) flushLoop() {
	panic("not called")
}

// Stop stops the demultiplexer.
// Resources are released, the instance should not be used after a call to `Stop()`.
func (d *AgentDemultiplexer) Stop(flush bool) {
	panic("not called")
}

// ForceFlushToSerializer triggers the execution of a flush from all data of samplers
// and the BufferedAggregator to the serializer.
// Safe to call from multiple threads.
func (d *AgentDemultiplexer) ForceFlushToSerializer(start time.Time, waitForSerializer bool) {
	panic("not called")
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
	panic("not called")
}

// GetEventsAndServiceChecksChannels returneds underlying events and service checks channels.
func (d *AgentDemultiplexer) GetEventsAndServiceChecksChannels() (chan []*event.Event, chan []*servicecheck.ServiceCheck) {
	panic("not called")
}

// GetEventPlatformForwarder returns underlying events and service checks channels.
func (d *AgentDemultiplexer) GetEventPlatformForwarder() (epforwarder.EventPlatformForwarder, error) {
	panic("not called")
}

// SendSamplesWithoutAggregation buffers a bunch of metrics with timestamp. This data will be directly
// transmitted "as-is" (i.e. no aggregation, no sampling) to the serializer.
func (d *AgentDemultiplexer) SendSamplesWithoutAggregation(samples metrics.MetricSampleBatch) {
	panic("not called")
}

// AggregateSamples adds a batch of MetricSample into the given DogStatsD time sampler shard.
// If you have to submit a single metric sample see `AggregateSample`.
func (d *AgentDemultiplexer) AggregateSamples(shard TimeSamplerID, samples metrics.MetricSampleBatch) {
	panic("not called")
}

// AggregateSample adds a MetricSample in the first DogStatsD time sampler.
func (d *AgentDemultiplexer) AggregateSample(sample metrics.MetricSample) {
	panic("not called")
}

// AggregateCheckSample adds check sample sent by a check from one of the collectors into a check sampler pipeline.
//
//nolint:revive // TODO(AML) Fix revive linter
func (d *AgentDemultiplexer) AggregateCheckSample(sample metrics.MetricSample) {
	panic("not called")
}

// GetDogStatsDPipelinesCount returns how many sampling pipeline are running for
// the DogStatsD samples.
func (d *AgentDemultiplexer) GetDogStatsDPipelinesCount() int {
	panic("not called")
}

// Serializer returns a serializer that anyone can use. This method exists
// to keep compatibility with existing code while introducing the Demultiplexer,
// however, the plan is to remove it anytime soon.
//
// Deprecated.
func (d *AgentDemultiplexer) Serializer() serializer.MetricSerializer {
	panic("not called")
}

// Aggregator returns an aggregator that anyone can use. This method exists
// to keep compatibility with existing code while introducing the Demultiplexer,
// however, the plan is to remove it anytime soon.
//
// Deprecated.
func (d *AgentDemultiplexer) Aggregator() *BufferedAggregator {
	panic("not called")
}

// GetMetricSamplePool returns a shared resource used in the whole DogStatsD
// pipeline to re-use metric samples slices: the server is getting a slice
// and filling it with samples, the rest of the pipeline process them the
// end of line (the time sampler) is putting back the slice in the pool.
// Main idea is to reduce the garbage generated by slices allocation.
func (d *AgentDemultiplexer) GetMetricSamplePool() *metrics.MetricSamplePool {
	panic("not called")
}

// DumpDogstatsdContexts writes the current state of the context resolver to dest.
//
// This blocks metrics processing, so dest is expected to be reasonably fast and not block for too
// long.
func (d *AgentDemultiplexer) DumpDogstatsdContexts(dest io.Writer) error {
	panic("not called")
}

// GetSender returns a sender.Sender with passed ID, properly registered with the aggregator
// If no error is returned here, DestroySender must be called with the same ID
// once the sender is not used anymore
func (d *AgentDemultiplexer) GetSender(id checkid.ID) (sender.Sender, error) {
	panic("not called")
}

// SetSender returns the passed sender with the passed ID.
// This is largely for testing purposes
func (d *AgentDemultiplexer) SetSender(s sender.Sender, id checkid.ID) error {
	panic("not called")
}

// DestroySender frees up the resources used by the sender with passed ID (by deregistering it from the aggregator)
// Should be called when no sender with this ID is used anymore
// The metrics of this (these) sender(s) that haven't been flushed yet will be lost
func (d *AgentDemultiplexer) DestroySender(id checkid.ID) {
	panic("not called")
}

// GetDefaultSender returns a default sender.
func (d *AgentDemultiplexer) GetDefaultSender() (sender.Sender, error) {
	panic("not called")
}
