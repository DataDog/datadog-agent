// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"math"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/export/atomic"
	"github.com/DataDog/datadog-agent/pkg/trace/export/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/export/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/export/watchdog"
)

const (
	// Sampler parameters not (yet?) configurable

	// DefaultDecayPeriod defines the default decayPeriod
	DefaultDecayPeriod time.Duration = 5 * time.Second
	// DefaultDecayFactor with this factor, any past trace counts for less than 50% after 6*decayPeriod and >1% after 39*decayPeriod
	// We can keep it hardcoded, but having `decayPeriod` configurable should be enough?
	DefaultDecayFactor          float64       = 1.125 // 9/8
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

// Sampler is the main component of the sampling logic
type Sampler struct {
	// Storage of the state of the sampler
	Backend *MemoryBackend

	// Extra sampling rate to combine to the existing sampling
	ExtraRate float64
	// Maximum limit to the total number of traces per second to sample
	TargetTPS float64
	// RateThresholdTo1 is the value above which all computed sampling rates will be set to 1
	RateThresholdTo1 float64

	// Sample any signature with a score lower than scoreSamplingOffset
	// It is basically the number of similar traces per second after which we start sampling
	SignatureScoreOffset *atomic.Float64
	// Logarithm slope for the scoring function
	SignatureScoreSlope *atomic.Float64
	// SignatureScoreFactor = math.Pow(SignatureScoreSlope, math.Log10(scoreSamplingOffset))
	SignatureScoreFactor *atomic.Float64

	tags    []string
	exit    chan struct{}
	stopped chan struct{}
}

// NewSampler returns an initialized Sampler
func NewSampler(extraRate float64, targetTPS float64, tags []string) *Sampler {
	s := &Sampler{
		Backend:              NewMemoryBackend(DefaultDecayPeriod, DefaultDecayFactor),
		ExtraRate:            extraRate,
		TargetTPS:            targetTPS,
		RateThresholdTo1:     defaultSamplingRateThresholdTo1,
		SignatureScoreOffset: atomic.NewFloat(0),
		SignatureScoreSlope:  atomic.NewFloat(0),
		SignatureScoreFactor: atomic.NewFloat(0),
		tags:                 tags,

		exit:    make(chan struct{}),
		stopped: make(chan struct{}),
	}

	s.SetSignatureCoefficients(initialSignatureScoreOffset, defaultSignatureScoreSlope)

	return s
}

// SetSignatureCoefficients updates the internal scoring coefficients used by the signature scoring
func (s *Sampler) SetSignatureCoefficients(offset float64, slope float64) {
	s.SignatureScoreOffset.Store(offset)
	s.SignatureScoreSlope.Store(slope)
	s.SignatureScoreFactor.Store(math.Pow(slope, math.Log10(offset)))
}

// UpdateExtraRate updates the extra sample rate
func (s *Sampler) UpdateExtraRate(extraRate float64) {
	s.ExtraRate = extraRate
}

// UpdateTargetTPS updates the max TPS limit
func (s *Sampler) UpdateTargetTPS(targetTPS float64) {
	s.TargetTPS = targetTPS
}

// Start runs and the Sampler main loop
func (s *Sampler) Start() {
	go func() {
		defer watchdog.LogOnPanic()
		decayTicker := time.NewTicker(s.Backend.DecayPeriod)
		adjustTicker := time.NewTicker(adjustPeriod)
		statsTicker := time.NewTicker(10 * time.Second)
		defer decayTicker.Stop()
		defer adjustTicker.Stop()
		defer statsTicker.Stop()
		for {
			select {
			case <-decayTicker.C:
				s.Backend.DecayScore()
			case <-adjustTicker.C:
				s.AdjustScoring()
			case <-statsTicker.C:
				s.report()
			case <-s.exit:
				close(s.stopped)
				return
			}
		}
	}()
}

func (s *Sampler) report() {
	kept, seen := s.Backend.report()
	metrics.Count("datadog.trace_agent.sampler.kept", kept, s.tags, 1)
	metrics.Count("datadog.trace_agent.sampler.seen", seen, s.tags, 1)
}

// Stop stops the main Run loop
func (s *Sampler) Stop() {
	close(s.exit)
	<-s.stopped
}

// GetSampleRate returns the sample rate to apply to a trace.
func (s *Sampler) GetSampleRate(trace pb.Trace, root *pb.Span, signature Signature) float64 {
	return s.loadRate(s.GetSignatureSampleRate(signature) * s.ExtraRate)
}

// GetTargetTPSSampleRate returns an extra sample rate to apply if we are above targetTPS.
func (s *Sampler) GetTargetTPSSampleRate() float64 {
	// When above targetTPS, apply an additional sample rate to statistically respect the limit
	targetTPSrate := 1.0
	if s.TargetTPS > 0 {
		currentTPS := s.Backend.GetUpperSampledScore()
		if currentTPS > s.TargetTPS {
			targetTPSrate = s.TargetTPS / currentTPS
		}
	}

	return targetTPSrate
}

// SetRateThresholdTo1 sets the RateThresholdTo1
func (s *Sampler) SetRateThresholdTo1(r float64) {
	s.RateThresholdTo1 = r
}
