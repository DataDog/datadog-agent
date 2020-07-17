// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package sampler

import (
	"math"
)

const (
	// 2^64 - 1
	maxTraceID      = ^uint64(0)
	maxTraceIDFloat = float64(maxTraceID)
	// Good number for Knuth hashing (large, prime, fit in int64 for languages without uint64)
	samplerHasher = uint64(1111111111111111111)
)

// SampleByRate tells if a trace (from its ID) with a given rate should be sampled
// Use Knuth multiplicative hashing to leverage imbalanced traceID generators
func SampleByRate(traceID uint64, rate float64) bool {
	if rate < 1 {
		return traceID*samplerHasher < uint64(rate*maxTraceIDFloat)
	}
	return true
}

// GetSignatureSampleRate gives the sample rate to apply to any signature.
// For now, only based on count score.
func (s *Sampler) GetSignatureSampleRate(signature Signature) float64 {
	return s.loadRate(s.GetCountScore(signature))
}

// GetAllSignatureSampleRates gives the sample rate to apply to all signatures.
// For now, only based on count score.
func (s *Sampler) GetAllSignatureSampleRates() map[Signature]float64 {
	m := s.GetAllCountScores()
	for k, v := range m {
		m[k] = s.loadRate(v)
	}
	return m
}

// GetDefaultSampleRate gives the sample rate to apply to an unknown signature.
// For now, only based on count score.
func (s *Sampler) GetDefaultSampleRate() float64 {
	return s.loadRate(s.GetDefaultCountScore())
}

func (s *Sampler) loadRate(rate float64) float64 {
	if rate >= s.rateThresholdTo1 {
		return 1
	}
	return rate
}

func (s *Sampler) backendScoreToSamplerScore(score float64) float64 {
	return s.signatureScoreFactor.Load() / math.Pow(s.signatureScoreSlope.Load(), math.Log10(score))
}

// GetCountScore scores any signature based on its recent throughput
// The score value can be seeing as the sample rate if the count were the only factor
// Since other factors can intervene (such as extra global sampling), its value can be larger than 1
func (s *Sampler) GetCountScore(signature Signature) float64 {
	return s.backendScoreToSamplerScore(s.Backend.GetSignatureScore(signature))
}

// GetAllCountScores scores all signatures based on their recent throughput
// The score value can be seeing as the sample rate if the count were the only factor
// Since other factors can intervene (such as extra global sampling), its value can be larger than 1
func (s *Sampler) GetAllCountScores() map[Signature]float64 {
	m := s.Backend.GetSignatureScores()
	for k, v := range m {
		m[k] = s.backendScoreToSamplerScore(v)
	}
	return m
}

// GetDefaultCountScore returns a default score when not knowing the signature for real.
// Since other factors can intervene (such as extra global sampling), its value can be larger than 1
func (s *Sampler) GetDefaultCountScore() float64 {
	return s.backendScoreToSamplerScore(s.Backend.GetTotalScore())
}
