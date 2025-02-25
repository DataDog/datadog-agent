// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

const defaultEnv = "testEnv"

func getTestErrorsSampler(tps float64) *ErrorsSampler {
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
	signature := testComputeSignature(trace, "")

	testTime := time.Now()
	// Feed the s with a signature so that it has a < 1 sample rate
	for i := 0; i < int(1e6); i++ {
		s.Sample(testTime, trace, root, defaultEnv)
	}
	// trigger rates update
	s.Sample(testTime.Add(10*time.Second), trace, root, defaultEnv)

	sRate := s.getSignatureSampleRate(signature)

	// Then turn on the extra sample rate, then ensure it affects both existing and new signatures
	s.extraRate = 0.33

	assert.Equal(s.getSignatureSampleRate(signature), s.extraRate*sRate)
}

func TestShrink(t *testing.T) {
	assert := assert.New(t)

	s := getTestErrorsSampler(10)
	testTime := time.Now()

	sigs := []Signature{}
	for i := 1; i < 3*shrinkCardinality; i++ {
		trace, root := getTestTrace()
		sigs = append(sigs, computeSignatureWithRootAndEnv(trace, root, ""))

		trace[1].Service = strconv.FormatInt(int64(i+1000), 10)
		s.Sample(testTime, trace, root, defaultEnv)
	}

	// verify that shrink did not apply to first signatures
	for i := 1; i < shrinkCardinality; i++ {
		assert.Equal(sigs[i], s.shrink(sigs[i]))
	}

	// shrunk
	for i := 2 * shrinkCardinality; i < 3*shrinkCardinality-1; i++ {
		assert.Equal(sigs[i]%shrinkCardinality, s.shrink(sigs[i]))
	}

	assert.Equal(int64(shrinkCardinality), s.size())
}

func TestDisable(t *testing.T) {
	assert := assert.New(t)

	s := getTestErrorsSampler(0)
	trace, root := getTestTrace()
	for i := 0; i < int(1e2); i++ {
		assert.False(s.Sample(time.Now(), trace, root, defaultEnv))
	}
}

func TestTargetTPS(t *testing.T) {
	// Test the "effectiveness" of the targetTPS option.
	assert := assert.New(t)
	targetTPS := 10.0
	s := getTestErrorsSampler(targetTPS)

	generatedTPS := 200.0
	// To avoid the edge effects from an non-initialized sampler, wait a bit before counting samples.
	initPeriods := 2
	periods := 10

	s.targetTPS = atomic.NewFloat64(targetTPS)
	periodSeconds := bucketDuration.Seconds()
	tracesPerPeriod := generatedTPS * periodSeconds

	sampledCount := 0

	testTime := time.Now()
	for period := 0; period < initPeriods+periods; period++ {
		testTime = testTime.Add(bucketDuration)
		for i := 0; i < int(tracesPerPeriod); i++ {
			trace, root := getTestTrace()
			sampled := s.Sample(testTime, trace, root, defaultEnv)
			// Once we got into the "supposed-to-be" stable "regime", count the samples
			if period > initPeriods && sampled {
				sampledCount++
			}
		}
	}

	// We should keep the right percentage of traces
	assert.InEpsilon(targetTPS/generatedTPS, float64(sampledCount)/(tracesPerPeriod*float64(periods)), 0.2)

	// We should have a throughput of sampled traces around targetTPS
	// Check for 1% epsilon, but the precision also depends on the backend imprecision (error factor = decayFactor).
	// Combine error rates with L1-norm instead of L2-norm by laziness, still good enough for tests.
	assert.InEpsilon(targetTPS, float64(sampledCount)/(float64(periods)*bucketDuration.Seconds()), 0.2)
}

func BenchmarkSampler(b *testing.B) {
	// Benchmark the resource consumption of many traces sampling

	// Up to signatureCount different signatures
	signatureCount := 20

	s := getTestErrorsSampler(10)

	ts := time.Now()
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
		s.Sample(ts, trace, trace[0], defaultEnv)
	}
}
