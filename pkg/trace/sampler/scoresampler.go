// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

const (
	errorsRateKey     = "_dd.errors_sr"
	noPriorityRateKey = "_dd.no_p_sr"
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
}

// NewNoPrioritySampler returns an initialized Sampler dedicated to traces with
// no priority set.
func NewNoPrioritySampler(conf *config.AgentConfig) *NoPrioritySampler {
	s := newSampler(conf.ExtraSampleRate, conf.TargetTPS, []string{"sampler:no_priority"})
	return &NoPrioritySampler{ScoreSampler{Sampler: s, samplingRateKey: noPriorityRateKey}}
}

// NewErrorsSampler returns an initialized Sampler dedicate to errors. It behaves
// just like the the normal ScoreEngine except for its GetType method (useful
// for reporting).
func NewErrorsSampler(conf *config.AgentConfig) *ErrorsSampler {
	s := newSampler(conf.ExtraSampleRate, conf.ErrorTPS, []string{"sampler:error"})
	return &ErrorsSampler{ScoreSampler{Sampler: s, samplingRateKey: errorsRateKey, disabled: conf.ErrorTPS == 0}}
}

// Sample counts an incoming trace and tells if it is a sample which has to be kept
func (s ScoreSampler) Sample(now time.Time, trace pb.Trace, root *pb.Span, env string) bool {
	if s.disabled {
		return false
	}

	// Extra safety, just in case one trace is empty
	if len(trace) == 0 {
		return false
	}
	signature := computeSignatureWithRootAndEnv(trace, root, env)
	// Update sampler state by counting this trace
	s.countWeightedSig(now, signature, 1)

	rate := s.getSignatureSampleRate(signature)

	return s.applySampleRate(root, rate)
}

func (s ScoreSampler) applySampleRate(root *pb.Span, rate float64) bool {
	initialRate := GetGlobalRate(root)
	newRate := initialRate * rate
	traceID := root.TraceID
	sampled := SampleByRate(traceID, newRate)
	if sampled {
		setMetric(root, s.samplingRateKey, rate)
	}
	return sampled
}
