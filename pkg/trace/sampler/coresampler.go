// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"math"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/atomic"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
)

const (
	decayPeriod time.Duration = 5 * time.Second
	// With this factor, any past trace counts for less than 50% after 6*decayPeriod and >1% after 39*decayPeriod
	// We can keep it hardcoded, but having `decayPeriod` configurable should be enough?
	defaultDecayFactor          float64 = 1.125 // 9/8
	initialSignatureScoreOffset float64 = 1
	minSignatureScoreOffset     float64 = 0.01
	defaultSignatureScoreSlope  float64 = 3
)

// Sampler is the main component of the sampling logic
type Sampler struct {
	// Storage of the state of the sampler
	Backend *MemoryBackend

	// Extra sampling rate to combine to the existing sampling
	extraRate float64
	// Maximum limit to the total number of traces per second to sample
	targetTPS *atomic.Float64

	// Sample any signature with a score lower than scoreSamplingOffset
	// It is basically the number of similar traces per second after which we start sampling
	signatureScoreOffset *atomic.Float64
	// Logarithm slope for the scoring function
	signatureScoreSlope *atomic.Float64
	// signatureScoreFactor = math.Pow(signatureScoreSlope, math.Log10(scoreSamplingOffset))
	signatureScoreFactor *atomic.Float64

	tags    []string
	exit    chan struct{}
	stopped chan struct{}
}

// newSampler returns an initialized Sampler
func newSampler(extraRate float64, targetTPS float64, tags []string) *Sampler {
	s := &Sampler{
		Backend:              NewMemoryBackend(decayPeriod, defaultDecayFactor),
		extraRate:            extraRate,
		targetTPS:            atomic.NewFloat(targetTPS),
		signatureScoreOffset: atomic.NewFloat(0),
		signatureScoreSlope:  atomic.NewFloat(0),
		signatureScoreFactor: atomic.NewFloat(0),
		tags:                 tags,

		exit:    make(chan struct{}),
		stopped: make(chan struct{}),
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

// UpdateTargetTPS updates the max TPS limit
func (s *Sampler) UpdateTargetTPS(targetTPS float64) {
	s.targetTPS.Store(targetTPS)
}

// Start runs and the Sampler main loop
func (s *Sampler) Start() {
	go func() {
		defer watchdog.LogOnPanic()
		decayTicker := time.NewTicker(s.Backend.DecayPeriod)
		statsTicker := time.NewTicker(10 * time.Second)
		defer decayTicker.Stop()
		defer statsTicker.Stop()
		for {
			select {
			case <-decayTicker.C:
				s.update()
			case <-statsTicker.C:
				s.report()
			case <-s.exit:
				close(s.stopped)
				return
			}
		}
	}()
}

// update decays scores and rate computation coefficients
func (s *Sampler) update() {
	s.Backend.DecayScore()
	s.AdjustScoring()
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
	return s.GetSignatureSampleRate(signature) * s.extraRate
}

// GetTargetTPSSampleRate returns an extra sample rate to apply if we are above targetTPS.
func (s *Sampler) GetTargetTPSSampleRate() float64 {
	// When above targetTPS, apply an additional sample rate to statistically respect the limit
	targetTPSrate := 1.0
	configuredTargetTPS := s.targetTPS.Load()
	if configuredTargetTPS > 0 {
		currentTPS := s.Backend.GetUpperSampledScore()
		if currentTPS > configuredTargetTPS {
			targetTPSrate = configuredTargetTPS / currentTPS
		}
	}

	return targetTPSrate
}
