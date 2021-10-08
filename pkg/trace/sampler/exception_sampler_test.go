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
	}
	testTime := time.Unix(13829192398, 0)
	testCases := []testCase{
		{"blocked-p1", false, testTime, map[string]float64{KeySamplingPriority: 1, "_top_level": 1}},
		{"p0-blocked-by-p1", false, testTime, map[string]float64{"_top_level": 1}},
		{"p1-ttl-before-expiration", false, testTime.Add(priorityTTL), map[string]float64{"_top_level": 1}},
		{"p1-ttl-expired", true, testTime.Add(priorityTTL + time.Nanosecond), map[string]float64{"_top_level": 1}},
		{"p0-ttl-active", false, testTime.Add(priorityTTL + time.Nanosecond), map[string]float64{"_top_level": 1}},
		{"p0-ttl-before-expiration", false, testTime.Add(priorityTTL + defaultTTL + time.Nanosecond), map[string]float64{"_top_level": 1}},
		{"p0-ttl-expired", true, testTime.Add(priorityTTL + defaultTTL + 2*time.Nanosecond), map[string]float64{"_dd.measured": 1}},
	}

	e := NewExceptionSampler()
	e.Stop()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			tr := pb.Trace{
				&pb.Span{Service: "s1", Resource: "r1", Metrics: tc.metrics},
			}
			assert.Equal(tc.expected, e.sample(tc.time, "", tr[0], tr))
		})
	}
}

func TestConsideredSpans(t *testing.T) {
	type testCase struct {
		name     string
		expected bool
		service  string
		metrics  map[string]float64
	}
	testTime := time.Unix(13829192398, 0)
	testCases := []testCase{
		{"p1-blocked", false, "s1", map[string]float64{KeySamplingPriority: 1, "_top_level": 1}},
		{"p0-top-passes", true, "s2", map[string]float64{"_top_level": 1}},
		{"p0-measured-passes", true, "s3", map[string]float64{"_dd.measured": 1}},
		{"p0-non-top-non-measured-blocked", false, "s4", nil},
	}

	e := NewExceptionSampler()
	e.Stop()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			tr := pb.Trace{
				&pb.Span{Service: tc.service, Metrics: tc.metrics},
			}
			assert.Equal(tc.expected, e.sample(testTime, "", tr[0], tr))
		})
	}
}

func TestExceptionSamplerRace(t *testing.T) {
	e := NewExceptionSampler()
	e.Stop()
	for i := 0; i < 2; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				tr := pb.Trace{
					&pb.Span{Resource: strconv.Itoa(j), Metrics: map[string]float64{"_top_level": 1}},
				}
				e.sample(time.Now(), "", tr[0], tr)
			}
		}()
	}
}

func TestCardinalityLimit(t *testing.T) {
	assert := assert.New(t)
	e := NewExceptionSampler()
	e.Stop()
	for j := 1; j <= cardinalityLimit; j++ {
		tr := pb.Trace{
			&pb.Span{Resource: strconv.Itoa(j), Metrics: map[string]float64{KeySamplingPriority: 1, "_top_level": 1}},
		}
		e.sample(time.Now(), "", tr[0], tr)
		for _, set := range e.seen {
			assert.Len(set.expires, j)
		}
	}
	tr := pb.Trace{
		&pb.Span{Resource: "newResource", Metrics: map[string]float64{"_top_level": 1}},
	}
	e.sample(time.Now(), "", tr[0], tr)

	assert.Len(e.seen, 1)
	for _, set := range e.seen {
		assert.True(len(set.expires) <= cardinalityLimit)
	}
}
