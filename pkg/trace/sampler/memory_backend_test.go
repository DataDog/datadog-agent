// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package sampler

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func getTestBackend() *MemoryBackend {
	decayPeriod := 5 * time.Second

	return NewMemoryBackend(decayPeriod, defaultDecayFactor)
}

func randomSignature() Signature {
	return Signature(rand.Int63())
}

func TestBasicNewBackend(t *testing.T) {
	assert := assert.New(t)

	backend := getTestBackend()

	sign := randomSignature()
	backend.CountSignature(sign)

	assert.True(backend.GetSignatureScore(sign) > 0.0)
	assert.Equal(0.0, backend.GetSignatureScore(randomSignature()))
}

func TestCountScoreConvergence(t *testing.T) {
	// With a constant number of tracesPerPeriod, the backend score should converge to tracesPerPeriod
	// Test the convergence of both signature and total sampled counters
	backend := getTestBackend()

	sign := randomSignature()

	periods := 50
	tracesPerPeriod := 1000
	period := backend.decayPeriod

	for period := 0; period < periods; period++ {
		backend.decayScore()
		for i := 0; i < tracesPerPeriod; i++ {
			backend.CountSignature(sign)
			backend.CountSample()
		}
	}

	assert.InEpsilon(t, backend.GetSignatureScore(sign), float64(tracesPerPeriod)/period.Seconds(), 0.01)
	assert.InEpsilon(t, backend.GetSampledScore(), float64(tracesPerPeriod)/period.Seconds(), 0.01)
}

func TestCountScoreOblivion(t *testing.T) {
	// After some time, past traces shouldn't impact the score
	assert := assert.New(t)
	backend := getTestBackend()

	sign := randomSignature()

	// Number of tracesPerPeriod in the initial phase
	tracesPerPeriod := 1000
	ticks := 50

	for period := 0; period < ticks; period++ {
		backend.decayScore()
		for i := 0; i < tracesPerPeriod; i++ {
			backend.CountSignature(sign)
		}
	}

	// Second phase: we stop receiving this signature

	// How long to wait until score is >50% the initial score (TODO: make it function of the config)
	halfLifePeriods := 6
	// How long to wait until score is >1% the initial score
	oblivionPeriods := 40

	for period := 0; period < halfLifePeriods; period++ {
		backend.decayScore()
	}

	assert.True(backend.GetSignatureScore(sign) < 0.5*float64(tracesPerPeriod))

	for period := 0; period < oblivionPeriods-halfLifePeriods; period++ {
		backend.decayScore()
	}

	assert.True(backend.GetSignatureScore(sign) < 0.01*float64(tracesPerPeriod))
}
