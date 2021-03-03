// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"math"
	"math/rand"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/cihub/seelog"

	"github.com/stretchr/testify/assert"
)

const (
	testServiceA = "service-a"
	testServiceB = "service-b"
)

func getTestPrioritySampler() *PrioritySampler {
	// Disable debug logs in these tests
	seelog.UseLogger(seelog.Disabled)

	// No extra fixed sampling, no maximum TPS
	conf := &config.AgentConfig{
		ExtraSampleRate: 1.0,
		TargetTPS:       0.0,
	}

	return NewPrioritySampler(conf, &DynamicConfig{})
}

func getTestTraceWithService(t *testing.T, service string, s *PrioritySampler) (pb.Trace, *pb.Span) {
	tID := randomTraceID()
	trace := pb.Trace{
		&pb.Span{TraceID: tID, SpanID: 1, ParentID: 0, Start: 42, Duration: 1000000, Service: service, Type: "web", Meta: map[string]string{"env": defaultEnv}, Metrics: map[string]float64{}},
		&pb.Span{TraceID: tID, SpanID: 2, ParentID: 1, Start: 100, Duration: 200000, Service: service, Type: "sql"},
	}
	r := rand.Float64()
	priority := PriorityAutoDrop
	rates := s.ratesByService()
	key := ServiceSignature{trace[0].Service, defaultEnv}
	var rate float64
	if serviceRate, ok := rates[key]; ok {
		rate = serviceRate
		trace[0].Metrics[agentRateKey] = serviceRate
	} else {
		rate = 1
	}
	if r <= rate {
		priority = PriorityAutoKeep
	}
	SetSamplingPriority(trace[0], priority)
	return trace, trace[0]
}

func TestPrioritySample(t *testing.T) {
	// Simple sample unit test
	assert := assert.New(t)

	env := defaultEnv

	s := getTestPrioritySampler()

	assert.Equal(0.0, s.Sampler.Backend.GetTotalScore(), "checking fresh backend total score is 0")
	assert.Equal(0.0, s.Sampler.Backend.GetSampledScore(), "checkeing fresh backend sampled score is 0")

	s = getTestPrioritySampler()
	trace, root := getTestTraceWithService(t, "my-service", s)

	SetSamplingPriority(root, -1)
	sampled := s.Sample(trace, root, env, false)
	assert.False(sampled, "trace with negative priority is dropped")
	assert.Equal(0.0, s.Sampler.Backend.GetTotalScore(), "sampling a priority -1 trace should *NOT* impact sampler backend")
	assert.Equal(0.0, s.Sampler.Backend.GetSampledScore(), "sampling a priority -1 trace should *NOT* impact sampler backend")

	s = getTestPrioritySampler()
	trace, root = getTestTraceWithService(t, "my-service", s)

	SetSamplingPriority(root, 0)
	sampled = s.Sample(trace, root, env, false)
	assert.False(sampled, "trace with priority 0 is dropped")
	assert.True(0.0 < s.Sampler.Backend.GetTotalScore(), "sampling a priority 0 trace should increase total score")
	assert.Equal(0.0, s.Sampler.Backend.GetSampledScore(), "sampling a priority 0 trace should *NOT* increase sampled score")

	s = getTestPrioritySampler()
	trace, root = getTestTraceWithService(t, "my-service", s)

	SetSamplingPriority(root, 1)
	sampled = s.Sample(trace, root, env, false)
	assert.True(sampled, "trace with priority 1 is kept")
	assert.True(0.0 < s.Sampler.Backend.GetTotalScore(), "sampling a priority 0 trace should increase total score")
	assert.True(0.0 < s.Sampler.Backend.GetSampledScore(), "sampling a priority 0 trace should increase sampled score")

	s = getTestPrioritySampler()
	trace, root = getTestTraceWithService(t, "my-service", s)

	SetSamplingPriority(root, 2)
	sampled = s.Sample(trace, root, env, false)
	assert.True(sampled, "trace with priority 2 is kept")
	assert.Equal(0.0, s.Sampler.Backend.GetTotalScore(), "sampling a priority 2 trace should *NOT* increase total score")
	assert.Equal(0.0, s.Sampler.Backend.GetSampledScore(), "sampling a priority 2 trace should *NOT* increase sampled score")

	s = getTestPrioritySampler()
	trace, root = getTestTraceWithService(t, "my-service", s)

	SetSamplingPriority(root, PriorityUserKeep)
	sampled = s.Sample(trace, root, env, false)
	assert.True(sampled, "trace with high priority is kept")
	assert.Equal(0.0, s.Sampler.Backend.GetTotalScore(), "sampling a high priority trace should *NOT* increase total score")
	assert.Equal(0.0, s.Sampler.Backend.GetSampledScore(), "sampling a high priority trace should *NOT* increase sampled score")

	delete(root.Metrics, KeySamplingPriority)
	sampled = s.Sample(trace, root, env, false)
	assert.False(sampled, "this should not happen but a trace without priority sampling set should be dropped")
}

func TestPrioritySampleThresholdTo1(t *testing.T) {
	assert := assert.New(t)
	env := defaultEnv

	s := getTestPrioritySampler()
	for i := 0; i < 1e2; i++ {
		trace, root := getTestTraceWithService(t, "my-service", s)
		SetSamplingPriority(root, SamplingPriority(i%2))
		sampled := s.Sample(trace, root, env, false)
		if sampled {
			rate, _ := root.Metrics[agentRateKey]
			assert.Equal(1.0, rate)
		}
	}
	for i := 0; i < 1e3; i++ {
		trace, root := getTestTraceWithService(t, "my-service", s)
		SetSamplingPriority(root, SamplingPriority(i%2))
		sampled := s.Sample(trace, root, env, false)
		if sampled {
			rate, _ := root.Metrics[agentRateKey]
			if rate < 1 {
				assert.True(rate < prioritySamplingRateThresholdTo1)
			}
		}
	}
}

func TestTargetTPSByService(t *testing.T) {
	rand.Seed(1)
	// Test the "effectiveness" of the targetTPS option.
	assert := assert.New(t)
	s := getTestPrioritySampler()

	type testCase struct {
		targetTPS     float64
		tps           float64
		relativeError float64
	}
	testCases := []testCase{
		{targetTPS: 10.0, tps: 20.0, relativeError: 0.2},
	}
	if !testing.Short() {
		testCases = append(testCases,
			testCase{targetTPS: 5.0, tps: 50.0, relativeError: 0.2},
			testCase{targetTPS: 3.0, tps: 200.0, relativeError: 0.2},
			testCase{targetTPS: 1.0, tps: 1000.0, relativeError: 0.2},
			testCase{targetTPS: 10.0, tps: 10.0, relativeError: 0.001},
			testCase{targetTPS: 10.0, tps: 3.0, relativeError: 0.001})
	}

	// To avoid the edge effects from an non-initialized sampler, wait a bit before counting samples.
	const (
		initPeriods = 50
		periods     = 500
	)

	s.Sampler.rateThresholdTo1 = 1
	for _, tc := range testCases {
		t.Logf("testing targetTPS=%0.1f tps=%0.1f", tc.targetTPS, tc.tps)
		s.Sampler.targetTPS = tc.targetTPS
		periodSeconds := defaultDecayPeriod.Seconds()
		tracesPerPeriod := tc.tps * periodSeconds
		// Set signature score offset high enough not to kick in during the test.
		s.Sampler.signatureScoreOffset.Store(2 * tc.tps)
		s.Sampler.signatureScoreFactor.Store(math.Pow(s.Sampler.signatureScoreSlope.Load(), math.Log10(s.Sampler.signatureScoreOffset.Load())))

		sampledCount := 0
		handledCount := 0

		for period := 0; period < initPeriods+periods; period++ {
			s.Sampler.Backend.DecayScore()
			s.Sampler.AdjustScoring()
			for i := 0; i < int(tracesPerPeriod); i++ {
				trace, root := getTestTraceWithService(t, "service-a", s)
				sampled := s.Sample(trace, root, defaultEnv, false)
				// Once we got into the "supposed-to-be" stable "regime", count the samples
				if period > initPeriods {
					handledCount++
					if sampled {
						sampledCount++
					}
				}
			}
		}

		// When tps is lower than targetTPS it means that we are actually not sampling
		// anything, so the target is the original tps, and not targetTPS.
		// Also, in that case, results should be more precise.
		targetTPS := tc.targetTPS
		relativeError := 0.01
		if tc.targetTPS > tc.tps {
			targetTPS = tc.tps
		} else {
			relativeError = 0.1 + defaultDecayFactor - 1
		}

		// Check that the sampled score is roughly equal to targetTPS. This is different from
		// the score sampler test as here we run adjustscoring on a regular basis so the converges to targetTPS.
		assert.InEpsilon(targetTPS, s.Sampler.Backend.GetSampledScore(), relativeError)

		// We should have keep the right percentage of traces
		assert.InEpsilon(targetTPS/tc.tps, float64(sampledCount)/float64(handledCount), relativeError)

		// We should have a throughput of sampled traces around targetTPS
		// Check for 1% epsilon, but the precision also depends on the backend imprecision (error factor = decayFactor).
		// Combine error rates with L1-norm instead of L2-norm by laziness, still good enough for tests.
		assert.InEpsilon(targetTPS, float64(sampledCount)/(float64(periods)*periodSeconds), relativeError)
	}
}

func TestTPSClientDrop(t *testing.T) {
	rand.Seed(1)
	// Test the "effectiveness" of the targetTPS option.
	assert := assert.New(t)
	s := getTestPrioritySampler()

	type testCase struct {
		targetTPS     float64
		tps           float64
		relativeError float64
	}
	testCases := []testCase{
		{targetTPS: 10.0, tps: 20.0, relativeError: 0.2},
	}
	if !testing.Short() {
		testCases = append(testCases,
			testCase{targetTPS: 5.0, tps: 50.0, relativeError: 0.2},
			testCase{targetTPS: 3.0, tps: 200.0, relativeError: 0.2},
			testCase{targetTPS: 1.0, tps: 1000.0, relativeError: 0.2},
			testCase{targetTPS: 10.0, tps: 3.0, relativeError: 0.001})
	}
	// To avoid the edge effects from an non-initialized sampler, wait a bit before counting samples.
	const (
		initPeriods = 50
		periods     = 500
	)

	s.Sampler.rateThresholdTo1 = 1
	for _, tc := range testCases {
		t.Logf("testing targetTPS=%0.1f tps=%0.1f", tc.targetTPS, tc.tps)
		s.Sampler.targetTPS = tc.targetTPS
		periodSeconds := defaultDecayPeriod.Seconds()
		tracesPerPeriod := tc.tps * periodSeconds
		// Set signature score offset high enough not to kick in during the test.
		s.Sampler.signatureScoreOffset.Store(2 * tc.tps)
		s.Sampler.signatureScoreFactor.Store(math.Pow(s.Sampler.signatureScoreSlope.Load(), math.Log10(s.Sampler.signatureScoreOffset.Load())))

		sampledCount := 0
		handledCount := 0

		for period := 0; period < initPeriods+periods; period++ {
			s.Sampler.Backend.DecayScore()
			s.Sampler.AdjustScoring()
			for i := 0; i < int(tracesPerPeriod); i++ {
				trace, root := getTestTraceWithService(t, "service-a", s)
				var sampled bool
				if prio, _ := GetSamplingPriority(root); prio == 1 {
					sampled = s.Sample(trace, root, defaultEnv, true)
				} else {
					s.CountClientDroppedP0s(1)
				}
				// Once we got into the "supposed-to-be" stable "regime", count the samples
				if period > initPeriods {
					handledCount++
					if sampled {
						sampledCount++
					}
				}
			}
		}

		// When tps is lower than targetTPS it means that we are actually not sampling
		// anything, so the target is the original tps, and not targetTPS.
		// Also, in that case, results should be more precise.
		targetTPS := tc.targetTPS
		relativeError := 0.01
		if tc.targetTPS > tc.tps {
			targetTPS = tc.tps
		} else {
			relativeError = 0.1 + defaultDecayFactor - 1
		}

		// Check that the sampled score is roughly equal to targetTPS. This is different from
		// the score sampler test as here we run adjustscoring on a regular basis so the converges to targetTPS.
		assert.InEpsilon(targetTPS, s.Sampler.Backend.GetSampledScore(), relativeError)

		// We should have keep the right percentage of traces
		assert.InEpsilon(targetTPS/tc.tps, float64(sampledCount)/float64(handledCount), relativeError)

		// We should have a throughput of sampled traces around targetTPS
		// Check for 1% epsilon, but the precision also depends on the backend imprecision (error factor = decayFactor).
		// Combine error rates with L1-norm instead of L2-norm by laziness, still good enough for tests.
		assert.InEpsilon(targetTPS, float64(sampledCount)/(float64(periods)*periodSeconds), relativeError)
	}
}
