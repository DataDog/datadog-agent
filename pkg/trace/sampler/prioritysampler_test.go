// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"math/rand"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/atomic"
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

func getTestTraceWithService(t *testing.T, service string, s *PrioritySampler) (*pb.TraceChunk, *pb.Span) {
	tID := randomTraceID()
	spans := []*pb.Span{
		{TraceID: tID, SpanID: 1, ParentID: 0, Start: 42, Duration: 1000000, Service: service, Type: "web", Meta: map[string]string{"env": defaultEnv}, Metrics: map[string]float64{}},
		{TraceID: tID, SpanID: 2, ParentID: 1, Start: 100, Duration: 200000, Service: service, Type: "sql"},
	}
	r := rand.Float64()
	priority := PriorityAutoDrop
	rates := s.ratesByService()
	key := ServiceSignature{spans[0].Service, defaultEnv}
	var rate float64
	if serviceRate, ok := rates[key]; ok {
		rate = serviceRate
		spans[0].Metrics[agentRateKey] = serviceRate
	} else {
		rate = 1
	}
	if r <= rate {
		priority = PriorityAutoKeep
	}
	return &pb.TraceChunk{
		Priority: int32(priority),
		Spans:    spans,
	}, spans[0]
}

func TestPrioritySample(t *testing.T) {
	// Simple sample unit test
	assert := assert.New(t)

	env := defaultEnv

	s := getTestPrioritySampler()

	assert.Equal(0.0, s.localRates.Backend.GetTotalScore(), "checking fresh backend total score is 0")
	assert.Equal(0.0, s.localRates.Backend.GetSampledScore(), "checkeing fresh backend sampled score is 0")

	s = getTestPrioritySampler()
	chunk, root := getTestTraceWithService(t, "my-service", s)

	chunk.Priority = -1
	sampled := s.Sample(chunk, root, env, false)
	assert.False(sampled, "trace with negative priority is dropped")
	assert.Equal(0.0, s.localRates.Backend.GetTotalScore(), "sampling a priority -1 trace should *NOT* impact sampler backend")
	assert.Equal(0.0, s.localRates.Backend.GetSampledScore(), "sampling a priority -1 trace should *NOT* impact sampler backend")

	s = getTestPrioritySampler()
	chunk, root = getTestTraceWithService(t, "my-service", s)

	chunk.Priority = 0
	sampled = s.Sample(chunk, root, env, false)
	assert.False(sampled, "trace with priority 0 is dropped")
	assert.True(0.0 < s.localRates.Backend.GetTotalScore(), "sampling a priority 0 trace should increase total score")
	assert.Equal(0.0, s.localRates.Backend.GetSampledScore(), "sampling a priority 0 trace should *NOT* increase sampled score")

	s = getTestPrioritySampler()
	chunk, root = getTestTraceWithService(t, "my-service", s)

	chunk.Priority = 1
	sampled = s.Sample(chunk, root, env, false)
	assert.True(sampled, "trace with priority 1 is kept")
	assert.True(0.0 < s.localRates.Backend.GetTotalScore(), "sampling a priority 0 trace should increase total score")
	assert.True(0.0 < s.localRates.Backend.GetSampledScore(), "sampling a priority 0 trace should increase sampled score")

	s = getTestPrioritySampler()
	chunk, root = getTestTraceWithService(t, "my-service", s)

	chunk.Priority = 2
	sampled = s.Sample(chunk, root, env, false)
	assert.True(sampled, "trace with priority 2 is kept")
	assert.Equal(0.0, s.localRates.Backend.GetTotalScore(), "sampling a priority 2 trace should *NOT* increase total score")
	assert.Equal(0.0, s.localRates.Backend.GetSampledScore(), "sampling a priority 2 trace should *NOT* increase sampled score")

	s = getTestPrioritySampler()
	chunk, root = getTestTraceWithService(t, "my-service", s)

	chunk.Priority = int32(PriorityUserKeep)
	sampled = s.Sample(chunk, root, env, false)
	assert.True(sampled, "trace with high priority is kept")
	assert.Equal(0.0, s.localRates.Backend.GetTotalScore(), "sampling a high priority trace should *NOT* increase total score")
	assert.Equal(0.0, s.localRates.Backend.GetSampledScore(), "sampling a high priority trace should *NOT* increase sampled score")

	chunk.Priority = int32(PriorityNone)
	sampled = s.Sample(chunk, root, env, false)
	assert.False(sampled, "this should not happen but a trace without priority sampling set should be dropped")
}

func TestPrioritySampleThresholdTo1(t *testing.T) {
	assert := assert.New(t)
	env := defaultEnv

	s := getTestPrioritySampler()
	for i := 0; i < 1e2; i++ {
		chunk, root := getTestTraceWithService(t, "my-service", s)
		chunk.Priority = int32(i % 2)
		sampled := s.Sample(chunk, root, env, false)
		if sampled {
			rate, _ := root.Metrics[agentRateKey]
			assert.Equal(1.0, rate)
		}
	}
	for i := 0; i < 1e3; i++ {
		chunk, root := getTestTraceWithService(t, "my-service", s)
		chunk.Priority = int32(i % 2)
		sampled := s.Sample(chunk, root, env, false)
		if sampled {
			rate, _ := root.Metrics[agentRateKey]
			if rate < 1 {
				assert.True(rate < priorityLocalRateThresholdTo1)
			}
		}
	}
}

func TestPrioritySamplerTPSFeedbackLoop(t *testing.T) {
	assert := assert.New(t)
	rand.Seed(1)

	s := getTestPrioritySampler()

	type testCase struct {
		targetTPS     float64
		generatedTPS  float64
		service       string
		localRate     bool
		clientDrop    bool
		relativeError float64
		expectedTPS   float64
	}
	testCases := []testCase{
		{targetTPS: 5.0, generatedTPS: 50.0, expectedTPS: 5.0, relativeError: 0.2, service: "bim"},
	}
	if !testing.Short() {
		testCases = append(testCases,
			testCase{targetTPS: 3.0, generatedTPS: 200.0, expectedTPS: 3.0, relativeError: 0.2, service: "2"},
			testCase{targetTPS: 10.0, generatedTPS: 10.0, expectedTPS: 10.0, relativeError: 0.0051, service: "4"},
			testCase{targetTPS: 10.0, generatedTPS: 3.0, expectedTPS: 3.0, relativeError: 0.0051, service: "10"},
			testCase{targetTPS: 0.5, generatedTPS: 100.0, expectedTPS: 0.5, relativeError: 0.5, service: "0.5"},
		)
	}
	// Duplicate each testcases and use local rates instead of remote rates
	for i := len(testCases) - 1; i >= 0; i-- {
		tc := testCases[i]
		tc.localRate = true
		tc.service = "local" + tc.service
		testCases = append(testCases, tc)
	}

	// Duplicate each testcases and consider that agent client drops unsampled spans
	for i := len(testCases) - 1; i >= 0; i-- {
		tc := testCases[i]
		tc.clientDrop = true
		testCases = append(testCases, tc)
	}

	// setting up remote store
	testCasesRates := pb.APMSampling{TargetTps: make([]pb.TargetTPS, 0, len(testCases))}
	for _, tc := range testCases {
		if tc.localRate {
			continue
		}
		testCasesRates.TargetTps = append(testCasesRates.TargetTps, pb.TargetTPS{Service: tc.service, Value: tc.targetTPS, Env: defaultEnv})
	}
	s.remoteRates = newTestRemoteRates()
	generatedConfigVersion := uint64(120)
	s.remoteRates.loadNewConfig(configGenerator(generatedConfigVersion, testCasesRates))

	for _, tc := range testCases {
		t.Logf("testing targetTPS=%0.1f generatedTPS=%0.1f localRate=%v clientDrop=%v", tc.targetTPS, tc.generatedTPS, tc.localRate, tc.clientDrop)
		if tc.localRate {
			s.localRates.targetTPS = atomic.NewFloat(tc.targetTPS)
		}

		var sampledCount, handledCount int
		// We wait for the warmUpDuration to reach a stable state
		// After the warm up is completed, we track rates during testDuration.
		// The time unit is a decayPeriod duration.
		const warmUpDuration, testDuration = 10, 300
		for timeElapsed := 0; timeElapsed < warmUpDuration+testDuration; timeElapsed++ {
			s.localRates.Backend.DecayScore()
			s.localRates.AdjustScoring()
			s.remoteRates.DecayScores()
			s.remoteRates.AdjustScoring()

			tracesPerDecay := tc.generatedTPS * defaultDecayPeriod.Seconds()
			for i := 0; i < int(tracesPerDecay); i++ {
				chunk, root := getTestTraceWithService(t, tc.service, s)

				var sampled bool
				if !tc.clientDrop {
					sampled = s.Sample(chunk, root, defaultEnv, false)
				} else {
					if prio, _ := GetSamplingPriority(chunk); prio == 1 {
						sampled = s.Sample(chunk, root, defaultEnv, true)

					} else {
						s.CountClientDroppedP0s(1)
					}
				}

				tpsTag, okTPS := root.Metrics[tagRemoteTPS]
				versionTag, okVersion := root.Metrics[tagRemoteVersion]
				if !tc.localRate && sampled {
					assert.True(okTPS)
					assert.Equal(tc.targetTPS, tpsTag)
					assert.True(okVersion)
					assert.Equal(float64(generatedConfigVersion), versionTag)
				} else {
					assert.False(okTPS)
					assert.False(okVersion)
				}

				if timeElapsed < warmUpDuration {
					continue
				}

				// We track rates stats only when the warm up phase is over
				handledCount++
				if sampled {
					sampledCount++
				}
			}
		}

		var backendSampler *Sampler
		var ok bool
		if tc.localRate {
			backendSampler = s.localRates
		} else {
			backendSampler, ok = s.remoteRates.getSampler(ServiceSignature{Name: tc.service, Env: defaultEnv}.Hash())
			assert.True(ok)
		}

		assert.InEpsilon(tc.expectedTPS, backendSampler.Backend.GetSampledScore(), tc.relativeError)

		// We should keep the right percentage of traces
		assert.InEpsilon(tc.expectedTPS/tc.generatedTPS, float64(sampledCount)/float64(handledCount), tc.relativeError)

		// We should have a throughput of sampled traces around targetTPS
		// Check for 1% epsilon, but the precision also depends on the backend imprecision (error factor = decayFactor).
		// Combine error rates with L1-norm instead of L2-norm by laziness, still good enough for tests.
		assert.InEpsilon(tc.expectedTPS, float64(sampledCount)/(float64(testDuration)*defaultDecayPeriod.Seconds()), tc.relativeError)
	}
}
