// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package event

import (
	"time"

	metricsClient "github.com/DataDog/datadog-agent/pkg/trace/exportable/metrics/client"
	"github.com/DataDog/datadog-agent/pkg/trace/exportable/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/exportable/sampler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const maxEPSReportFrequency = 10 * time.Second

// maxEPSSampler (Max Events Per Second Sampler) is an event maxEPSSampler that samples provided events so as to try to ensure
// no more than a certain amount of events is sampled per second.
//
// Note that events associated with traces with UserPriorityKeep are always sampled and don't influence underlying
// rate counters so as not to skew stats.
type maxEPSSampler struct {
	maxEPS      float64
	rateCounter rateCounter

	reportFrequency time.Duration
	reportDone      chan bool
}

// NewMaxEPSSampler creates a new instance of a maxEPSSampler with the provided maximum amount of events per second.
func newMaxEPSSampler(maxEPS float64) *maxEPSSampler {
	return &maxEPSSampler{
		maxEPS:      maxEPS,
		rateCounter: newSamplerBackendRateCounter(),

		reportDone: make(chan bool),
	}
}

// Start starts the underlying rate counter.
func (s *maxEPSSampler) Start() {
	s.rateCounter.Start()

	go func() {
		ticker := time.NewTicker(maxEPSReportFrequency)
		defer close(s.reportDone)
		defer ticker.Stop()

		for {
			select {
			case <-s.reportDone:
				return
			case <-ticker.C:
				s.report()
			}
		}
	}()
}

// Stop stops the underlying rate counter.
func (s *maxEPSSampler) Stop() {
	s.reportDone <- true
	<-s.reportDone

	s.rateCounter.Stop()
}

// Sample determines whether or not we should sample the provided event in order to ensure no more than maxEPS events
// are sampled every second.
func (s *maxEPSSampler) Sample(event *pb.Span) (sampled bool, rate float64) {
	// Count that we saw a new event
	s.rateCounter.Count()
	rate = 1.0
	currentEPS := s.rateCounter.GetRate()
	if currentEPS > s.maxEPS {
		rate = s.maxEPS / currentEPS
	}
	sampled = sampler.SampleByRate(event.TraceID, rate)
	return
}

// getSampleRate returns the applied sample rate based on this sampler's current state.
func (s *maxEPSSampler) getSampleRate() float64 {
	rate := 1.0
	currentEPS := s.rateCounter.GetRate()
	if currentEPS > s.maxEPS {
		rate = s.maxEPS / currentEPS
	}
	return rate
}

func (s *maxEPSSampler) report() {
	maxRate := s.maxEPS
	metricsClient.Gauge("datadog.trace_agent.events.max_eps.max_rate", maxRate, nil, 1)

	currentRate := s.rateCounter.GetRate()
	metricsClient.Gauge("datadog.trace_agent.events.max_eps.current_rate", currentRate, nil, 1)

	sampleRate := s.getSampleRate()
	metricsClient.Gauge("datadog.trace_agent.events.max_eps.sample_rate", sampleRate, nil, 1)

	reachedMaxGaugeV := 0.
	if sampleRate < 1 {
		reachedMaxGaugeV = 1.
		log.Warnf("Max events per second reached (current=%.2f/s, max=%.2f/s). "+
			"Some events are now being dropped (sample rate=%.2f). Consider adjusting event sampling rates.",
			currentRate, maxRate, sampleRate)
	}
	metricsClient.Gauge("datadog.trace_agent.events.max_eps.reached_max", reachedMaxGaugeV, nil, 1)
}

// rateCounter keeps track of different event rates.
type rateCounter interface {
	Start()
	Count()
	GetRate() float64
	Stop()
}

// samplerBackendRateCounter is a rateCounter backed by a maxEPSSampler.Backend.
type samplerBackendRateCounter struct {
	backend sampler.Backend
}

// newSamplerBackendRateCounter creates a new samplerBackendRateCounter based on exponential decay counters.
func newSamplerBackendRateCounter() *samplerBackendRateCounter {
	return &samplerBackendRateCounter{
		// TODO: Allow these to be configurable or study better defaults based on intended target
		backend: sampler.NewMemoryBackend(1*time.Second, 1.125),
	}
}

// Start starts the decaying of the backend rate counter.
func (sb *samplerBackendRateCounter) Start() {
	go sb.backend.Run()
}

// Stop stops the decaying of the backend rate counter.
func (sb *samplerBackendRateCounter) Stop() {
	sb.backend.Stop()
}

// Count adds an event to the rate computation.
func (sb *samplerBackendRateCounter) Count() {
	sb.backend.CountSample()
}

// GetRate gets the current event rate.
func (sb *samplerBackendRateCounter) GetRate() float64 {
	return sb.backend.GetUpperSampledScore()
}
