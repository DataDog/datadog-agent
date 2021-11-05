// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/atomic"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

const defaultEnv = "none"

func getTestErrorsSampler(tps float64) *ErrorsSampler {
	// Disable debug logs in these tests
	seelog.UseLogger(seelog.Disabled)

	// No extra fixed sampling, no maximum TPS
	conf := &config.AgentConfig{
		ExtraSampleRate: 1,
		ErrorTPS:        tps,
	}
	return NewErrorsSampler(conf)
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

	s := getTestErrorsSampler(10)
	trace, root := getTestTrace()
	signature := testComputeSignature(trace)

	// Feed the s with a signature so that it has a < 1 sample rate
	for i := 0; i < int(1e6); i++ {
		s.Sample(trace, root, defaultEnv)
	}

	sRate := s.GetSampleRate(trace, root, signature)

	// Then turn on the extra sample rate, then ensure it affects both existing and new signatures
	s.extraRate = 0.33

	assert.Equal(s.GetSampleRate(trace, root, signature), s.extraRate*sRate)
}

func TestTargetTPS(t *testing.T) {
	// Test the "effectiveness" of the targetTPS option.
	assert := assert.New(t)
	targetTPS := 5.0
	s := getTestErrorsSampler(targetTPS)

	tps := 100.0
	// To avoid the edge effects from an non-initialized sampler, wait a bit before counting samples.
	initPeriods := 20
	periods := 50

	s.targetTPS = atomic.NewFloat(targetTPS)
	periodSeconds := defaultDecayPeriod.Seconds()
	tracesPerPeriod := tps * periodSeconds
	// Set signature score offset high enough not to kick in during the test.
	s.signatureScoreOffset.Store(2 * tps)
	s.signatureScoreFactor.Store(math.Pow(s.Sampler.signatureScoreSlope.Load(), math.Log10(s.Sampler.signatureScoreOffset.Load())))

	sampledCount := 0

	for period := 0; period < initPeriods+periods; period++ {
		s.Backend.DecayScore()
		for i := 0; i < int(tracesPerPeriod); i++ {
			trace, root := getTestTrace()
			sampled := s.Sample(trace, root, defaultEnv)
			// Once we got into the "supposed-to-be" stable "regime", count the samples
			if period > initPeriods && sampled {
				sampledCount++
			}
		}
	}

	// Check that the sampled score pre-targetTPS is equals to the incoming number of traces per second
	assert.InEpsilon(tps, s.Backend.GetSampledScore(), 0.01)

	// We should have kept less traces per second than targetTPS
	assert.True(s.targetTPS.Load() >= float64(sampledCount)/(float64(periods)*periodSeconds))

	// We should have a throughput of sampled traces around targetTPS
	// Check for 1% epsilon, but the precision also depends on the backend imprecision (error factor = decayFactor).
	// Combine error rates with L1-norm instead of L2-norm by laziness, still good enough for tests.
	assert.InEpsilon(s.targetTPS.Load(), float64(sampledCount)/(float64(periods)*periodSeconds),
		0.01+defaultDecayFactor-1)
}

func TestDisable(t *testing.T) {
	assert := assert.New(t)

	s := getTestErrorsSampler(0)
	trace, root := getTestTrace()
	for i := 0; i < int(1e2); i++ {
		assert.False(s.Sample(trace, root, defaultEnv))
	}
}

func BenchmarkSampler(b *testing.B) {
	// Benchmark the resource consumption of many traces sampling

	// Up to signatureCount different signatures
	signatureCount := 20

	s := getTestErrorsSampler(10)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		trace := pb.Trace{
			&pb.Span{TraceID: 1, SpanID: 1, ParentID: 0, Start: 42, Duration: 1000000000, Service: "mcnulty", Type: "web", Resource: fmt.Sprint(rand.Intn(signatureCount))},
			&pb.Span{TraceID: 1, SpanID: 2, ParentID: 1, Start: 100, Duration: 200000000, Service: "mcnulty", Type: "sql"},
			&pb.Span{TraceID: 1, SpanID: 3, ParentID: 2, Start: 150, Duration: 199999000, Service: "master-db", Type: "sql"},
			&pb.Span{TraceID: 1, SpanID: 4, ParentID: 1, Start: 500000000, Duration: 500000, Service: "redis", Type: "redis"},
			&pb.Span{TraceID: 1, SpanID: 5, ParentID: 1, Start: 700000000, Duration: 700000, Service: "mcnulty", Type: ""},
		}
		s.Sample(trace, trace[0], defaultEnv)
	}
}
