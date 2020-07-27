// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package stats

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"

	"github.com/stretchr/testify/assert"
)

var (
	testBucketInterval = time.Duration(2 * time.Second).Nanoseconds()
)

func NewTestConcentrator() *Concentrator {
	statsChan := make(chan []Bucket)
	return NewConcentrator([]string{}, time.Second.Nanoseconds(), statsChan)
}

// getTsInBucket gives a timestamp in ns which is `offset` buckets late
func getTsInBucket(alignedNow int64, bsize int64, offset int64) int64 {
	return alignedNow - offset*bsize + rand.Int63n(bsize)
}

// testSpan avoids typo and inconsistency in test spans (typical pitfall: duration, start time,
// and end time are aligned, and end time is the one that needs to be aligned
func testSpan(spanID uint64, parentID uint64, duration, offset int64, service, resource string, err int32) *pb.Span {
	now := time.Now().UnixNano()
	alignedNow := now - now%testBucketInterval

	return &pb.Span{
		SpanID:   spanID,
		ParentID: parentID,
		Duration: duration,
		Start:    getTsInBucket(alignedNow, testBucketInterval, offset) - duration,
		Service:  service,
		Name:     "query",
		Resource: resource,
		Error:    err,
		Type:     "db",
	}
}

// newMeasuredSpan is a function that can make measured spans as test fixtures.
func newMeasuredSpan(spanID uint64, parentID uint64, duration, offset int64, name, service, resource string, err int32) *pb.Span {
	now := time.Now().UnixNano()
	alignedNow := now - now%testBucketInterval

	return &pb.Span{
		SpanID:   spanID,
		ParentID: parentID,
		Duration: duration,
		Start:    getTsInBucket(alignedNow, testBucketInterval, offset) - duration,
		Service:  service,
		Name:     name,
		Resource: resource,
		Error:    err,
		Type:     "db",
		Metrics:  map[string]float64{"_dd.measured": 1},
	}
}

// countValsEq is a test utility function to assert expected == actual for count aggregations.
func countValsEq(t *testing.T, expected map[string]float64, actual map[string]Count) {
	assert := assert.New(t)
	assert.Equal(len(expected), len(actual))
	for key, val := range expected {
		count, ok := actual[key]
		assert.True(ok, "Missing expected key from actual counts: %s", key)
		assert.Equal(val, count.Value)
	}
}

func TestCountValsEq(t *testing.T) {
	ts := TagSet{
		Tag{
			Name:  "env",
			Value: "staging",
		},
		Tag{
			Name:  "service",
			Value: "myservice",
		},
		Tag{
			Name:  "resource",
			Value: "resource1",
		},
	}
	countValsEq(
		t,
		map[string]float64{
			"query|duration|env:staging,service:myservice,resource:resource1": 450.0,
			"query|hits|env:staging,service:myservice,resource:resource1":     1.0,
			"query|errors|env:staging,service:myservice,resource:resource1":   0.0,
		},
		map[string]Count{
			"query|duration|env:staging,service:myservice,resource:resource1": {
				Key:      "query|hits|env:staging,service:myservice,resource:resource1",
				Name:     "query",
				Measure:  "hits",
				TagSet:   ts,
				TopLevel: 1.0,
				Value:    450.0,
			},
			"query|hits|env:staging,service:myservice,resource:resource1": {
				Key:      "query|hits|env:staging,service:myservice,resource:resource1",
				Name:     "query",
				Measure:  "hits",
				TagSet:   ts,
				TopLevel: 1.0,
				Value:    1.0,
			},
			"query|errors|env:staging,service:myservice,resource:resource1": {
				Key:      "query|hits|env:staging,service:myservice,resource:resource1",
				Name:     "query",
				Measure:  "hits",
				TagSet:   ts,
				TopLevel: 1.0,
				Value:    0.0,
			},
		},
	)
}

// TestConcentratorOldestTs tests that the Agent doesn't report time buckets from a
// time before its start
func TestConcentratorOldestTs(t *testing.T) {
	assert := assert.New(t)
	statsChan := make(chan []Bucket)

	now := time.Now().UnixNano()

	// Build that simply have spans spread over time windows.
	trace := pb.Trace{
		testSpan(1, 0, 50, 5, "A1", "resource1", 0),
		testSpan(1, 0, 40, 4, "A1", "resource1", 0),
		testSpan(1, 0, 30, 3, "A1", "resource1", 0),
		testSpan(1, 0, 20, 2, "A1", "resource1", 0),
		testSpan(1, 0, 10, 1, "A1", "resource1", 0),
		testSpan(1, 0, 1, 0, "A1", "resource1", 0),
	}

	traceutil.ComputeTopLevel(trace)
	wt := NewWeightedTrace(trace, traceutil.GetRoot(trace))

	testTrace := &Input{
		Env:   "none",
		Trace: wt,
	}

	t.Run("cold", func(t *testing.T) {
		// Running cold, all spans in the past should end up in the current time bucket.
		flushTime := now
		c := NewConcentrator([]string{}, testBucketInterval, statsChan)
		c.addNow(testTrace, time.Now().UnixNano())

		for i := 0; i < c.bufferLen; i++ {
			stats := c.flushNow(flushTime)
			if !assert.Equal(0, len(stats), "We should get exactly 0 Bucket") {
				t.FailNow()
			}
			flushTime += testBucketInterval
		}

		stats := c.flushNow(flushTime)

		if !assert.Equal(1, len(stats), "We should get exactly 1 Bucket") {
			t.FailNow()
		}

		// First oldest bucket aggregates old past time buckets, so each count
		// should be an aggregated total across the spans.
		expected := map[string]float64{
			"query|duration|env:none,resource:resource1,service:A1": 151,
			"query|hits|env:none,resource:resource1,service:A1":     6,
			"query|errors|env:none,resource:resource1,service:A1":   0,
		}
		countValsEq(t, expected, stats[0].Counts)
	})

	t.Run("hot", func(t *testing.T) {
		flushTime := now
		c := NewConcentrator([]string{}, testBucketInterval, statsChan)
		c.oldestTs = alignTs(now, c.bsize) - int64(c.bufferLen-1)*c.bsize
		c.addNow(testTrace, time.Now().UnixNano())

		for i := 0; i < c.bufferLen-1; i++ {
			stats := c.flushNow(flushTime)
			if !assert.Equal(0, len(stats), "We should get exactly 0 Bucket") {
				t.FailNow()
			}
			flushTime += testBucketInterval
		}

		stats := c.flushNow(flushTime)
		if !assert.Equal(1, len(stats), "We should get exactly 1 Bucket") {
			t.FailNow()
		}
		flushTime += testBucketInterval

		// First oldest bucket aggregates, it should have it all except the
		// last four spans that have offset of 0.
		expected := map[string]float64{
			"query|duration|env:none,resource:resource1,service:A1": 150,
			"query|hits|env:none,resource:resource1,service:A1":     5,
			"query|errors|env:none,resource:resource1,service:A1":   0,
		}
		countValsEq(t, expected, stats[0].Counts)

		stats = c.flushNow(flushTime)
		if !assert.Equal(1, len(stats), "We should get exactly 1 Bucket") {
			t.FailNow()
		}

		// Stats of the last four spans.
		expected = map[string]float64{
			"query|duration|env:none,resource:resource1,service:A1": 1,
			"query|hits|env:none,resource:resource1,service:A1":     1,
			"query|errors|env:none,resource:resource1,service:A1":   0,
		}
		countValsEq(t, expected, stats[0].Counts)
	})
}

//TestConcentratorStatsTotals tests that the total stats are correct, independently of the
// time bucket they end up.
func TestConcentratorStatsTotals(t *testing.T) {
	assert := assert.New(t)
	statsChan := make(chan []Bucket)
	c := NewConcentrator([]string{}, testBucketInterval, statsChan)

	now := time.Now().UnixNano()
	alignedNow := alignTs(now, c.bsize)

	// update oldestTs as it running for quite some time, to avoid the fact that at startup
	// it only allows recent stats.
	c.oldestTs = alignedNow - int64(c.bufferLen)*c.bsize

	// Build that simply have spans spread over time windows.
	trace := pb.Trace{
		testSpan(1, 0, 50, 5, "A1", "resource1", 0),
		testSpan(1, 0, 40, 4, "A1", "resource1", 0),
		testSpan(1, 0, 30, 3, "A1", "resource1", 0),
		testSpan(1, 0, 20, 2, "A1", "resource1", 0),
		testSpan(1, 0, 10, 1, "A1", "resource1", 0),
		testSpan(1, 0, 1, 0, "A1", "resource1", 0),
	}

	traceutil.ComputeTopLevel(trace)
	wt := NewWeightedTrace(trace, traceutil.GetRoot(trace))

	testTrace := &Input{
		Env:   "none",
		Trace: wt,
	}
	c.addNow(testTrace, time.Now().UnixNano())

	var duration float64
	var hits float64
	var errors float64

	flushTime := now
	for i := 0; i <= c.bufferLen; i++ {
		stats := c.flushNow(flushTime)

		if len(stats) == 0 {
			continue
		}

		for key, count := range stats[0].Counts {
			if key == "query|duration|env:none,resource:resource1,service:A1" {
				duration += count.Value
			}
			if key == "query|hits|env:none,resource:resource1,service:A1" {
				hits += count.Value
			}
			if key == "query|errors|env:none,resource:resource1,service:A1" {
				errors += count.Value
			}
		}
		flushTime += c.bsize
	}

	assert.Equal(duration, float64(50+40+30+20+10+1), "Wrong value for total duration %d", duration)
	assert.Equal(hits, float64(len(trace)), "Wrong value for total hits %d", hits)
	assert.Equal(errors, float64(0), "Wrong value for total errors %d", errors)
}

// TestConcentratorStatsCounts tests exhaustively each stats bucket, over multiple time buckets.
func TestConcentratorStatsCounts(t *testing.T) {
	assert := assert.New(t)
	statsChan := make(chan []Bucket)
	c := NewConcentrator([]string{}, testBucketInterval, statsChan)

	now := time.Now().UnixNano()
	alignedNow := alignTs(now, c.bsize)

	// update oldestTs as it running for quite some time, to avoid the fact that at startup
	// it only allows recent stats.
	c.oldestTs = alignedNow - int64(c.bufferLen)*c.bsize

	// Build a trace with stats which should cover 3 time buckets.
	trace := pb.Trace{
		// more than 2 buckets old, should be added to the 2 bucket-old, first flush.
		testSpan(1, 0, 111, 10, "A1", "resource1", 0),
		testSpan(1, 0, 222, 3, "A1", "resource1", 0),
		// 2 buckets old, part of the first flush
		testSpan(1, 0, 24, 2, "A1", "resource1", 0),
		testSpan(2, 0, 12, 2, "A1", "resource1", 2),
		testSpan(3, 0, 40, 2, "A2", "resource2", 2),
		testSpan(4, 0, 300000000000, 2, "A2", "resource2", 2), // 5 minutes trace
		testSpan(5, 0, 30, 2, "A2", "resourcefoo", 0),
		// 1 bucket old, part of the second flush
		testSpan(6, 0, 24, 1, "A1", "resource2", 0),
		testSpan(7, 0, 12, 1, "A1", "resource1", 2),
		testSpan(8, 0, 40, 1, "A2", "resource1", 2),
		testSpan(9, 0, 30, 1, "A2", "resource2", 2),
		testSpan(10, 0, 3600000000000, 1, "A2", "resourcefoo", 0), // 1 hour trace
		// present data, part of the third flush
		testSpan(6, 0, 24, 0, "A1", "resource2", 0),
	}

	expectedCountValByKeyByTime := make(map[int64]map[string]float64)
	// 2-bucket old flush
	expectedCountValByKeyByTime[alignedNow-2*testBucketInterval] = map[string]float64{
		"query|duration|env:none,resource:resource1,service:A1":   369,
		"query|duration|env:none,resource:resource2,service:A2":   300000000040,
		"query|duration|env:none,resource:resourcefoo,service:A2": 30,
		"query|errors|env:none,resource:resource1,service:A1":     1,
		"query|errors|env:none,resource:resource2,service:A2":     2,
		"query|errors|env:none,resource:resourcefoo,service:A2":   0,
		"query|hits|env:none,resource:resource1,service:A1":       4,
		"query|hits|env:none,resource:resource2,service:A2":       2,
		"query|hits|env:none,resource:resourcefoo,service:A2":     1,
	}
	// 1-bucket old flush
	expectedCountValByKeyByTime[alignedNow-1*testBucketInterval] = map[string]float64{
		"query|duration|env:none,resource:resource1,service:A1":   12,
		"query|duration|env:none,resource:resource2,service:A1":   24,
		"query|duration|env:none,resource:resource1,service:A2":   40,
		"query|duration|env:none,resource:resource2,service:A2":   30,
		"query|duration|env:none,resource:resourcefoo,service:A2": 3600000000000,
		"query|errors|env:none,resource:resource1,service:A1":     1,
		"query|errors|env:none,resource:resource2,service:A1":     0,
		"query|errors|env:none,resource:resource1,service:A2":     1,
		"query|errors|env:none,resource:resource2,service:A2":     1,
		"query|errors|env:none,resource:resourcefoo,service:A2":   0,
		"query|hits|env:none,resource:resource1,service:A1":       1,
		"query|hits|env:none,resource:resource2,service:A1":       1,
		"query|hits|env:none,resource:resource1,service:A2":       1,
		"query|hits|env:none,resource:resource2,service:A2":       1,
		"query|hits|env:none,resource:resourcefoo,service:A2":     1,
	}
	// last bucket to be flushed
	expectedCountValByKeyByTime[alignedNow] = map[string]float64{
		"query|duration|env:none,resource:resource2,service:A1": 24,
		"query|errors|env:none,resource:resource2,service:A1":   0,
		"query|hits|env:none,resource:resource2,service:A1":     1,
	}
	expectedCountValByKeyByTime[alignedNow+testBucketInterval] = map[string]float64{}

	traceutil.ComputeTopLevel(trace)
	wt := NewWeightedTrace(trace, traceutil.GetRoot(trace))

	testTrace := &Input{
		Env:   "none",
		Trace: wt,
	}
	c.addNow(testTrace, time.Now().UnixNano())

	// flush every testBucketInterval
	flushTime := now
	for i := 0; i <= c.bufferLen+2; i++ {
		t.Run(fmt.Sprintf("flush-%d", i), func(t *testing.T) {
			stats := c.flushNow(flushTime)

			expectedFlushedTs := alignTs(flushTime, c.bsize) - int64(c.bufferLen)*testBucketInterval
			if len(expectedCountValByKeyByTime[expectedFlushedTs]) == 0 {
				// That's a flush for which we expect no data
				return
			}

			if !assert.Equal(1, len(stats), "We should get exactly 1 Bucket") {
				t.FailNow()
			}

			receivedBuckets := []Bucket{stats[0]}

			assert.Equal(expectedFlushedTs, receivedBuckets[0].Start)

			expectedCountValByKey := expectedCountValByKeyByTime[expectedFlushedTs]
			receivedCounts := receivedBuckets[0].Counts

			countValsEq(t, expectedCountValByKey, receivedCounts)

			// Flushing again at the same time should return nothing
			stats = c.flushNow(flushTime)

			if !assert.Equal(0, len(stats), "Second flush of the same time should be empty") {
				t.FailNow()
			}

		})
		flushTime += c.bsize
	}
}

// TestConcentratorSublayersStatsCounts tests exhaustively the sublayer stats of a single time window.
func TestConcentratorSublayersStatsCounts(t *testing.T) {
	assert := assert.New(t)
	statsChan := make(chan []Bucket)
	c := NewConcentrator([]string{}, testBucketInterval, statsChan)

	now := time.Now().UnixNano()
	alignedNow := now - now%c.bsize

	trace := pb.Trace{
		// first bucket
		testSpan(1, 0, 2000, 0, "A1", "resource1", 0),
		testSpan(2, 1, 1000, 0, "A2", "resource2", 0),
		testSpan(3, 1, 1000, 0, "A2", "resource3", 0),
		testSpan(4, 2, 40, 0, "A3", "resource4", 0),
		testSpan(5, 4, 300, 0, "A3", "resource5", 0),
		testSpan(6, 2, 30, 0, "A3", "resource6", 0),
	}
	traceutil.ComputeTopLevel(trace)
	wt := NewWeightedTrace(trace, traceutil.GetRoot(trace))

	subtraces := ExtractSubtraces(trace, traceutil.GetRoot(trace))
	sublayers := make(map[*pb.Span][]SublayerValue)
	for _, subtrace := range subtraces {
		subtraceSublayers := NewSublayerCalculator().ComputeSublayers(subtrace.Trace)
		sublayers[subtrace.Root] = subtraceSublayers
	}

	testTrace := &Input{
		Env:       "none",
		Trace:     wt,
		Sublayers: sublayers,
	}

	c.addNow(testTrace, time.Now().UnixNano())
	stats := c.flushNow(alignedNow + int64(c.bufferLen)*c.bsize)

	if !assert.Equal(1, len(stats), "We should get exactly 1 Bucket") {
		t.FailNow()
	}

	assert.Equal(alignedNow, stats[0].Start)

	var receivedCounts map[string]Count

	// Start with the first/older bucket
	receivedCounts = stats[0].Counts
	expectedCountValByKey := map[string]float64{
		"query|_sublayers.duration.by_service|env:none,resource:resource1,service:A1,sublayer_service:A1": 2000,
		"query|_sublayers.duration.by_service|env:none,resource:resource1,service:A1,sublayer_service:A2": 2000,
		"query|_sublayers.duration.by_service|env:none,resource:resource1,service:A1,sublayer_service:A3": 370,
		"query|_sublayers.duration.by_service|env:none,resource:resource4,service:A3,sublayer_service:A3": 340,
		"query|_sublayers.duration.by_service|env:none,resource:resource2,service:A2,sublayer_service:A2": 1000,
		"query|_sublayers.duration.by_service|env:none,resource:resource2,service:A2,sublayer_service:A3": 370,
		"query|_sublayers.duration.by_type|env:none,resource:resource1,service:A1,sublayer_type:db":       4370,
		"query|_sublayers.duration.by_type|env:none,resource:resource2,service:A2,sublayer_type:db":       1370,
		"query|_sublayers.duration.by_type|env:none,resource:resource4,service:A3,sublayer_type:db":       340,
		"query|_sublayers.span_count|env:none,resource:resource1,service:A1,:":                            6,
		"query|_sublayers.span_count|env:none,resource:resource2,service:A2,:":                            4,
		"query|_sublayers.span_count|env:none,resource:resource4,service:A3,:":                            2,
		"query|duration|env:none,resource:resource1,service:A1":                                           2000,
		"query|duration|env:none,resource:resource2,service:A2":                                           1000,
		"query|duration|env:none,resource:resource3,service:A2":                                           1000,
		"query|duration|env:none,resource:resource4,service:A3":                                           40,
		"query|duration|env:none,resource:resource6,service:A3":                                           30,
		"query|errors|env:none,resource:resource1,service:A1":                                             0,
		"query|errors|env:none,resource:resource2,service:A2":                                             0,
		"query|errors|env:none,resource:resource3,service:A2":                                             0,
		"query|errors|env:none,resource:resource4,service:A3":                                             0,
		"query|errors|env:none,resource:resource6,service:A3":                                             0,
		"query|hits|env:none,resource:resource1,service:A1":                                               1,
		"query|hits|env:none,resource:resource2,service:A2":                                               1,
		"query|hits|env:none,resource:resource3,service:A2":                                               1,
		"query|hits|env:none,resource:resource4,service:A3":                                               1,
		"query|hits|env:none,resource:resource6,service:A3":                                               1,
	}
	countValsEq(t, expectedCountValByKey, receivedCounts)
}

// TestConcentratorAddNow tests the count aggregation behavior of addNow.
func TestConcentratorAddNow(t *testing.T) {
	now := time.Now().UnixNano()
	for name, test := range map[string]struct {
		in  pb.Trace
		out map[string]float64
	}{
		// case of existing behavior
		"top": {
			pb.Trace{
				testSpan(1, 0, 50, 5, "A1", "resource1", 0),
				testSpan(2, 1, 40, 4, "A1", "resource1", 1),
			},
			map[string]float64{
				"query|duration|env:none,resource:resource1,service:A1":                                           50,
				"query|hits|env:none,resource:resource1,service:A1":                                               1,
				"query|errors|env:none,resource:resource1,service:A1":                                             0,
				"query|_sublayers.duration.by_service|env:none,resource:resource1,service:A1,sublayer_service:A1": 90,
				"query|_sublayers.duration.by_type|env:none,resource:resource1,service:A1,sublayer_type:db":       90,
				"query|_sublayers.span_count|env:none,resource:resource1,service:A1,:":                            2,
			},
		},
		// mixed = first span is both top-level _and_ measured
		"mixed": {
			pb.Trace{
				newMeasuredSpan(1, 0, 50, 5, "http.request", "A1", "resource1", 0),
				testSpan(2, 1, 40, 4, "A1", "resource1", 1),
			},
			map[string]float64{
				"http.request|duration|env:none,resource:resource1,service:A1":                                           50,
				"http.request|hits|env:none,resource:resource1,service:A1":                                               1,
				"http.request|errors|env:none,resource:resource1,service:A1":                                             0,
				"http.request|_sublayers.duration.by_service|env:none,resource:resource1,service:A1,sublayer_service:A1": 90,
				"http.request|_sublayers.duration.by_type|env:none,resource:resource1,service:A1,sublayer_type:db":       90,
				"http.request|_sublayers.span_count|env:none,resource:resource1,service:A1,:":                            2,
			},
		},
		// distinct top-level and measured spans
		// only the top-level span gets sublayer metrics (span name == "query")
		"distinct": {
			pb.Trace{
				testSpan(1, 0, 50, 5, "A1", "resource1", 0),
				newMeasuredSpan(2, 1, 40, 4, "custom_query_op", "A1", "resource1", 1),
				testSpan(3, 2, 50, 5, "A1", "resource1", 0),
			},
			map[string]float64{
				"query|duration|env:none,resource:resource1,service:A1":                                                     50,
				"query|hits|env:none,resource:resource1,service:A1":                                                         1,
				"query|errors|env:none,resource:resource1,service:A1":                                                       0,
				"query|_sublayers.duration.by_service|env:none,resource:resource1,service:A1,sublayer_service:A1":           140,
				"query|_sublayers.duration.by_type|env:none,resource:resource1,service:A1,sublayer_type:db":                 140,
				"query|_sublayers.span_count|env:none,resource:resource1,service:A1,:":                                      3,
				"custom_query_op|duration|env:none,resource:resource1,service:A1":                                           40,
				"custom_query_op|hits|env:none,resource:resource1,service:A1":                                               1,
				"custom_query_op|errors|env:none,resource:resource1,service:A1":                                             1,
				"custom_query_op|_sublayers.duration.by_service|env:none,resource:resource1,service:A1,sublayer_service:A1": 90,
				"custom_query_op|_sublayers.duration.by_type|env:none,resource:resource1,service:A1,sublayer_type:db":       90,
				"custom_query_op|_sublayers.span_count|env:none,resource:resource1,service:A1,:":                            2,
			},
		},
	} {
		t.Run(name, func(*testing.T) {
			statsChan := make(chan []Bucket)
			traceutil.ComputeTopLevel(test.in)
			wt := NewWeightedTrace(test.in, traceutil.GetRoot(test.in))
			testTrace := &Input{
				Env:   "none",
				Trace: wt,
			}
			subtraces := ExtractSubtraces(test.in, traceutil.GetRoot(test.in))
			sublayers := make(map[*pb.Span][]SublayerValue)
			for _, subtrace := range subtraces {
				subtraceSublayers := NewSublayerCalculator().ComputeSublayers(subtrace.Trace)
				sublayers[subtrace.Root] = subtraceSublayers
			}
			testTrace.Sublayers = sublayers
			c := NewConcentrator([]string{}, testBucketInterval, statsChan)
			c.addNow(testTrace, time.Now().UnixNano())
			stats := c.flushNow(now + (int64(c.bufferLen) * testBucketInterval))
			countValsEq(t, test.out, stats[0].Counts)
		})
	}
}
