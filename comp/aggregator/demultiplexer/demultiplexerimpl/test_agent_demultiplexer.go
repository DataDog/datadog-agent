// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package demultiplexerimpl

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/noopimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// TestAgentDemultiplexer is an implementation of the Demultiplexer which is sending
// the time samples into a fake sampler, you can then use WaitForSamples() to retrieve
// the samples that the TimeSamplers should have received.
type TestAgentDemultiplexer struct {
	*aggregator.AgentDemultiplexer
	aggregatedSamples []metrics.MetricSample
	noAggSamples      []metrics.MetricSample
	sync.Mutex

	events        chan []*event.Event
	serviceChecks chan []*servicecheck.ServiceCheck
}

// NewTestAgentDemultiplexer returns a new instance of TestAgentDemultiplexer.
func NewTestAgentDemultiplexer(demultiplexer *aggregator.AgentDemultiplexer) *TestAgentDemultiplexer {
	return &TestAgentDemultiplexer{
		AgentDemultiplexer: demultiplexer,
		events:             make(chan []*event.Event),
		serviceChecks:      make(chan []*servicecheck.ServiceCheck),
	}
}

// AggregateSamples implements a noop timesampler, appending the samples in an internal slice.
//
//nolint:revive // TODO(AML) Fix revive linter
func (a *TestAgentDemultiplexer) AggregateSamples(shard aggregator.TimeSamplerID, samples metrics.MetricSampleBatch) {
	a.Lock()
	a.aggregatedSamples = append(a.aggregatedSamples, samples...)
	a.Unlock()
}

// AggregateSample implements a noop timesampler, appending the sample in an internal slice.
func (a *TestAgentDemultiplexer) AggregateSample(sample metrics.MetricSample) {
	a.Lock()
	a.aggregatedSamples = append(a.aggregatedSamples, sample)
	a.Unlock()
}

// GetEventPlatformForwarder returns a event platform forwarder
func (a *TestAgentDemultiplexer) GetEventPlatformForwarder() (eventplatform.Forwarder, error) {
	return a.AgentDemultiplexer.GetEventPlatformForwarder()
}

// GetEventsAndServiceChecksChannels returneds underlying events and service checks channels.
func (a *TestAgentDemultiplexer) GetEventsAndServiceChecksChannels() (chan []*event.Event, chan []*servicecheck.ServiceCheck) {
	return a.events, a.serviceChecks
}

// SendSamplesWithoutAggregation implements a fake no aggregation pipeline ingestion part,
// there will be NO AUTOMATIC FLUSH as it could exist in the real implementation
// Use Reset() to clean the buffer.
func (a *TestAgentDemultiplexer) SendSamplesWithoutAggregation(metrics metrics.MetricSampleBatch) {
	a.Lock()
	a.noAggSamples = append(a.noAggSamples, metrics...)
	a.Unlock()
}

func (a *TestAgentDemultiplexer) samples() (ontime []metrics.MetricSample, timed []metrics.MetricSample) {
	a.Lock()
	ontime = make([]metrics.MetricSample, len(a.aggregatedSamples))
	timed = make([]metrics.MetricSample, len(a.noAggSamples))
	copy(ontime, a.aggregatedSamples)
	copy(timed, a.noAggSamples)
	a.Unlock()
	return ontime, timed
}

// WaitForSamples returns the samples received by the demultiplexer.
// Note that it returns as soon as something is avaible in either the live
// metrics buffer or the late metrics one.
func (a *TestAgentDemultiplexer) WaitForSamples(timeout time.Duration) (ontime []metrics.MetricSample, timed []metrics.MetricSample) {
	return a.waitForSamples(timeout, func(ontime, timed []metrics.MetricSample) bool {
		return len(ontime) > 0 || len(timed) > 0
	})
}

// WaitForNumberOfSamples returns the samples received by the demultiplexer.
// Note that it waits until at least the requested number of samples are
// available in both the live metrics buffer and the late metrics one.
func (a *TestAgentDemultiplexer) WaitForNumberOfSamples(ontimeCount, timedCount int, timeout time.Duration) (ontime []metrics.MetricSample, timed []metrics.MetricSample) {
	return a.waitForSamples(timeout, func(ontime, timed []metrics.MetricSample) bool {
		return (len(ontime) >= ontimeCount || ontimeCount == 0) &&
			(len(timed) >= timedCount || timedCount == 0)
	})
}

// waitForSamples returns the samples received by the demultiplexer.
// It returns once the given foundFunc returns true or the timeout is reached.
func (a *TestAgentDemultiplexer) waitForSamples(timeout time.Duration, foundFunc func([]metrics.MetricSample, []metrics.MetricSample) bool) (ontime []metrics.MetricSample, timed []metrics.MetricSample) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeoutOn := time.Now().Add(timeout)
	for {
		select {
		case <-ticker.C:
			ontime, timed = a.samples()

			// this case could always take priority on the timeout case, we have to make sure
			// we've not timeout
			if time.Now().After(timeoutOn) {
				return ontime, timed
			}

			if foundFunc(ontime, timed) {
				return ontime, timed
			}
		case <-time.After(timeout):
			return nil, nil
		}
	}
}

// WaitEventPlatformEvents waits for timeout and eventually returns the event platform events samples received by the demultiplexer.
func (a *TestAgentDemultiplexer) WaitEventPlatformEvents(eventType string, minEvents int, timeout time.Duration) ([]*message.Message, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeoutOn := time.Now().Add(timeout)
	var savedEvents []*message.Message
	for {
		select {
		case <-ticker.C:
			allEvents := a.AgentDemultiplexer.Aggregator().GetEventPlatformEvents()
			savedEvents = append(savedEvents, allEvents[eventType]...)
			// this case could always take priority on the timeout case, we have to make sure
			// we've not timeout
			if time.Now().After(timeoutOn) {
				return nil, fmt.Errorf("timeout waitig for events (expected at least %d events but only received %d)", minEvents, len(savedEvents))
			}

			if len(savedEvents) >= minEvents {
				return savedEvents, nil
			}
		case <-time.After(timeout):
			return nil, fmt.Errorf("timeout waitig for events (expected at least %d events but only received %d)", minEvents, len(savedEvents))
		}
	}
}

// Reset resets the internal samples slice.
func (a *TestAgentDemultiplexer) Reset() {
	a.Lock()
	a.aggregatedSamples = a.aggregatedSamples[0:0]
	a.noAggSamples = a.noAggSamples[0:0]
	a.Unlock()
}

// initTestAgentDemultiplexerWithFlushInterval inits a TestAgentDemultiplexer with the given flush interval.
func initTestAgentDemultiplexerWithFlushInterval(log log.Component, hostname hostname.Component, compressor compression.Component, flushInterval time.Duration) *TestAgentDemultiplexer {
	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.FlushInterval = flushInterval
	opts.DontStartForwarders = true
	opts.EnableNoAggregationPipeline = true

	sharedForwarderOptions := defaultforwarder.NewOptions(pkgconfigsetup.Datadog(), log, nil)
	sharedForwarder := defaultforwarder.NewDefaultForwarder(pkgconfigsetup.Datadog(), log, sharedForwarderOptions)
	orchestratorForwarder := optional.NewOption[defaultforwarder.Forwarder](defaultforwarder.NoopForwarder{})
	eventPlatformForwarder := optional.NewOptionPtr[eventplatform.Forwarder](eventplatformimpl.NewNoopEventPlatformForwarder(hostname))
	demux := aggregator.InitAndStartAgentDemultiplexer(log, sharedForwarder, &orchestratorForwarder, opts, eventPlatformForwarder, compressor, noopimpl.NewTaggerClient(), "hostname")
	return NewTestAgentDemultiplexer(demux)
}
