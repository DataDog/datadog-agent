// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package sampler

import (
	"math"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/exportable/atomic"
	"github.com/DataDog/datadog-agent/pkg/trace/exportable/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/exportable/watchdog"
)

const (
	// Sampler parameters not (yet?) configurable
	defaultDecayPeriod time.Duration = 5 * time.Second
	// With this factor, any past trace counts for less than 50% after 6*decayPeriod and >1% after 39*decayPeriod
	// We can keep it hardcoded, but having `decayPeriod` configurable should be enough?
	defaultDecayFactor          float64       = 1.125 // 9/8
	adjustPeriod                time.Duration = 10 * time.Second
	initialSignatureScoreOffset float64       = 1
	minSignatureScoreOffset     float64       = 0.01
	defaultSignatureScoreSlope  float64       = 3
	// defaultSamplingRateThresholdTo1 defines the maximum allowed sampling rate below 1.
	// If this is surpassed, the rate is set to 1.
	defaultSamplingRateThresholdTo1 float64 = 1
)

// EngineType represents the type of a sampler engine.
type EngineType int

const (
	// NormalScoreEngineType is the type of the ScoreEngine sampling non-error traces.
	NormalScoreEngineType EngineType = iota
	// ErrorsScoreEngineType is the type of the ScoreEngine sampling error traces.
	ErrorsScoreEngineType
	// PriorityEngineType is type of the priority sampler engine type.
	PriorityEngineType
)

// Engine is a common basic interface for sampler engines.
type Engine interface {
	// Run the sampler.
	Run()
	// Stop the sampler.
	Stop()
	// Sample a trace.
	Sample(trace pb.Trace, root *pb.Span, env string) (sampled bool, samplingRate float64)
	// GetState returns information about the sampler.
	GetState() interface{}
	// GetType returns the type of the sampler.
	GetType() EngineType
}

// Sampler is the main component of the sampling logic
type Sampler struct {
	// Storage of the state of the sampler
	Backend Backend

	// Extra sampling rate to combine to the existing sampling
	extraRate float64
	// Maximum limit to the total number of traces per second to sample
	maxTPS float64
	// rateThresholdTo1 is the value above which all computed sampling rates will be set to 1
	rateThresholdTo1 float64

	// Sample any signature with a score lower than scoreSamplingOffset
	// It is basically the number of similar traces per second after which we start sampling
	signatureScoreOffset *atomic.Float64
	// Logarithm slope for the scoring function
	signatureScoreSlope *atomic.Float64
	// signatureScoreFactor = math.Pow(signatureScoreSlope, math.Log10(scoreSamplingOffset))
	signatureScoreFactor *atomic.Float64

	exit chan struct{}
}

// newSampler returns an initialized Sampler
func newSampler(extraRate float64, maxTPS float64) *Sampler {
	s := &Sampler{
		Backend:              NewMemoryBackend(defaultDecayPeriod, defaultDecayFactor),
		extraRate:            extraRate,
		maxTPS:               maxTPS,
		rateThresholdTo1:     defaultSamplingRateThresholdTo1,
		signatureScoreOffset: atomic.NewFloat(0),
		signatureScoreSlope:  atomic.NewFloat(0),
		signatureScoreFactor: atomic.NewFloat(0),

		exit: make(chan struct{}),
	}

	s.SetSignatureCoefficients(initialSignatureScoreOffset, defaultSignatureScoreSlope)

	return s
}

// SetSignatureCoefficients updates the internal scoring coefficients used by the signature scoring
func (s *Sampler) SetSignatureCoefficients(offset float64, slope float64) {
	s.signatureScoreOffset.Store(offset)
	s.signatureScoreSlope.Store(slope)
	s.signatureScoreFactor.Store(math.Pow(slope, math.Log10(offset)))
}

// UpdateExtraRate updates the extra sample rate
func (s *Sampler) UpdateExtraRate(extraRate float64) {
	s.extraRate = extraRate
}

// UpdateMaxTPS updates the max TPS limit
func (s *Sampler) UpdateMaxTPS(maxTPS float64) {
	s.maxTPS = maxTPS
}

// Run runs and block on the Sampler main loop
func (s *Sampler) Run() {
	go func() {
		defer watchdog.LogOnPanic()
		s.Backend.Run()
	}()
	s.RunAdjustScoring()
}

// Stop stops the main Run loop
func (s *Sampler) Stop() {
	s.Backend.Stop()
	close(s.exit)
}

// RunAdjustScoring is the sampler feedback loop to adjust the scoring coefficients
func (s *Sampler) RunAdjustScoring() {
	t := time.NewTicker(adjustPeriod)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			s.AdjustScoring()
		case <-s.exit:
			return
		}
	}
}

// GetSampleRate returns the sample rate to apply to a trace.
func (s *Sampler) GetSampleRate(trace pb.Trace, root *pb.Span, signature Signature) float64 {
	return s.loadRate(s.GetSignatureSampleRate(signature) * s.extraRate)
}

// GetMaxTPSSampleRate returns an extra sample rate to apply if we are above maxTPS.
func (s *Sampler) GetMaxTPSSampleRate() float64 {
	// When above maxTPS, apply an additional sample rate to statistically respect the limit
	maxTPSrate := 1.0
	if s.maxTPS > 0 {
		currentTPS := s.Backend.GetUpperSampledScore()
		if currentTPS > s.maxTPS {
			maxTPSrate = s.maxTPS / currentTPS
		}
	}

	return maxTPSrate
}

func (s *Sampler) setRateThresholdTo1(r float64) {
	s.rateThresholdTo1 = r
}

// CombineRates merges two rates from Sampler1, Sampler2. Both samplers law are independent,
// and {sampled} = {sampled by Sampler1} or {sampled by Sampler2}
func CombineRates(rate1 float64, rate2 float64) float64 {
	if rate1 >= 1 || rate2 >= 1 {
		return 1
	}
	return rate1 + rate2 - rate1*rate2
}
