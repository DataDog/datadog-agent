// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"math/rand"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

func randomTraceID() uint64 {
	return uint64(rand.Int63())
}

func getTestPrioritySampler() *PrioritySampler {
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
	priority := PriorityAutoDrop
	r := rand.Float64()
	rates := s.rateByService.rates
	key := ServiceSignature{spans[0].Service, defaultEnv}

	serviceRate, ok := rates[key.String()]
	if !ok {
		serviceRate, _ = rates[ServiceSignature{}.String()]
	}
	rate := float64(1)
	if serviceRate != nil {
		rate = serviceRate.r
	}
	if r <= rate {
		priority = PriorityAutoKeep
	}
	spans[0].Metrics[agentRateKey] = rate
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

	assert.Equal(float32(0), s.localRates.totalSeen, "checking fresh backend total score is 0")
	assert.Equal(int64(0), s.localRates.totalKept.Load(), "checkeing fresh backend sampled score is 0")

	s = getTestPrioritySampler()
	chunk, root := getTestTraceWithService(t, "my-service", s)

	chunk.Priority = -1
	sampled := s.Sample(time.Now(), chunk, root, env, 0)
	assert.False(sampled, "trace with negative priority is dropped")
	assert.Equal(float32(0), s.localRates.totalSeen, "sampling a priority -1 trace should *NOT* impact sampler backend")
	assert.Equal(int64(0), s.localRates.totalKept.Load(), "sampling a priority -1 trace should *NOT* impact sampler backend")

	s = getTestPrioritySampler()
	chunk, root = getTestTraceWithService(t, "my-service", s)

	chunk.Priority = 0
	sampled = s.Sample(time.Now(), chunk, root, env, 0)
	assert.False(sampled, "trace with priority 0 is dropped")
	assert.True(float32(0) < s.localRates.totalSeen, "sampling a priority 0 trace should increase total score")
	assert.Equal(int64(0), s.localRates.totalKept.Load(), "sampling a priority 0 trace should *NOT* increase sampled score")

	s = getTestPrioritySampler()
	chunk, root = getTestTraceWithService(t, "my-service", s)

	chunk.Priority = 1
	sampled = s.Sample(time.Now(), chunk, root, env, 0)
	assert.True(sampled, "trace with priority 1 is kept")
	assert.True(float32(0) < s.localRates.totalSeen, "sampling a priority 0 trace should increase total score")
	assert.True(int64(0) < s.localRates.totalKept.Load(), "sampling a priority 0 trace should increase sampled score")

	s = getTestPrioritySampler()
	chunk, root = getTestTraceWithService(t, "my-service", s)

	chunk.Priority = 2
	sampled = s.Sample(time.Now(), chunk, root, env, 0)
	assert.True(sampled, "trace with priority 2 is kept")
	assert.Equal(float32(0), s.localRates.totalSeen, "sampling a priority 2 trace should *NOT* increase total score")
	assert.Equal(int64(0), s.localRates.totalKept.Load(), "sampling a priority 2 trace should *NOT* increase sampled score")

	s = getTestPrioritySampler()
	chunk, root = getTestTraceWithService(t, "my-service", s)

	chunk.Priority = int32(PriorityUserKeep)
	sampled = s.Sample(time.Now(), chunk, root, env, 0)
	assert.True(sampled, "trace with high priority is kept")
	assert.Equal(float32(0), s.localRates.totalSeen, "sampling a high priority trace should *NOT* increase total score")
	assert.Equal(int64(0), s.localRates.totalKept.Load(), "sampling a high priority trace should *NOT* increase sampled score")

	chunk.Priority = int32(PriorityNone)
	sampled = s.Sample(time.Now(), chunk, root, env, 0)
	assert.False(sampled, "this should not happen but a trace without priority sampling set should be dropped")
}

func TestPrioritySamplerWithNilRemote(t *testing.T) {
	conf := &config.AgentConfig{
		ExtraSampleRate: 1.0,
		TargetTPS:       1.0,
	}
	s := NewPrioritySampler(conf, NewDynamicConfig())
	s.Start()
	s.updateRates()
	s.reportStats()
	chunk, root := getTestTraceWithService(t, "my-service", s)
	assert.True(t, s.Sample(time.Now(), chunk, root, "", 0))
	s.Stop()
}

func TestPrioritySamplerWithRemote(t *testing.T) {
	conf := &config.AgentConfig{
		ExtraSampleRate: 1.0,
		TargetTPS:       1.0,
	}
	s := NewPrioritySampler(conf, NewDynamicConfig())
	s.remoteRates = newRemoteRates(nil, 10, "6.0.0")
	s.Start()
	s.updateRates()
	s.reportStats()
	chunk, root := getTestTraceWithService(t, "my-service", s)
	assert.True(t, s.Sample(time.Now(), chunk, root, "", 0))
	s.Stop()
}

func TestPrioritySamplerTPSFeedbackLoop(t *testing.T) {
	assert := assert.New(t)

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
		{targetTPS: 5.0, generatedTPS: 50.0, expectedTPS: 5.0, relativeError: 0.05, service: "bim"},
	}
	if !testing.Short() {
		testCases = append(testCases,
			testCase{targetTPS: 3.0, generatedTPS: 200.0, expectedTPS: 3.0, relativeError: 0.05, service: "2"},
			testCase{targetTPS: 10.0, generatedTPS: 10.0, expectedTPS: 10.0, relativeError: 0.03, service: "4"},
			testCase{targetTPS: 10.0, generatedTPS: 3.0, expectedTPS: 3.0, relativeError: 0.03, service: "10"},
			testCase{targetTPS: 0.5, generatedTPS: 100.0, expectedTPS: 0.5, relativeError: 0.1, service: "0.5"},
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
	testCasesRates := apmsampling.APMSampling{TargetTPS: make([]apmsampling.TargetTPS, 0, len(testCases))}
	for _, tc := range testCases {

		if tc.localRate {
			continue
		}
		testCasesRates.TargetTPS = append(testCasesRates.TargetTPS, apmsampling.TargetTPS{Service: tc.service, Value: tc.targetTPS, Env: defaultEnv})
	}
	for _, tc := range testCases {
		rand.Seed(3)
		s := getTestPrioritySampler()
		s.remoteRates = newTestRemoteRates()
		generatedConfigVersion := uint64(120)
		s.remoteRates.onUpdate(configGenerator(generatedConfigVersion, testCasesRates))

		t.Logf("testing targetTPS=%0.1f generatedTPS=%0.1f localRate=%v clientDrop=%v", tc.targetTPS, tc.generatedTPS, tc.localRate, tc.clientDrop)
		s.localRates.targetTPS = atomic.NewFloat64(tc.targetTPS)

		var sampledCount, handledCount int
		const warmUpDuration, testDuration = 2, 10
		testTime := time.Now()
		for timeElapsed := 0; timeElapsed < warmUpDuration+testDuration; timeElapsed++ {
			tracesPerPeriod := tc.generatedTPS * bucketDuration.Seconds()
			testTime = testTime.Add(bucketDuration)
			var clientDrops int
			for i := 0; i < int(tracesPerPeriod); i++ {
				chunk, root := getTestTraceWithService(t, tc.service, s)

				var sampled bool
				if !tc.clientDrop {
					sampled = s.Sample(testTime, chunk, root, defaultEnv, 0)
				} else {
					if prio, _ := GetSamplingPriority(chunk); prio == 1 {
						sampled = s.Sample(testTime, chunk, root, defaultEnv, float64(clientDrops))
						clientDrops = 0
					} else {
						clientDrops++
					}
				}

				tpsTag, okTPS := root.Metrics[tagRemoteTPS]
				versionTag, okVersion := root.Metrics[tagRemoteVersion]
				if !tc.localRate && timeElapsed > 1 {
					localRates, _ := s.localRates.getAllSignatureSampleRates()
					remoteRates := s.remoteRates.getAllSignatureSampleRates()
					require.Equal(t, len(localRates), len(remoteRates))

					for sig, rate := range localRates {
						remoteRate, ok := remoteRates[sig]
						assert.True(ok)
						require.Equal(t, rate, remoteRate.r)
					}
				}

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

				// outside of warmUp the rate should match
				// skipping clientDrop as the feedback loop is different when client drops
				// seen is actually the last rate sent
				if !tc.clientDrop {
					appliedRate := root.Metrics[agentRateKey]
					assert.InEpsilon(tc.expectedTPS/tc.generatedTPS, appliedRate, 0.0000001)
				}

				// We track rates stats only when the warm up phase is over
				handledCount++
				if sampled {
					sampledCount++
				}
			}
		}

		// We should keep the right percentage of traces
		assert.InEpsilon(tc.expectedTPS/tc.generatedTPS, float64(sampledCount)/float64(handledCount), tc.relativeError)

		// We should have a throughput of sampled traces around targetTPS
		// Check for 1% epsilon, but the precision also depends on the backend imprecision (error factor = decayFactor).
		// Combine error rates with L1-norm instead of L2-norm by laziness, still good enough for tests.
		assert.InEpsilon(tc.expectedTPS, float64(sampledCount)/(float64(testDuration)*bucketDuration.Seconds()), tc.relativeError)
	}
}
