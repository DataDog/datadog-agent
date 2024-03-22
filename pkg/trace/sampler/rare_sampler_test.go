// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"

	"github.com/DataDog/datadog-go/v5/statsd"
)

func TestSpanSeenTTLExpiration(t *testing.T) {
	type testCase struct {
		name     string
		expected bool
		time     time.Time
		metrics  map[string]float64
		priority SamplingPriority
	}
	c := config.New()
	c.RareSamplerEnabled = true
	testTime := time.Unix(13829192398, 0)
	testCases := []testCase{
		{"blocked-p1", false, testTime, map[string]float64{"_top_level": 1}, PriorityAutoKeep},
		{"p0-blocked-by-p1", false, testTime, map[string]float64{"_top_level": 1}, PriorityNone},
		{"p1-ttl-before-expiration", false, testTime.Add(priorityTTL), map[string]float64{"_top_level": 1}, PriorityNone},
		{"p1-ttl-expired", true, testTime.Add(priorityTTL + time.Nanosecond), map[string]float64{"_top_level": 1}, PriorityNone},
		{"p0-ttl-active", false, testTime.Add(priorityTTL + time.Nanosecond), map[string]float64{"_top_level": 1}, PriorityNone},
		{"p0-ttl-before-expiration", false, testTime.Add(priorityTTL + c.RareSamplerCooldownPeriod + time.Nanosecond), map[string]float64{"_top_level": 1}, PriorityNone},
		{"p0-ttl-expired", true, testTime.Add(priorityTTL + c.RareSamplerCooldownPeriod + 2*time.Nanosecond), map[string]float64{"_dd.measured": 1}, PriorityNone},
	}

	e := NewRareSampler(c, &statsd.NoOpClient{})
	e.Stop()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			span := &pb.Span{Service: "s1", Resource: "r1", Metrics: tc.metrics}
			assert.Equal(tc.expected, e.Sample(tc.time, getTraceChunkWithSpanAndPriority(span, tc.priority), ""))
		})
	}
}

func TestConsideredSpans(t *testing.T) {
	type testCase struct {
		name     string
		expected bool
		service  string
		metrics  map[string]float64
		priority SamplingPriority
	}
	testTime := time.Unix(13829192398, 0)
	testCases := []testCase{
		{"p1-blocked", false, "s1", map[string]float64{"_top_level": 1}, PriorityAutoKeep},
		{"p0-top-passes", true, "s2", map[string]float64{"_top_level": 1}, PriorityNone},
		{"p0-measured-passes", true, "s3", map[string]float64{"_dd.measured": 1}, PriorityNone},
		{"p0-non-top-non-measured-blocked", false, "s4", nil, PriorityNone},
	}

	c := config.New()
	c.RareSamplerEnabled = true
	e := NewRareSampler(c, &statsd.NoOpClient{})
	e.Stop()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			span := &pb.Span{Service: tc.service, Metrics: tc.metrics}
			assert.Equal(tc.expected, e.Sample(testTime, getTraceChunkWithSpanAndPriority(span, tc.priority), ""))
		})
	}
}

func TestRareSamplerRace(_ *testing.T) {
	e := NewRareSampler(config.New(), &statsd.NoOpClient{})
	e.Stop()
	for i := 0; i < 2; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				span := &pb.Span{Resource: strconv.Itoa(j), Metrics: map[string]float64{"_top_level": 1}}
				e.Sample(time.Now(), getTraceChunkWithSpanAndPriority(span, PriorityNone), "")
			}
		}()
	}
}

func TestCardinalityLimit(t *testing.T) {
	assert := assert.New(t)
	c := config.New()
	c.RareSamplerEnabled = true
	e := NewRareSampler(c, &statsd.NoOpClient{})
	e.Stop()
	for j := 1; j <= c.RareSamplerCardinality; j++ {
		span := &pb.Span{Resource: strconv.Itoa(j), Metrics: map[string]float64{"_top_level": 1}}
		e.Sample(time.Now(), getTraceChunkWithSpanAndPriority(span, PriorityAutoKeep), "")
		for _, set := range e.seen {
			assert.Len(set.expires, j)
		}
	}
	span := &pb.Span{Resource: "newResource", Metrics: map[string]float64{"_top_level": 1}}
	e.Sample(time.Now(), getTraceChunkWithSpanAndPriority(span, PriorityNone), "")

	assert.Len(e.seen, 1)
	for _, set := range e.seen {
		assert.True(len(set.expires) <= c.RareSamplerCardinality)
	}
}

func TestMultipleTopeLevels(t *testing.T) {
	assert := assert.New(t)
	c := config.New()
	c.RareSamplerEnabled = true
	e := NewRareSampler(c, &statsd.NoOpClient{})
	e.Stop()
	now := time.Unix(13829192398, 0)
	trace1 := getTraceChunkWithSpansAndPriority(
		[]*pb.Span{
			{Service: "s1", Resource: "r1", Metrics: map[string]float64{"_top_level": 1}},
		},
		PriorityNone,
	)
	trace2 := getTraceChunkWithSpansAndPriority(
		[]*pb.Span{
			{Service: "s1", Resource: "r1", Metrics: map[string]float64{"_top_level": 1}},
			{Service: "s1", Resource: "r2", Metrics: map[string]float64{"_top_level": 1}},
		},
		PriorityNone,
	)

	// sampled because of `r1`
	assert.True(e.Sample(now, trace1, "prod"))
	assert.EqualValues(1, trace1.Spans[0].Metrics["_dd.rare"])

	// sampled because of `r2`
	// `r1`'s timestamp gets refreshed
	assert.True(e.Sample(now.Add(e.ttl), trace2, "prod"))
	assert.NotContains(trace2.Spans[0].Metrics, "_dd.rare")
	assert.EqualValues(1, trace2.Spans[1].Metrics["_dd.rare"])

	// not sampled, because `r1` was sampled on the above
	assert.False(e.Sample(now.Add(e.ttl+time.Nanosecond), trace1, "prod"))
}

func getTraceChunkWithSpanAndPriority(span *pb.Span, priority SamplingPriority) *pb.TraceChunk {
	return getTraceChunkWithSpansAndPriority([]*pb.Span{span}, priority)
}

func getTraceChunkWithSpansAndPriority(spans []*pb.Span, priority SamplingPriority) *pb.TraceChunk {
	return &pb.TraceChunk{
		Priority: int32(priority),
		Spans:    spans,
	}
}
