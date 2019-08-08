// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package sampler

import (
	"math"
	"math/rand"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

const defaultEnv = "none"

func getTestScoreEngine() *ScoreEngine {
	// Disable debug logs in these tests
	seelog.UseLogger(seelog.Disabled)

	// No extra fixed sampling, no maximum TPS
	extraRate := 1.0
	maxTPS := 0.0

	return NewScoreEngine(extraRate, maxTPS)
}

func getTestTrace() (pb.Trace, *pb.Span) {
	tID := randomTraceID()
	trace := pb.Trace{
		&pb.Span{TraceID: tID, SpanID: 1, ParentID: 0, Start: 42, Duration: 1000000, Service: "mcnulty", Type: "web"},
		&pb.Span{TraceID: tID, SpanID: 2, ParentID: 1, Start: 100, Duration: 200000, Service: "mcnulty", Type: "sql"},
	}
	return trace, trace[0]
}

func TestExtraSampleRate(t *testing.T) {
	assert := assert.New(t)

	s := getTestScoreEngine()
	trace, root := getTestTrace()
	signature := testComputeSignature(trace)

	// Feed the s with a signature so that it has a < 1 sample rate
	for i := 0; i < int(1e6); i++ {
		s.Sample(trace, root, defaultEnv)
	}

	sRate := s.Sampler.GetSampleRate(trace, root, signature)

	// Then turn on the extra sample rate, then ensure it affects both existing and new signatures
	s.Sampler.extraRate = 0.33

	assert.Equal(s.Sampler.GetSampleRate(trace, root, signature), s.Sampler.extraRate*sRate)
}

func TestMaxTPS(t *testing.T) {
	// Test the "effectiveness" of the maxTPS option.
	assert := assert.New(t)
	s := getTestScoreEngine()

	maxTPS := 5.0
	tps := 100.0
	// To avoid the edge effects from an non-initialized sampler, wait a bit before counting samples.
	initPeriods := 20
	periods := 50

	s.Sampler.maxTPS = maxTPS
	periodSeconds := defaultDecayPeriod.Seconds()
	tracesPerPeriod := tps * periodSeconds
	// Set signature score offset high enough not to kick in during the test.
	s.Sampler.signatureScoreOffset.Store(2 * tps)
	s.Sampler.signatureScoreFactor.Store(math.Pow(s.Sampler.signatureScoreSlope.Load(), math.Log10(s.Sampler.signatureScoreOffset.Load())))

	sampledCount := 0

	for period := 0; period < initPeriods+periods; period++ {
		s.Sampler.Backend.(*MemoryBackend).decayScore()
		for i := 0; i < int(tracesPerPeriod); i++ {
			trace, root := getTestTrace()
			sampled, _ := s.Sample(trace, root, defaultEnv)
			// Once we got into the "supposed-to-be" stable "regime", count the samples
			if period > initPeriods && sampled {
				sampledCount++
			}
		}
	}

	// Check that the sampled score pre-maxTPS is equals to the incoming number of traces per second
	assert.InEpsilon(tps, s.Sampler.Backend.GetSampledScore(), 0.01)

	// We should have kept less traces per second than maxTPS
	assert.True(s.Sampler.maxTPS >= float64(sampledCount)/(float64(periods)*periodSeconds))

	// We should have a throughput of sampled traces around maxTPS
	// Check for 1% epsilon, but the precision also depends on the backend imprecision (error factor = decayFactor).
	// Combine error rates with L1-norm instead of L2-norm by laziness, still good enough for tests.
	assert.InEpsilon(s.Sampler.maxTPS, float64(sampledCount)/(float64(periods)*periodSeconds),
		0.01+defaultDecayFactor-1)
}

func BenchmarkSampler(b *testing.B) {
	// Benchmark the resource consumption of many traces sampling

	// Up to signatureCount different signatures
	signatureCount := 20

	s := getTestScoreEngine()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		trace := pb.Trace{
			&pb.Span{TraceID: 1, SpanID: 1, ParentID: 0, Start: 42, Duration: 1000000000, Service: "mcnulty", Type: "web", Resource: string(rand.Intn(signatureCount))},
			&pb.Span{TraceID: 1, SpanID: 2, ParentID: 1, Start: 100, Duration: 200000000, Service: "mcnulty", Type: "sql"},
			&pb.Span{TraceID: 1, SpanID: 3, ParentID: 2, Start: 150, Duration: 199999000, Service: "master-db", Type: "sql"},
			&pb.Span{TraceID: 1, SpanID: 4, ParentID: 1, Start: 500000000, Duration: 500000, Service: "redis", Type: "redis"},
			&pb.Span{TraceID: 1, SpanID: 5, ParentID: 1, Start: 700000000, Duration: 700000, Service: "mcnulty", Type: ""},
		}
		s.Sample(trace, trace[0], defaultEnv)
	}
}

// Ensure ScoreEngine implements engine.
var testScoreEngine Engine = &ScoreEngine{}
