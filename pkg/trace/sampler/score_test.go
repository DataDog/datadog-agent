// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package sampler

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func randomTraceID() uint64 {
	return uint64(rand.Int63())
}

func TestTrivialSampleByRate(t *testing.T) {
	assert := assert.New(t)

	assert.False(SampleByRate(randomTraceID(), 0))
	assert.True(SampleByRate(randomTraceID(), 1))
}

func TestSampleRateManyTraces(t *testing.T) {
	// Test that the effective sample rate isn't far from the theoretical
	// Test with multiple sample rates
	assert := assert.New(t)

	times := 1000000

	for _, rate := range []float64{1.0, 0.1, 0.5, 0.99} {
		sampled := 0
		for i := 0; i < times; i++ {
			if SampleByRate(randomTraceID(), rate) {
				sampled++
			}
		}
		assert.InEpsilon(float64(sampled), float64(times)*rate, 0.01)
	}
}

func BenchmarkBackendScoreToSamplerScore(b *testing.B) {
	s := newSampler(1.0, 10)
	for i := 0; i < b.N; i++ {
		s.backendScoreToSamplerScore(10)
	}
}
