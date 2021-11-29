package sampler

import (
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
)

func TestSpanSeenTTLExpiration(t *testing.T) {
	type testCase struct {
		name     string
		expected bool
		time     time.Time
		metrics  map[string]float64
		priority SamplingPriority
	}
	testTime := time.Unix(13829192398, 0)
	testCases := []testCase{
		{"blocked-p1", false, testTime, map[string]float64{"_top_level": 1}, PriorityAutoKeep},
		{"p0-blocked-by-p1", false, testTime, map[string]float64{"_top_level": 1}, PriorityNone},
		{"p1-ttl-before-expiration", false, testTime.Add(priorityTTL), map[string]float64{"_top_level": 1}, PriorityNone},
		{"p1-ttl-expired", true, testTime.Add(priorityTTL + time.Nanosecond), map[string]float64{"_top_level": 1}, PriorityNone},
		{"p0-ttl-active", false, testTime.Add(priorityTTL + time.Nanosecond), map[string]float64{"_top_level": 1}, PriorityNone},
		{"p0-ttl-before-expiration", false, testTime.Add(priorityTTL + defaultTTL + time.Nanosecond), map[string]float64{"_top_level": 1}, PriorityNone},
		{"p0-ttl-expired", true, testTime.Add(priorityTTL + defaultTTL + 2*time.Nanosecond), map[string]float64{"_dd.measured": 1}, PriorityNone},
	}

	e := NewRareSampler()
	e.Stop()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			span := &pb.Span{Service: "s1", Resource: "r1", Metrics: tc.metrics}
			assert.Equal(tc.expected, e.sample(tc.time, "", getTraceChunkWithSpanAndPriority(span, tc.priority)))
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

	e := NewRareSampler()
	e.Stop()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			span := &pb.Span{Service: tc.service, Metrics: tc.metrics}
			assert.Equal(tc.expected, e.sample(testTime, "", getTraceChunkWithSpanAndPriority(span, tc.priority)))
		})
	}
}

func TestRareSamplerRace(t *testing.T) {
	e := NewRareSampler()
	e.Stop()
	for i := 0; i < 2; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				span := &pb.Span{Resource: strconv.Itoa(j), Metrics: map[string]float64{"_top_level": 1}}
				e.sample(time.Now(), "", getTraceChunkWithSpanAndPriority(span, PriorityNone))
			}
		}()
	}
}

func TestCardinalityLimit(t *testing.T) {
	assert := assert.New(t)
	e := NewRareSampler()
	e.Stop()
	for j := 1; j <= cardinalityLimit; j++ {
		span := &pb.Span{Resource: strconv.Itoa(j), Metrics: map[string]float64{"_top_level": 1}}
		e.sample(time.Now(), "", getTraceChunkWithSpanAndPriority(span, PriorityAutoKeep))
		for _, set := range e.seen {
			assert.Len(set.expires, j)
		}
	}
	span := &pb.Span{Resource: "newResource", Metrics: map[string]float64{"_top_level": 1}}
	e.sample(time.Now(), "", getTraceChunkWithSpanAndPriority(span, PriorityNone))

	assert.Len(e.seen, 1)
	for _, set := range e.seen {
		assert.True(len(set.expires) <= cardinalityLimit)
	}
}

func getTraceChunkWithSpanAndPriority(span *pb.Span, priority SamplingPriority) *pb.TraceChunk {
	return &pb.TraceChunk{
		Priority: int32(priority),
		Spans: []*pb.Span{
			span,
		},
	}
}
