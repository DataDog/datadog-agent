package stats

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	fuzz "github.com/google/gofuzz"
	"github.com/stretchr/testify/assert"
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
	return pb.StatsPayload{
		AgentEnv:       "agentEnv",
		AgentHostname:  "agentHostname",
		ClientComputed: true,
		Stats:          []pb.ClientStatsPayload{p},
	}
}

func payloadWithCounts(ts time.Time, k bucketAggregationKey, hits, errors, duration uint64) pb.ClientStatsPayload {
	return pb.ClientStatsPayload{
		Env:     "test-env",
		Version: "test-version",
		Stats: []pb.ClientStatsBucket{
			{
				Start: uint64(ts.UnixNano()),
				Stats: []pb.ClientGroupedStats{
					{
						Service:        k.service,
						Name:           k.name,
						Resource:       k.resource,
						HTTPStatusCode: k.statusCode,
						Type:           k.typ,
						Synthetics:     k.synthetics,
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
	stats := pb.ClientStatsPayload{
		Stats:   []pb.ClientStatsBucket{b},
		Version: "0.1",
	}
	return stats
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
	a.flushOnTime(testTime.Add(19 * time.Second))
	assert.Len(a.out, 0)
	a.flushOnTime(testTime.Add(21 * time.Second))
	assert.Equal(<-a.out, wrapPayload(testPayload))
	assert.Len(a.buckets, 0)
}

func TestMergeMany(t *testing.T) {
	assert := assert.New(t)
	for i := 0; i < 10; i++ {
		a := newTestAggregator()
		testTime := time.Unix(time.Now().Unix(), 0)
		merge1 := getTestStatsWithStart(testTime)
		merge2 := getTestStatsWithStart(testTime.Add(time.Nanosecond))
		other := getTestStatsWithStart(testTime.Add(-time.Nanosecond))
		merge3 := getTestStatsWithStart(testTime.Add(time.Second - time.Nanosecond))

		a.add(testTime, deepCopy(merge1))
		a.add(testTime, deepCopy(merge2))
		a.add(testTime, deepCopy(other))
		a.add(testTime, deepCopy(merge3))
		assert.Len(a.out, 3)
		a.flushOnTime(testTime.Add(20 * time.Second))
		assert.Len(a.out, 4)
		a.flushOnTime(testTime.Add(21 * time.Second))
		assert.Len(a.out, 5)

		assertDistribPayload(t, wrapPayload(merge1), <-a.out)
		assertDistribPayload(t, wrapPayload(merge2), <-a.out)
		assertDistribPayload(t, wrapPayload(merge3), <-a.out)
		assert.Equal(wrapPayload(other), <-a.out)
		assertAggCountsPayload(t, <-a.out)
		assert.Len(a.buckets, 0)
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
			expectedShift: -11*time.Hour + oldestBucketStart,
			name:          "past",
		},
	}
	for _, tc := range tts {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			a := newTestAggregator()
			agentTime := time.Unix(time.Now().Unix(), 0)
			payloadTime := time.Unix(time.Now().Unix(), 0).Add(tc.shift)

			stats := getTestStatsWithStart(payloadTime)
			a.add(agentTime, deepCopy(stats))
			a.flushOnTime(agentTime)
			assert.Len(a.out, 0)
			a.flushOnTime(agentTime.Add(21 * time.Second))
			assert.Len(a.out, 1)
			stats.Stats[0].AgentTimeShift = -tc.expectedShift.Nanoseconds()
			stats.Stats[0].Start -= uint64(tc.expectedShift.Nanoseconds())
			assert.Equal(wrapPayload(stats), <-a.out)
		})
	}
}

func TestCountAggregation(t *testing.T) {
	assert := assert.New(t)
	a := newTestAggregator()
	testTime := time.Unix(time.Now().Unix(), 0)
	k := bucketAggregationKey{synthetics: true}
	c1 := payloadWithCounts(testTime, k, 11, 7, 100)
	c2 := payloadWithCounts(testTime, k, 27, 2, 300)
	c3 := payloadWithCounts(testTime, k, 5, 10, 3)
	k55 := bucketAggregationKey{synthetics: false}
	c55 := payloadWithCounts(testTime, k55, 0, 2, 4)

	assert.Len(a.out, 0)
	a.add(testTime, deepCopy(c1))
	a.add(testTime, deepCopy(c2))
	a.add(testTime, deepCopy(c3))
	a.add(testTime, deepCopy(c55))
	assert.Len(a.out, 4)
	a.flushOnTime(testTime.Add(21 * time.Second))
	assert.Len(a.out, 5)

	assertDistribPayload(t, wrapPayload(c1), <-a.out)
	assertDistribPayload(t, wrapPayload(c2), <-a.out)
	assertDistribPayload(t, wrapPayload(c3), <-a.out)
	assertDistribPayload(t, wrapPayload(c55), <-a.out)
	aggCounts := <-a.out
	assertAggCountsPayload(t, aggCounts)
	assert.ElementsMatch(aggCounts.Stats[0].Stats[0].Stats, []pb.ClientGroupedStats{
		{
			Synthetics: true,
			Hits:       43,
			Errors:     19,
			Duration:   403,
		},
		{
			Hits:     0,
			Errors:   2,
			Duration: 4,
		},
	})
	assert.Len(a.buckets, 0)
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
