// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sampler contains all the logic of the agent-side trace sampling
//
// Currently implementation is based on the scoring of the "signature" of each trace
// Based on the score, we get a sample rate to apply to the given trace
//
// Current score implementation is super-simple, it is a counter with polynomial decay per signature.
// We increment it for each incoming trace then we periodically divide the score by two every X seconds.
// Right after the division, the score is an approximation of the number of received signatures over X seconds.
// It is different from the scoring in the Agent.
//
// Since the sampling can happen at different levels (client, agent, server) or depending on different rules,
// we have to track the sample rate applied at previous steps. This way, sampling twice at 50% can result in an
// effective 25% sampling. The rate is stored as a metric in the trace root.
package sampler

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

const (
	deprecatedRateKey = "_sampling_priority_rate_v1"
	agentRateKey      = "_dd.agent_psr"
	ruleRateKey       = "_dd.rule_psr"
	syncPeriod        = 3 * time.Second
	// priorityLocalRateThresholdTo1 defines the maximum allowed sampling rate below 1.
	// If this is surpassed, the rate is set to 1.
	priorityLocalRateThresholdTo1 = 0.3
)

// PrioritySampler computes priority rates per env, service to apply in a feedback loop with trace-agent clients.
// Computed rates are sent in http responses to trace-agent. The rates are continuously adjusted in function
// of the received traffic to match a targetTPS (target traces per second).
// In order of priority, the sampler will match a targetTPS set remotely (remoteRates) and then the local targetTPS.
type PrioritySampler struct {
	// localRates targetTPS is defined locally on the agent
	// This sampler tries to get the received number of sampled trace chunks/s to match its targetTPS.
	localRates *Sampler
	// remoteRates targetTPS is set remotely and distributed by remote configurations.
	// One target is defined per combination of env, service and it applies only to root spans.
	remoteRates *RemoteRates

	// rateByService contains the sampling rates in % to communicate with trace-agent clients.
	// This struct is shared with the agent API which sends the rates in http responses to spans post requests
	rateByService *RateByService
	catalog       *serviceKeyCatalog
	exit          chan struct{}
}

// NewPrioritySampler returns an initialized Sampler
func NewPrioritySampler(conf *config.AgentConfig, dynConf *DynamicConfig) *PrioritySampler {
	s := &PrioritySampler{
		localRates:    newSampler(conf.ExtraSampleRate, conf.TargetTPS, []string{"sampler:priority"}),
		remoteRates:   newRemoteRates(),
		rateByService: &dynConf.RateByService,
		catalog:       newServiceLookup(),
		exit:          make(chan struct{}),
	}
	s.localRates.setRateThresholdTo1(priorityLocalRateThresholdTo1)
	return s
}

// Start runs and block on the Sampler main loop
func (s *PrioritySampler) Start() {
	s.localRates.Start()
	if s.remoteRates != nil {
		s.remoteRates.Start()
	}
	go func() {
		t := time.NewTicker(syncPeriod)
		defer t.Stop()

		for {
			select {
			case <-t.C:
				s.rateByService.SetAll(s.ratesByService())
			case <-s.exit:
				return
			}
		}
	}()
}

// Stop stops the sampler main loop
func (s *PrioritySampler) Stop() {
	s.localRates.Stop()
	if s.remoteRates != nil {
		s.remoteRates.Stop()
	}
	close(s.exit)
}

// Sample counts an incoming trace and returns the trace sampling decision and the applied sampling rate
func (s *PrioritySampler) Sample(trace *pb.TraceChunk, root *pb.Span, env string, clientDroppedP0s bool) bool {
	// Extra safety, just in case one trace is empty
	if len(trace.Spans) == 0 {
		return false
	}

	samplingPriority, _ := GetSamplingPriority(trace)
	// Regardless of rates, sampling here is based on the metadata set
	// by the client library. Which, is turn, is based on agent hints,
	// but the rule of thumb is: respect client choice.
	sampled := samplingPriority > 0

	// Short-circuit and return without counting the trace in the sampling rate logic
	// if its value has not been set automaticallt by the client lib.
	// The feedback loop should be scoped to the values it can act upon.
	if samplingPriority < 0 {
		return sampled
	}
	if samplingPriority > 1 {
		return sampled
	}
	// short-circuiting root P0 trace chunks that the client passed. The sig is already taken in account
	// in CountWeightedSig below. This chunk will likely be sampled by the ExceptionSampler
	if clientDroppedP0s && samplingPriority == 0 && root.ParentID == 0 {
		return sampled
	}

	signature := s.catalog.register(ServiceSignature{Name: root.Service, Env: env})

	// Update sampler state by counting this trace
	s.CountSignature(root, signature)

	if sampled {
		rate := s.applyRate(sampled, root, signature)
		s.CountSampled(root, clientDroppedP0s, signature, rate)
	}
	return sampled
}

// CountSignature counts all chunks received with local chunk root signature.
func (s *PrioritySampler) CountSignature(root *pb.Span, signature Signature) {
	s.localRates.Backend.CountSignature(signature)

	// remoteRates only considers root spans
	if s.remoteRates != nil && root.ParentID == 0 {
		s.remoteRates.CountSignature(signature)
	}
}

// CountSampled counts sampled chunks with local chunk root signature.
func (s *PrioritySampler) CountSampled(root *pb.Span, clientDroppedP0s bool, signature Signature, rate float64) {
	s.localRates.Backend.CountSample()
	// adjust sig score with the expected P0 count for that sig
	// adjusting this score only matters for root spans
	if clientDroppedP0s && rate > 0 && rate < 1 {
		// removing 1 to not count twice the P1 chunk
		weight := 1/rate - 1
		s.localRates.Backend.CountWeightedSig(signature, weight)
	}

	// remoteRates only considers root spans
	if s.remoteRates != nil && root.ParentID == 0 {
		s.remoteRates.CountSample(root, signature)
		if clientDroppedP0s && rate > 0 && rate < 1 {
			// removing 1 to not count twice the P1 chunk
			weight := 1/rate - 1
			s.remoteRates.CountWeightedSig(signature, weight)
		}
	}
}

// CountClientDroppedP0s counts client dropped traces. They are added
// to the totalScore, allowing them to weight on sampling rates during
// adjust calls
func (s *PrioritySampler) CountClientDroppedP0s(dropped int64) {
	s.localRates.Backend.AddTotalScore(float64(dropped))
}

func (s *PrioritySampler) applyRate(sampled bool, root *pb.Span, signature Signature) float64 {
	if root.ParentID != 0 {
		return 1.0
	}
	// recent tracers annotate roots with applied priority rate
	// agentRateKey is set when the agent computed rate is applied
	if rate, ok := getMetric(root, agentRateKey); ok {
		return rate
	}
	// ruleRateKey is set when a tracer rule rate is applied
	if rate, ok := getMetric(root, ruleRateKey); ok {
		return rate
	}

	// slow path used by older tracer versions
	// dd-trace-go used to set the rate in deprecatedRateKey
	if rate, ok := getMetric(root, deprecatedRateKey); ok {
		return rate
	}
	if s.remoteRates != nil {
		// use a remote rate, if available
		rate, ok := s.remoteRates.GetSignatureSampleRate(signature)
		if ok {
			setMetric(root, deprecatedRateKey, rate)
			return rate
		}
	}
	// Use the rate from the default local feedback loop
	rate := s.localRates.GetSignatureSampleRate(signature)
	if rate > priorityLocalRateThresholdTo1 {
		rate = 1
	}
	setMetric(root, deprecatedRateKey, rate)
	return rate
}

// ratesByService returns all rates by service, this information is useful for
// agents to pick the right service rate.
func (s *PrioritySampler) ratesByService() map[ServiceSignature]float64 {
	var remoteRates map[Signature]float64
	if s.remoteRates != nil {
		remoteRates = s.remoteRates.GetAllSignatureSampleRates()
	}
	localRates := s.localRates.GetAllSignatureSampleRates()
	return s.catalog.ratesByService(localRates, remoteRates, s.localRates.GetDefaultSampleRate())
}
