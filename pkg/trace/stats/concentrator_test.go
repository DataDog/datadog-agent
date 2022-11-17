// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/pb/sketchpb"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

var (
	testBucketInterval = (2 * time.Second).Nanoseconds()
)

func NewTestConcentrator(now time.Time) *Concentrator {
	statsChan := make(chan pb.StatsPayload)
	cfg := config.AgentConfig{
		BucketInterval: time.Duration(testBucketInterval),
		AgentVersion:   "0.99.0",
		DefaultEnv:     "env",
		Hostname:       "hostname",
	}
	return NewConcentrator(&cfg, statsChan, now)
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

func toProcessedTrace(spans []*pb.Span, env, tracerHostname string) *traceutil.ProcessedTrace {
	return &traceutil.ProcessedTrace{
		TracerEnv:      env,
		Root:           traceutil.GetRoot(spans),
		TraceChunk:     spansToTraceChunk(spans),
		TracerHostname: tracerHostname,
	}
}

// spansToTraceChunk converts the given spans to a pb.TraceChunk
func spansToTraceChunk(spans []*pb.Span) *pb.TraceChunk {
	return &pb.TraceChunk{
		Priority: int32(sampler.PriorityNone),
		Spans:    spans,
	}
}

// assertCountsEqual is a test utility function to assert expected == actual for count aggregations.
func assertCountsEqual(t *testing.T, expected []pb.ClientGroupedStats, actual []pb.ClientGroupedStats) {
	expectedM := make(map[string]pb.ClientGroupedStats)
	actualM := make(map[string]pb.ClientGroupedStats)
	for _, e := range expected {
		e.ErrorSummary = nil
		e.OkSummary = nil
		expectedM[e.Service+e.Resource] = e
	}
	for _, a := range actual {
		a.ErrorSummary = nil
		a.OkSummary = nil
		actualM[a.Service+a.Resource] = a
	}
	assert.Equal(t, expectedM, actualM)
}

// TestTracerHostname tests if `Concentrator` uses the tracer hostname rather than agent hostname, if there is one.
func TestTracerHostname(t *testing.T) {
	assert := assert.New(t)
	now := time.Now()

	spans := []*pb.Span{
		testSpan(1, 0, 50, 5, "A1", "resource1", 0),
	}
	traceutil.ComputeTopLevel(spans)
	testTrace := toProcessedTrace(spans, "none", "tracer-hostname")
	c := NewTestConcentrator(now)
	c.addNow(testTrace, "")

	stats := c.flushNow(now.UnixNano() + int64(c.bufferLen)*testBucketInterval)
	assert.Equal("tracer-hostname", stats.Stats[0].Hostname)
}

// TestConcentratorOldestTs tests that the Agent doesn't report time buckets from a
// time before its start
func TestConcentratorOldestTs(t *testing.T) {
	assert := assert.New(t)
	now := time.Now()

	// Build that simply have spans spread over time windows.
	spans := []*pb.Span{
		testSpan(1, 0, 50, 5, "A1", "resource1", 0),
		testSpan(1, 0, 40, 4, "A1", "resource1", 0),
		testSpan(1, 0, 30, 3, "A1", "resource1", 0),
		testSpan(1, 0, 20, 2, "A1", "resource1", 0),
		testSpan(1, 0, 10, 1, "A1", "resource1", 0),
		testSpan(1, 0, 1, 0, "A1", "resource1", 0),
	}

	traceutil.ComputeTopLevel(spans)
	testTrace := toProcessedTrace(spans, "none", "")

	t.Run("cold", func(t *testing.T) {
		// Running cold, all spans in the past should end up in the current time bucket.
		flushTime := now.UnixNano()
		c := NewTestConcentrator(now)
		c.addNow(testTrace, "")

		for i := 0; i < c.bufferLen; i++ {
			stats := c.flushNow(flushTime)
			if !assert.Equal(0, len(stats.Stats), "We should get exactly 0 Bucket") {
				t.FailNow()
			}
			flushTime += testBucketInterval
		}

		stats := c.flushNow(flushTime)

		if !assert.Equal(1, len(stats.Stats), "We should get exactly 1 Bucket") {
			t.FailNow()
		}

		// First oldest bucket aggregates old past time buckets, so each count
		// should be an aggregated total across the spans.
		expected := []pb.ClientGroupedStats{
			{
				Service:      "A1",
				Resource:     "resource1",
				Type:         "db",
				Name:         "query",
				Duration:     151,
				Hits:         6,
				TopLevelHits: 6,
				Errors:       0,
			},
		}
		assertCountsEqual(t, expected, stats.Stats[0].Stats[0].Stats)
	})

	t.Run("hot", func(t *testing.T) {
		flushTime := now.UnixNano()
		c := NewTestConcentrator(now)
		c.oldestTs = alignTs(flushTime, c.bsize) - int64(c.bufferLen-1)*c.bsize
		c.addNow(testTrace, "")

		for i := 0; i < c.bufferLen-1; i++ {
			stats := c.flushNow(flushTime)
			if !assert.Equal(0, len(stats.Stats), "We should get exactly 0 Bucket") {
				t.FailNow()
			}
			flushTime += testBucketInterval
		}

		stats := c.flushNow(flushTime)
		if !assert.Equal(1, len(stats.Stats), "We should get exactly 1 Bucket") {
			t.FailNow()
		}
		flushTime += testBucketInterval

		// First oldest bucket aggregates, it should have it all except the
		// last four spans that have offset of 0.
		expected := []pb.ClientGroupedStats{
			{
				Service:      "A1",
				Resource:     "resource1",
				Type:         "db",
				Name:         "query",
				Duration:     150,
				Hits:         5,
				TopLevelHits: 5,
				Errors:       0,
			},
		}
		assertCountsEqual(t, expected, stats.Stats[0].Stats[0].Stats)

		stats = c.flushNow(flushTime)
		if !assert.Equal(1, len(stats.Stats), "We should get exactly 1 Bucket") {
			t.FailNow()
		}

		// Stats of the last four spans.
		expected = []pb.ClientGroupedStats{
			{
				Service:      "A1",
				Resource:     "resource1",
				Type:         "db",
				Name:         "query",
				Duration:     1,
				Hits:         1,
				TopLevelHits: 1,
				Errors:       0,
			},
		}
		assertCountsEqual(t, expected, stats.Stats[0].Stats[0].Stats)
	})
}

// TestConcentratorStatsTotals tests that the total stats are correct, independently of the
// time bucket they end up.
func TestConcentratorStatsTotals(t *testing.T) {
	assert := assert.New(t)
	now := time.Now()
	c := NewTestConcentrator(now)
	alignedNow := alignTs(now.UnixNano(), c.bsize)

	// update oldestTs as it running for quite some time, to avoid the fact that at startup
	// it only allows recent stats.
	c.oldestTs = alignedNow - int64(c.bufferLen)*c.bsize

	// Build that simply have spans spread over time windows.
	spans := []*pb.Span{
		testSpan(1, 0, 50, 5, "A1", "resource1", 0),
		testSpan(1, 0, 40, 4, "A1", "resource1", 0),
		testSpan(1, 0, 30, 3, "A1", "resource1", 0),
		testSpan(1, 0, 20, 2, "A1", "resource1", 0),
		testSpan(1, 0, 10, 1, "A1", "resource1", 0),
		testSpan(1, 0, 1, 0, "A1", "resource1", 0),
	}

	traceutil.ComputeTopLevel(spans)
	testTrace := toProcessedTrace(spans, "none", "")

	t.Run("ok", func(t *testing.T) {
		c.addNow(testTrace, "")

		var duration uint64
		var hits uint64
		var errors uint64
		var topLevelHits uint64

		flushTime := now.UnixNano()
		for i := 0; i <= c.bufferLen; i++ {
			stats := c.flushNow(flushTime)

			if len(stats.Stats) == 0 {
				continue
			}

			for _, b := range stats.Stats[0].Stats[0].Stats {
				duration += b.Duration
				hits += b.Hits
				errors += b.Errors
				topLevelHits += b.TopLevelHits
			}
			flushTime += c.bsize
		}

		assert.Equal(duration, uint64(50+40+30+20+10+1), "Wrong value for total duration %d", duration)
		assert.Equal(hits, uint64(len(spans)), "Wrong value for total hits %d", hits)
		assert.Equal(topLevelHits, uint64(len(spans)), "Wrong value for total top level hits %d", topLevelHits)
		assert.Equal(errors, uint64(0), "Wrong value for total errors %d", errors)
	})
}

// TestConcentratorStatsCounts tests exhaustively each stats bucket, over multiple time buckets.
func TestConcentratorStatsCounts(t *testing.T) {
	assert := assert.New(t)
	now := time.Now()
	c := NewTestConcentrator(now)

	alignedNow := alignTs(now.UnixNano(), c.bsize)

	// update oldestTs as it running for quite some time, to avoid the fact that at startup
	// it only allows recent stats.
	c.oldestTs = alignedNow - int64(c.bufferLen)*c.bsize

	// Build a trace with stats which should cover 3 time buckets.
	spans := []*pb.Span{
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

	expectedCountValByKeyByTime := make(map[int64][]pb.ClientGroupedStats)
	// 2-bucket old flush
	expectedCountValByKeyByTime[alignedNow-2*testBucketInterval] = []pb.ClientGroupedStats{
		{
			Service:      "A1",
			Resource:     "resource1",
			Type:         "db",
			Name:         "query",
			Duration:     369,
			Hits:         4,
			TopLevelHits: 4,
			Errors:       1,
		},
		{
			Service:      "A2",
			Resource:     "resource2",
			Type:         "db",
			Name:         "query",
			Duration:     300000000040,
			Hits:         2,
			TopLevelHits: 2,
			Errors:       2,
		},
		{
			Service:      "A2",
			Resource:     "resourcefoo",
			Type:         "db",
			Name:         "query",
			Duration:     30,
			Hits:         1,
			TopLevelHits: 1,
			Errors:       0,
		},
	}
	// 1-bucket old flush
	expectedCountValByKeyByTime[alignedNow-testBucketInterval] = []pb.ClientGroupedStats{
		{
			Service:      "A1",
			Resource:     "resource1",
			Type:         "db",
			Name:         "query",
			Duration:     12,
			Hits:         1,
			TopLevelHits: 1,
			Errors:       1,
		},
		{
			Service:      "A1",
			Resource:     "resource2",
			Type:         "db",
			Name:         "query",
			Duration:     24,
			Hits:         1,
			TopLevelHits: 1,
			Errors:       0,
		},
		{
			Service:      "A2",
			Resource:     "resource1",
			Type:         "db",
			Name:         "query",
			Duration:     40,
			Hits:         1,
			TopLevelHits: 1,
			Errors:       1,
		},
		{
			Service:      "A2",
			Resource:     "resource2",
			Type:         "db",
			Name:         "query",
			Duration:     30,
			Hits:         1,
			TopLevelHits: 1,
			Errors:       1,
		},
		{
			Service:      "A2",
			Resource:     "resourcefoo",
			Type:         "db",
			Name:         "query",
			Duration:     3600000000000,
			Hits:         1,
			TopLevelHits: 1,
			Errors:       0,
		},
	}
	// last bucket to be flushed
	expectedCountValByKeyByTime[alignedNow] = []pb.ClientGroupedStats{
		{
			Service:      "A1",
			Resource:     "resource2",
			Type:         "db",
			Name:         "query",
			Duration:     24,
			Hits:         1,
			TopLevelHits: 1,
			Errors:       0,
		},
	}
	expectedCountValByKeyByTime[alignedNow+testBucketInterval] = []pb.ClientGroupedStats{}

	traceutil.ComputeTopLevel(spans)
	testTrace := toProcessedTrace(spans, "none", "")

	c.addNow(testTrace, "")

	// flush every testBucketInterval
	flushTime := now.UnixNano()
	for i := 0; i <= c.bufferLen+2; i++ {
		t.Run(fmt.Sprintf("flush-%d", i), func(t *testing.T) {
			stats := c.flushNow(flushTime)

			expectedFlushedTs := alignTs(flushTime, c.bsize) - int64(c.bufferLen)*testBucketInterval
			if len(expectedCountValByKeyByTime[expectedFlushedTs]) == 0 {
				// That's a flush for which we expect no data
				return
			}
			if !assert.Equal(1, len(stats.Stats), "We should get exactly 1 Bucket") {
				t.FailNow()
			}
			assert.Equal(uint64(expectedFlushedTs), stats.Stats[0].Stats[0].Start)
			expectedCountValByKey := expectedCountValByKeyByTime[expectedFlushedTs]
			assertCountsEqual(t, expectedCountValByKey, stats.Stats[0].Stats[0].Stats)
			assert.Equal("hostname", stats.AgentHostname)
			assert.Equal("env", stats.AgentEnv)
			assert.Equal("0.99.0", stats.AgentVersion)
			assert.Equal(false, stats.ClientComputed)

			// Flushing again at the same time should return nothing
			stats = c.flushNow(flushTime)
			if !assert.Equal(0, len(stats.Stats), "Second flush of the same time should be empty") {
				t.FailNow()
			}

		})
		flushTime += c.bsize
	}
}

func generateDistribution(t *testing.T, generator func(i int) int64) *ddsketch.DDSketch {
	assert := assert.New(t)
	now := time.Now()
	c := NewTestConcentrator(now)
	alignedNow := alignTs(now.UnixNano(), c.bsize)
	// update oldestTs as it running for quite some time, to avoid the fact that at startup
	// it only allows recent stats.
	c.oldestTs = alignedNow - int64(c.bufferLen)*c.bsize
	// Build a trace with stats representing the distribution given by the generator
	spans := []*pb.Span{}
	for i := 0; i < 100; i++ {
		spans = append(spans, testSpan(uint64(i)+1, 0, generator(i), 0, "A1", "resource1", 0))
	}
	traceutil.ComputeTopLevel(spans)
	c.addNow(toProcessedTrace(spans, "none", ""), "")
	stats := c.flushNow(now.UnixNano() + c.bsize*int64(c.bufferLen))
	expectedFlushedTs := alignedNow
	assert.Len(stats.Stats, 1)
	assert.Len(stats.Stats[0].Stats, 1)
	assert.Equal(uint64(expectedFlushedTs), stats.Stats[0].Stats[0].Start)
	assert.Len(stats.Stats[0].Stats, 1)
	b := stats.Stats[0].Stats[0].Stats[0].OkSummary
	var msg sketchpb.DDSketch
	err := proto.Unmarshal(b, &msg)
	assert.Nil(err)
	summary, err := ddsketch.FromProto(&msg)
	assert.Nil(err)
	return summary
}

func TestDistributions(t *testing.T) {
	assert := assert.New(t)
	testQuantiles := []float64{0.1, 0.5, 0.95, 0.99, 1}
	t.Run("constant", func(t *testing.T) {
		constantDistribution := generateDistribution(t, func(i int) int64 { return 42 })
		expectedConstant := []float64{42, 42, 42, 42, 42}
		for i, q := range testQuantiles {
			actual, err := constantDistribution.GetValueAtQuantile(q)
			assert.Nil(err)
			assert.InEpsilon(expectedConstant[i], actual, 0.015)
		}
	})
	t.Run("uniform", func(t *testing.T) {
		uniformDistribution := generateDistribution(t, func(i int) int64 { return int64(i) + 1 })
		expectedUniform := []float64{10, 50, 95, 99, 100}
		for i, q := range testQuantiles {
			actual, err := uniformDistribution.GetValueAtQuantile(q)
			assert.Nil(err)
			assert.InEpsilon(expectedUniform[i], actual, 0.015)
		}
	})
}
func TestIgnoresPartialSpans(t *testing.T) {
	assert := assert.New(t)
	now := time.Now()

	span := testSpan(1, 0, 50, 5, "A1", "resource1", 0)
	span.Metrics = map[string]float64{"_dd.partial_version": 830604}
	spans := []*pb.Span{span}
	traceutil.ComputeTopLevel(spans)

	// we only have one top level but partial. We expect to ignore it when calculating stats
	testTrace := toProcessedTrace(spans, "none", "tracer-hostname")

	c := NewTestConcentrator(now)
	c.addNow(testTrace, "")

	stats := c.flushNow(now.UnixNano() + int64(c.bufferLen)*testBucketInterval)
	assert.Empty(stats.GetStats())
}
