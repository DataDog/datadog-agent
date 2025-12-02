// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"sync"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	errorsRateKey     = "_dd.errors_sr"
	noPriorityRateKey = "_dd.no_p_sr"
	// shrinkCardinality is the max Signature cardinality before shrinking
	shrinkCardinality = 200
)

// ErrorsSampler is dedicated to catching traces containing spans with errors.
type ErrorsSampler struct{ ScoreSampler }

// NoPrioritySampler is dedicated to catching traces with no priority set.
type NoPrioritySampler struct{ ScoreSampler }

// ScoreSampler samples pieces of traces by computing a signature based on spans (service, name, rsc, http.status, error.type)
// scoring it and applying a rate.
// The rates are applied on the TraceID to maximize the number of chunks with errors caught for the same traceID.
// For a set traceID: P(chunk1 kept and chunk2 kept) = min(P(chunk1 kept), P(chunk2 kept))
type ScoreSampler struct {
	*Sampler
	samplingRateKey string
	disabled        bool
	mu              sync.Mutex
	shrinkAllowList map[Signature]float64
}

// NewNoPrioritySampler returns an initialized Sampler dedicated to traces with
// no priority set.
func NewNoPrioritySampler(conf *config.AgentConfig) *NoPrioritySampler {
	s := newSampler(conf.ExtraSampleRate, conf.TargetTPS)
	return &NoPrioritySampler{ScoreSampler{Sampler: s, samplingRateKey: noPriorityRateKey}}
}

var _ AdditionalMetricsReporter = (*NoPrioritySampler)(nil)

func (s *NoPrioritySampler) report(statsd statsd.ClientInterface) {
	s.Sampler.report(statsd, NameNoPriority)
}

// NewErrorsSampler returns an initialized Sampler dedicate to errors. It behaves
// just like the normal ScoreEngine except for its GetType method (useful
// for reporting).
func NewErrorsSampler(conf *config.AgentConfig) *ErrorsSampler {
	s := newSampler(conf.ExtraSampleRate, conf.ErrorTPS)
	return &ErrorsSampler{ScoreSampler{Sampler: s, samplingRateKey: errorsRateKey, disabled: conf.ErrorTPS == 0}}
}

var _ AdditionalMetricsReporter = (*ErrorsSampler)(nil)

func (s *ErrorsSampler) report(statsd statsd.ClientInterface) {
	s.Sampler.report(statsd, NameError)
}

// Sample counts an incoming trace and tells if it is a sample which has to be kept
func (s *ScoreSampler) Sample(now time.Time, trace pb.Trace, root *pb.Span, env string) bool {
	if s.disabled {
		return false
	}

	// Extra safety, just in case one trace is empty
	if len(trace) == 0 {
		return false
	}
	signature := computeSignatureWithRootAndEnv(trace, root, env)
	signature = s.shrink(signature)
	// Update sampler state by counting this trace
	s.countWeightedSig(now, signature, weightRoot(root))

	rate := s.getSignatureSampleRate(signature)

	sampled := s.applySampleRate(root, rate)
	return sampled
}

// SampleV1 counts an incoming trace and tells if it is a sample which has to be kept
func (s *ScoreSampler) SampleV1(now time.Time, chunk *idx.InternalTraceChunk, root *idx.InternalSpan, env string) bool {
	if s.disabled {
		return false
	}

	// Extra safety, just in case one trace is empty
	if len(chunk.Spans) == 0 {
		return false
	}
	signature := computeSignatureWithRootAndEnvV1(chunk, root, env)
	signature = s.shrink(signature)
	// Update sampler state by counting this trace
	s.countWeightedSig(now, signature, weightRootV1(root))

	rate := s.getSignatureSampleRate(signature)

	sampled := s.applySampleRateV1(root, chunk.LegacyTraceID(), rate)
	return sampled
}

// UpdateTargetTPS updates the target tps
func (s *ScoreSampler) UpdateTargetTPS(targetTPS float64) {
	s.Sampler.updateTargetTPS(targetTPS)
}

// GetTargetTPS returns the target tps
func (s *ScoreSampler) GetTargetTPS() float64 {
	return s.Sampler.targetTPS.Load()
}

func (s *ScoreSampler) applySampleRate(root *pb.Span, rate float64) bool {
	initialRate := GetGlobalRate(root)
	newRate := initialRate * rate
	traceID := root.TraceID
	sampled := SampleByRate(traceID, newRate)
	if sampled {
		setMetric(root, s.samplingRateKey, rate)
	}
	return sampled
}

// We use the legacy traceID here for backwards compatibility with any older version of the agent
func (s *ScoreSampler) applySampleRateV1(root *idx.InternalSpan, traceID uint64, rate float64) bool {
	initialRate := GetGlobalRateV1(root)
	newRate := initialRate * rate
	sampled := SampleByRate(traceID, newRate)
	if sampled {
		root.SetFloat64Attribute(s.samplingRateKey, rate)
	}
	return sampled
}

// shrink limits the number of signatures stored in the sampler.
// After a cardinality above shrinkCardinality/2 is reached
// signatures are spread uniformly on a fixed set of values.
// This ensures that ScoreSamplers are memory capped.
// When the shrink is triggered, previously active signatures
// stay unaffected.
// New signatures may share the same TPS computation.
func (s *ScoreSampler) shrink(sig Signature) Signature {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.size() < shrinkCardinality/2 {
		s.shrinkAllowList = nil
		return sig
	}
	if s.shrinkAllowList == nil {
		rates, _ := s.getAllSignatureSampleRates()
		s.shrinkAllowList = rates
	}
	if _, ok := s.shrinkAllowList[sig]; ok {
		return sig
	}
	return sig % (shrinkCardinality / 2)
}
