// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"testing"
	"time"

	fuzz "github.com/google/gofuzz"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

var fuzzer = fuzz.NewWithSeed(1)

func newTestAggregator() *ClientStatsAggregator {
	conf := &config.AgentConfig{
		DefaultEnv: "agentEnv",
		Hostname:   "agentHostname",
	}
	a := NewClientStatsAggregator(conf, make(chan pb.StatsPayload, 100))
	a.Start()
	a.flushTicker.Stop()
	return a
}

func wrapPayload(p pb.ClientStatsPayload) pb.StatsPayload {
	return wrapPayloads([]pb.ClientStatsPayload{p})
}

func wrapPayloads(p []pb.ClientStatsPayload) pb.StatsPayload {
	return pb.StatsPayload{
		AgentEnv:       "agentEnv",
		AgentHostname:  "agentHostname",
		ClientComputed: true,
		Stats:          p,
	}
}

func payloadWithCounts(ts time.Time, k BucketsAggregationKey, hits, errors, duration uint64) pb.ClientStatsPayload {
	return pb.ClientStatsPayload{
		Env:     "test-env",
		Version: "test-version",
		Stats: []pb.ClientStatsBucket{
			{
				Start: uint64(ts.UnixNano()),
				Stats: []pb.ClientGroupedStats{
					{
						Service:        k.Service,
						Name:           k.Name,
						Resource:       k.Resource,
						HTTPStatusCode: k.StatusCode,
						Type:           k.Type,
						Synthetics:     k.Synthetics,
						Hits:           hits,
						Errors:         errors,
						Duration:       duration,
					},
				},
			},
		},
	}
}

func getTestStatsWithStart(start time.Time) pb.ClientStatsPayload {
	b := pb.ClientStatsBucket{}
	fuzzer.Fuzz(&b)
	b.Start = uint64(start.UnixNano())
	p := pb.ClientStatsPayload{}
	fuzzer.Fuzz(&p)
	p.Tags = nil
	p.Stats = []pb.ClientStatsBucket{b}
	return p
}

func assertDistribPayload(t *testing.T, withCounts, res pb.StatsPayload) {
	for j, p := range withCounts.Stats {
		withCounts.Stats[j].AgentAggregation = keyDistributions
		for _, s := range p.Stats {
			for i := range s.Stats {
				s.Stats[i].Hits = 0
				s.Stats[i].Errors = 0
				s.Stats[i].Duration = 0
			}
		}
	}
	assert.Equal(t, withCounts, res)
}

func assertAggCountsPayload(t *testing.T, aggCounts pb.StatsPayload) {
	for _, p := range aggCounts.Stats {
		assert.Empty(t, p.Lang)
		assert.Empty(t, p.TracerVersion)
		assert.Empty(t, p.RuntimeID)
		assert.Equal(t, uint64(0), p.Sequence)
		assert.Equal(t, keyCounts, p.AgentAggregation)
		for _, s := range p.Stats {
			for _, b := range s.Stats {
				assert.Nil(t, b.OkSummary)
				assert.Nil(t, b.ErrorSummary)
			}
		}
	}
}

func agg2Counts(insertionTime time.Time, p pb.ClientStatsPayload) pb.ClientStatsPayload {
	p.Lang = ""
	p.TracerVersion = ""
	p.RuntimeID = ""
	p.Sequence = 0
	p.AgentAggregation = "counts"
	p.Service = ""
	p.ContainerID = ""
	for i, s := range p.Stats {
		p.Stats[i].Start = uint64(alignAggTs(insertionTime).UnixNano())
		p.Stats[i].Duration = uint64(clientBucketDuration.Nanoseconds())
		p.Stats[i].AgentTimeShift = 0
		for j := range s.Stats {
			s.Stats[j].DBType = ""
			s.Stats[j].Hits *= 2
			s.Stats[j].Errors *= 2
			s.Stats[j].Duration *= 2
			s.Stats[j].TopLevelHits = 0
			s.Stats[j].OkSummary = nil
			s.Stats[j].ErrorSummary = nil
		}
	}
	return p
}

func TestAggregatorFlushTime(t *testing.T) {
	assert := assert.New(t)
	a := newTestAggregator()
	testTime := time.Now()
	a.flushOnTime(testTime)
	assert.Len(a.out, 0)
	testPayload := getTestStatsWithStart(testTime)
	a.add(testTime, deepCopy(testPayload))
	a.flushOnTime(testTime)
	assert.Len(a.out, 0)
	a.flushOnTime(testTime.Add(oldestBucketStart - bucketDuration))
	assert.Len(a.out, 0)
	a.flushOnTime(testTime.Add(oldestBucketStart))
	assert.Equal(<-a.out, wrapPayload(testPayload))
	assert.Len(a.buckets, 0)
}

func TestMergeMany(t *testing.T) {
	assert := assert.New(t)
	for i := 0; i < 10; i++ {
		a := newTestAggregator()
		payloadTime := time.Now().Truncate(bucketDuration)
		merge1 := getTestStatsWithStart(payloadTime)
		merge2 := getTestStatsWithStart(payloadTime.Add(time.Nanosecond))
		other := getTestStatsWithStart(payloadTime.Add(-time.Nanosecond))
		merge3 := getTestStatsWithStart(payloadTime.Add(time.Second - time.Nanosecond))

		insertionTime := payloadTime.Add(time.Second)
		a.add(insertionTime, deepCopy(merge1))
		a.add(insertionTime, deepCopy(merge2))
		a.add(insertionTime, deepCopy(other))
		a.add(insertionTime, deepCopy(merge3))
		assert.Len(a.out, 2)
		a.flushOnTime(payloadTime.Add(oldestBucketStart - time.Nanosecond))
		assert.Len(a.out, 3)
		a.flushOnTime(payloadTime.Add(oldestBucketStart))
		assert.Len(a.out, 4)
		assertDistribPayload(t, wrapPayloads([]pb.ClientStatsPayload{merge1, merge2}), <-a.out)
		assertDistribPayload(t, wrapPayload(merge3), <-a.out)
		assert.Equal(wrapPayload(other), <-a.out)
		assertAggCountsPayload(t, <-a.out)
		assert.Len(a.buckets, 0)
	}
}

func TestConcentratorAggregatorNotAligned(t *testing.T) {
	var ts time.Time
	bsize := clientBucketDuration.Nanoseconds()
	for i := 0; i < 50; i++ {
		fuzzer.Fuzz(&ts)
		aggTs := alignAggTs(ts)
		assert.True(t, aggTs.UnixNano()%bsize != 0)
		concentratorTs := alignTs(ts.UnixNano(), bsize)
		assert.True(t, concentratorTs%bsize == 0)
	}
}

func TestTimeShifts(t *testing.T) {
	type tt struct {
		shift, expectedShift time.Duration
		name                 string
	}
	tts := []tt{
		{
			shift:         100 * time.Hour,
			expectedShift: 100 * time.Hour,
			name:          "future",
		},
		{
			shift:         -11 * time.Hour,
			expectedShift: -11*time.Hour + oldestBucketStart - bucketDuration,
			name:          "past",
		},
	}
	for _, tc := range tts {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			a := newTestAggregator()
			agentTime := alignAggTs(time.Now())
			payloadTime := agentTime.Add(tc.shift)

			stats := getTestStatsWithStart(payloadTime)
			a.add(agentTime, deepCopy(stats))
			a.flushOnTime(agentTime)
			assert.Len(a.out, 0)
			a.flushOnTime(agentTime.Add(oldestBucketStart + time.Nanosecond))
			assert.Len(a.out, 1)
			stats.Stats[0].AgentTimeShift = -tc.expectedShift.Nanoseconds()
			stats.Stats[0].Start -= uint64(tc.expectedShift.Nanoseconds())
			assert.Equal(wrapPayload(stats), <-a.out)
		})
	}
}

func TestFuzzCountFields(t *testing.T) {
	assert := assert.New(t)
	for i := 0; i < 30; i++ {
		a := newTestAggregator()
		payloadTime := time.Now().Truncate(bucketDuration)
		merge1 := getTestStatsWithStart(payloadTime)

		insertionTime := payloadTime.Add(time.Second)
		a.add(insertionTime, deepCopy(merge1))
		a.add(insertionTime, deepCopy(merge1))
		assert.Len(a.out, 1)
		a.flushOnTime(payloadTime.Add(oldestBucketStart))
		assert.Len(a.out, 2)
		assertDistribPayload(t, wrapPayloads([]pb.ClientStatsPayload{deepCopy(merge1), deepCopy(merge1)}), <-a.out)
		aggCounts := <-a.out
		expectedAggCounts := wrapPayload(agg2Counts(insertionTime, merge1))
		// map gives random orders post aggregation
		assert.ElementsMatch(aggCounts.Stats[0].Stats[0].Stats, expectedAggCounts.Stats[0].Stats[0].Stats)
		aggCounts.Stats[0].Stats[0].Stats = nil
		expectedAggCounts.Stats[0].Stats[0].Stats = nil
		assert.Equal(expectedAggCounts, aggCounts)
		assert.Len(a.buckets, 0)
	}
}

func TestCountAggregation(t *testing.T) {
	assert := assert.New(t)
	type tt struct {
		k    BucketsAggregationKey
		res  pb.ClientGroupedStats
		name string
	}
	tts := []tt{
		{
			BucketsAggregationKey{Service: "s"},
			pb.ClientGroupedStats{Service: "s"},
			"service",
		},
		{
			BucketsAggregationKey{Name: "n"},
			pb.ClientGroupedStats{Name: "n"},
			"name",
		},
		{
			BucketsAggregationKey{Resource: "r"},
			pb.ClientGroupedStats{Resource: "r"},
			"resource",
		},
		{
			BucketsAggregationKey{Type: "t"},
			pb.ClientGroupedStats{Type: "t"},
			"resource",
		},
		{
			BucketsAggregationKey{Synthetics: true},
			pb.ClientGroupedStats{Synthetics: true},
			"synthetics",
		},
		{
			BucketsAggregationKey{StatusCode: 10},
			pb.ClientGroupedStats{HTTPStatusCode: 10},
			"status",
		},
	}
	for _, tc := range tts {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestAggregator()
			testTime := time.Unix(time.Now().Unix(), 0)

			c1 := payloadWithCounts(testTime, tc.k, 11, 7, 100)
			c2 := payloadWithCounts(testTime, tc.k, 27, 2, 300)
			c3 := payloadWithCounts(testTime, tc.k, 5, 10, 3)
			keyDefault := BucketsAggregationKey{}
			cDefault := payloadWithCounts(testTime, keyDefault, 0, 2, 4)

			assert.Len(a.out, 0)
			a.add(testTime, deepCopy(c1))
			a.add(testTime, deepCopy(c2))
			a.add(testTime, deepCopy(c3))
			a.add(testTime, deepCopy(cDefault))
			assert.Len(a.out, 3)
			a.flushOnTime(testTime.Add(oldestBucketStart + time.Nanosecond))
			assert.Len(a.out, 4)

			assertDistribPayload(t, wrapPayloads([]pb.ClientStatsPayload{c1, c2}), <-a.out)
			assertDistribPayload(t, wrapPayload(c3), <-a.out)
			assertDistribPayload(t, wrapPayload(cDefault), <-a.out)
			aggCounts := <-a.out
			assertAggCountsPayload(t, aggCounts)

			tc.res.Hits = 43
			tc.res.Errors = 19
			tc.res.Duration = 403
			assert.ElementsMatch(aggCounts.Stats[0].Stats[0].Stats, []pb.ClientGroupedStats{
				tc.res,
				{
					Hits:     0,
					Errors:   2,
					Duration: 4,
				},
			})
			assert.Len(a.buckets, 0)
		})
	}
}

func deepCopy(p pb.ClientStatsPayload) pb.ClientStatsPayload {
	new := p
	new.Stats = deepCopyStatsBucket(p.Stats)
	return new
}

func deepCopyStatsBucket(s []pb.ClientStatsBucket) []pb.ClientStatsBucket {
	if s == nil {
		return nil
	}
	new := make([]pb.ClientStatsBucket, len(s))
	for i, b := range s {
		new[i] = b
		new[i].Stats = deepCopyGroupedStats(b.Stats)
	}
	return new
}

func deepCopyGroupedStats(s []pb.ClientGroupedStats) []pb.ClientGroupedStats {
	if s == nil {
		return nil
	}
	new := make([]pb.ClientGroupedStats, len(s))
	for i, b := range s {
		new[i] = b
		if b.OkSummary != nil {
			new[i].OkSummary = make([]byte, len(b.OkSummary))
			copy(new[i].OkSummary, b.OkSummary)
		}
		if b.ErrorSummary != nil {
			new[i].ErrorSummary = make([]byte, len(b.ErrorSummary))
			copy(new[i].ErrorSummary, b.ErrorSummary)
		}
	}
	return new
}
