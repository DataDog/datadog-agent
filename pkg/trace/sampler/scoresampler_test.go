// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

const defaultEnv = "testEnv"

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

func TestDisable(t *testing.T) {
	assert := assert.New(t)

	s := getTestErrorsSampler(0)
	trace, root := getTestTrace()
	for i := 0; i < int(1e2); i++ {
		assert.False(s.Sample(time.Now(), trace, root, defaultEnv))
	}
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
