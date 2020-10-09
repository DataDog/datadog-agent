// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package sampler

import "github.com/DataDog/datadog-agent/pkg/trace/exportable/pb"

const (
	// errorSamplingRateThresholdTo1 defines the maximum allowed sampling rate below 1.
	// If this is surpassed, the rate is set to 1.
	errorSamplingRateThresholdTo1 = 0.1
)

// ScoreEngine is the main component of the sampling logic
type ScoreEngine struct {
	// Sampler is the underlying sampler used by this engine, sharing logic among various engines.
	Sampler    *Sampler
	engineType EngineType
}

// NewScoreEngine returns an initialized Sampler
func NewScoreEngine(extraRate float64, maxTPS float64) *ScoreEngine {
	s := &ScoreEngine{
		Sampler:    newSampler(extraRate, maxTPS),
		engineType: NormalScoreEngineType,
	}

	return s
}

// NewErrorsEngine returns an initialized Sampler dedicate to errors. It behaves
// just like the the normal ScoreEngine except for its GetType method (useful
// for reporting).
func NewErrorsEngine(extraRate float64, maxTPS float64) *ScoreEngine {
	s := &ScoreEngine{
		Sampler:    newSampler(extraRate, maxTPS),
		engineType: ErrorsScoreEngineType,
	}
	s.Sampler.setRateThresholdTo1(errorSamplingRateThresholdTo1)

	return s
}

// Run runs and block on the Sampler main loop
func (s *ScoreEngine) Run() {
	s.Sampler.Run()
}

// Stop stops the main Run loop
func (s *ScoreEngine) Stop() {
	s.Sampler.Stop()
}

func applySampleRate(root *pb.Span, rate float64) bool {
	initialRate := GetGlobalRate(root)
	newRate := initialRate * rate
	traceID := root.TraceID
	return SampleByRate(traceID, newRate)
}

// Sample counts an incoming trace and tells if it is a sample which has to be kept
func (s *ScoreEngine) Sample(trace pb.Trace, root *pb.Span, env string) (sampled bool, rate float64) {
	// Extra safety, just in case one trace is empty
	if len(trace) == 0 {
		return false, 0
	}

	signature := computeSignatureWithRootAndEnv(trace, root, env)

	// Update sampler state by counting this trace
	s.Sampler.Backend.CountSignature(signature)

	rate = s.Sampler.GetSampleRate(trace, root, signature)

	sampled = applySampleRate(root, rate)

	if sampled {
		// Count the trace to allow us to check for the maxTPS limit.
		// It has to happen before the maxTPS sampling.
		s.Sampler.Backend.CountSample()

		// Check for the maxTPS limit, and if we require an extra sampling.
		// No need to check if we already decided not to keep the trace.
		maxTPSrate := s.Sampler.GetMaxTPSSampleRate()
		if maxTPSrate < 1 {
			sampled = applySampleRate(root, maxTPSrate)
		}
	}

	return sampled, rate
}

// GetState collects and return internal statistics and coefficients for indication purposes
// It returns an interface{}, as other samplers might return other informations.
func (s *ScoreEngine) GetState() interface{} {
	return s.Sampler.GetState()
}

// GetType returns the type of the sampler
func (s *ScoreEngine) GetType() EngineType {
	return s.engineType
}
