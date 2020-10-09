// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

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
	"math"

	"github.com/DataDog/datadog-agent/pkg/trace/exportable/pb"
)

const (
	// KeySamplingRateGlobal is a metric key holding the global sampling rate.
	KeySamplingRateGlobal = "_sample_rate"

	// KeySamplingRateClient is a metric key holding the client-set sampling rate for APM events.
	KeySamplingRateClient = "_dd1.sr.rcusr"

	// KeySamplingRatePreSampler is a metric key holding the API rate limiter's rate for APM events.
	KeySamplingRatePreSampler = "_dd1.sr.rapre"

	// KeySamplingRateEventExtraction is the key of the metric storing the event extraction rate on an APM event.
	KeySamplingRateEventExtraction = "_dd1.sr.eausr"

	// KeySamplingRateMaxEPSSampler is the key of the metric storing the max eps sampler rate on an APM event.
	KeySamplingRateMaxEPSSampler = "_dd1.sr.eamax"

	// KeySamplingPriority is the key of the sampling priority value in the metrics map of the root span
	KeySamplingPriority = "_sampling_priority_v1"

	// KeyErrorType is the key of the error type in the meta map
	KeyErrorType = "error.type"

	// KeyHTTPStatusCode is the key of the http status code in the meta map
	KeyHTTPStatusCode = "http.status_code"
)

// SamplingPriority is the type encoding a priority sampling decision.
type SamplingPriority int8

const (
	// PriorityNone is the value for SamplingPriority when no priority sampling decision could be found.
	PriorityNone SamplingPriority = math.MinInt8

	// PriorityUserDrop is the value set by a user to explicitly drop a trace.
	PriorityUserDrop SamplingPriority = -1

	// PriorityAutoDrop is the value set by a tracer to suggest dropping a trace.
	PriorityAutoDrop SamplingPriority = 0

	// PriorityAutoKeep is the value set by a tracer to suggest keeping a trace.
	PriorityAutoKeep SamplingPriority = 1

	// PriorityUserKeep is the value set by a user to explicitly keep a trace.
	PriorityUserKeep SamplingPriority = 2
)

// GetSamplingPriority returns the value of the sampling priority metric set on this span and a boolean indicating if
// such a metric was actually found or not.
func GetSamplingPriority(s *pb.Span) (SamplingPriority, bool) {
	p, ok := getMetric(s, KeySamplingPriority)
	return SamplingPriority(p), ok
}

// SetSamplingPriority sets the sampling priority value on this span, overwriting any previously set value.
func SetSamplingPriority(s *pb.Span, priority SamplingPriority) {
	setMetric(s, KeySamplingPriority, float64(priority))
}

// GetGlobalRate gets the cumulative sample rate of the trace to which this span belongs to.
func GetGlobalRate(s *pb.Span) float64 {
	return getMetricDefault(s, KeySamplingRateGlobal, 1.0)
}

// SetGlobalRate sets the cumulative sample rate of the trace to which this span belongs to.
func SetGlobalRate(s *pb.Span, rate float64) {
	setMetric(s, KeySamplingRateGlobal, rate)
}

// AddGlobalRate updates the cumulative sample rate of the trace to which this span belongs to with the provided
// rate which is assumed to belong to an independent sampler. The combination is done by simple multiplications.
func AddGlobalRate(s *pb.Span, rate float64) {
	setMetric(s, KeySamplingRateGlobal, GetGlobalRate(s)*rate)
}

// GetClientRate gets the rate at which the trace this span belongs to was sampled by the tracer.
// NOTE: This defaults to 1 if no rate is stored.
func GetClientRate(s *pb.Span) float64 {
	return getMetricDefault(s, KeySamplingRateClient, 1.0)
}

// SetClientRate sets the rate at which the trace this span belongs to was sampled by the tracer.
func SetClientRate(s *pb.Span, rate float64) {
	if rate < 1 {
		setMetric(s, KeySamplingRateClient, rate)
	} else {
		// We assume missing value is 1 to save bandwidth (check getter).
		delete(s.Metrics, KeySamplingRateClient)
	}
}

// GetPreSampleRate returns the rate at which the trace this span belongs to was sampled by the agent's presampler.
// NOTE: This defaults to 1 if no rate is stored.
func GetPreSampleRate(s *pb.Span) float64 {
	return getMetricDefault(s, KeySamplingRatePreSampler, 1.0)
}

// SetPreSampleRate sets the rate at which the trace this span belongs to was sampled by the agent's presampler.
func SetPreSampleRate(s *pb.Span, rate float64) {
	if rate < 1 {
		setMetric(s, KeySamplingRatePreSampler, rate)
	} else {
		// We assume missing value is 1 to save bandwidth (check getter).
		delete(s.Metrics, KeySamplingRatePreSampler)
	}
}

// GetEventExtractionRate gets the rate at which the trace from which we extracted this event was sampled at the tracer.
// This defaults to 1 if no rate is stored.
func GetEventExtractionRate(s *pb.Span) float64 {
	return getMetricDefault(s, KeySamplingRateEventExtraction, 1.0)
}

// SetEventExtractionRate sets the rate at which the trace from which we extracted this event was sampled at the tracer.
func SetEventExtractionRate(s *pb.Span, rate float64) {
	if rate < 1 {
		setMetric(s, KeySamplingRateEventExtraction, rate)
	} else {
		// reduce bandwidth, default is assumed 1.0 in backend
		delete(s.Metrics, KeySamplingRateEventExtraction)
	}
}

// GetMaxEPSRate gets the rate at which this event was sampled by the max eps event sampler.
func GetMaxEPSRate(s *pb.Span) float64 {
	return getMetricDefault(s, KeySamplingRateMaxEPSSampler, 1.0)
}

// SetMaxEPSRate sets the rate at which this event was sampled by the max eps event sampler.
func SetMaxEPSRate(s *pb.Span, rate float64) {
	if rate < 1 {
		setMetric(s, KeySamplingRateMaxEPSSampler, rate)
	} else {
		// reduce bandwidth, default is assumed 1.0 in backend
		delete(s.Metrics, KeySamplingRateMaxEPSSampler)
	}
}

func getMetric(s *pb.Span, k string) (float64, bool) {
	if s.Metrics == nil {
		return 0, false
	}
	val, ok := s.Metrics[k]
	return val, ok
}

// getMetricDefault gets a value in the span Metrics map or default if no value is stored there.
func getMetricDefault(s *pb.Span, k string, def float64) float64 {
	if val, ok := getMetric(s, k); ok {
		return val
	}
	return def
}

// setMetric sets a value in the span Metrics map.
func setMetric(s *pb.Span, key string, val float64) {
	if s.Metrics == nil {
		s.Metrics = make(map[string]float64)
	}
	s.Metrics[key] = val
}
