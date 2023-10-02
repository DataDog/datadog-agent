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
	"google.golang.org/protobuf/runtime/protoiface"

	proto "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

var fuzzer = fuzz.NewWithSeed(1)

func newTestAggregator() *ClientStatsAggregator {
	conf := &config.AgentConfig{
		DefaultEnv: "agentEnv",
		Hostname:   "agentHostname",
	}
	a := NewClientStatsAggregator(conf, make(chan *proto.StatsPayload, 100))
	a.Start()
	a.flushTicker.Stop()
	return a
}

func wrapPayload(p *proto.ClientStatsPayload) *proto.StatsPayload {
	return wrapPayloads([]*proto.ClientStatsPayload{p})
}

func wrapPayloads(p []*proto.ClientStatsPayload) *proto.StatsPayload {
	return &proto.StatsPayload{
		AgentEnv:       "agentEnv",
		AgentHostname:  "agentHostname",
		ClientComputed: true,
		Stats:          p,
	}
}

func payloadWithCounts(ts time.Time, k BucketsAggregationKey, hits, errors, duration uint64) *proto.ClientStatsPayload {
	return &proto.ClientStatsPayload{
		Env:     "test-env",
		Version: "test-version",
		Stats: []*proto.ClientStatsBucket{
			{
				Start: uint64(ts.UnixNano()),
				Stats: []*proto.ClientGroupedStats{
					{
						Service:        k.Service,
						PeerService:    k.PeerService,
						Name:           k.Name,
						SpanKind:       k.SpanKind,
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

func getTestStatsWithStart(start time.Time) *proto.ClientStatsPayload {
	b := &proto.ClientStatsBucket{}
	fuzzer.Fuzz(b)
	b.Start = uint64(start.UnixNano())
	p := &proto.ClientStatsPayload{}
	fuzzer.Fuzz(p)
	p.Tags = nil
	p.Stats = []*proto.ClientStatsBucket{b}
	return p
}

func assertDistribPayload(t *testing.T, withCounts, res *proto.StatsPayload) {
	for j, p := range withCounts.Stats {
		withCounts.Stats[j].AgentAggregation = keyDistributions
		for _, s := range p.Stats {
			for i := range s.Stats {
				if s.Stats[i] == nil {
					continue
				}
				s.Stats[i].Hits = 0
				s.Stats[i].Errors = 0
				s.Stats[i].Duration = 0
			}
		}
	}
	assert.Equal(t, withCounts.String(), res.String())
}

func assertAggCountsPayload(t *testing.T, aggCounts *proto.StatsPayload) {
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

func agg2Counts(insertionTime time.Time, p *proto.ClientStatsPayload) *proto.ClientStatsPayload {
	p.Lang = ""
	p.TracerVersion = ""
	p.RuntimeID = ""
	p.Sequence = 0
	p.AgentAggregation = "counts"
	p.Service = ""
	p.ContainerID = ""
	for _, s := range p.Stats {
		s.Start = uint64(alignAggTs(insertionTime).UnixNano())
		s.Duration = uint64(clientBucketDuration.Nanoseconds())
		s.AgentTimeShift = 0
		for _, stat := range s.Stats {
			if stat == nil {
				continue
			}
			stat.DBType = ""
			stat.Hits *= 2
			stat.Errors *= 2
			stat.Duration *= 2
			stat.TopLevelHits = 0
			stat.OkSummary = nil
			stat.ErrorSummary = nil
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
	s := <-a.out
	assert.Equal(s.String(), wrapPayload(testPayload).String())
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
		assertDistribPayload(t, wrapPayloads([]*proto.ClientStatsPayload{merge1, merge2}), <-a.out)
		assertDistribPayload(t, wrapPayload(merge3), <-a.out)
		s := <-a.out
		assert.Equal(wrapPayload(other).String(), s.String())
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
			s := <-a.out
			assert.Equal(wrapPayload(stats).String(), s.String())
		})
	}
}

func TestFuzzCountFields(t *testing.T) {
	assert := assert.New(t)
	for i := 0; i < 30; i++ {
		a := newTestAggregator()
		// Ensure that peer.service aggregation is on. Some tests may expect non-empty values for peer.service.
		a.peerSvcAggregation = true
		payloadTime := time.Now().Truncate(bucketDuration)
		merge1 := getTestStatsWithStart(payloadTime)

		insertionTime := payloadTime.Add(time.Second)
		a.add(insertionTime, deepCopy(merge1))
		a.add(insertionTime, deepCopy(merge1))
		assert.Len(a.out, 1)
		a.flushOnTime(payloadTime.Add(oldestBucketStart))
		assert.Len(a.out, 2)
		assertDistribPayload(t, wrapPayloads([]*proto.ClientStatsPayload{deepCopy(merge1), deepCopy(merge1)}), <-a.out)
		aggCounts := <-a.out
		expectedAggCounts := wrapPayload(agg2Counts(insertionTime, merge1))

		// map gives random orders post aggregation

		actual := []protoiface.MessageV1{}
		expected := []protoiface.MessageV1{}
		for _, s := range expectedAggCounts.Stats[0].Stats[0].Stats {
			if s == nil {
				continue
			}
			actual = append(actual, s)
		}
		for _, s := range aggCounts.Stats[0].Stats[0].Stats {
			if s == nil {
				continue
			}
			expected = append(expected, s)
		}

		assert.ElementsMatch(pb.PbToStringSlice(expected), pb.PbToStringSlice(actual))
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
		res  *proto.ClientGroupedStats
		name string
	}
	tts := []tt{
		{
			BucketsAggregationKey{Service: "s"},
			&proto.ClientGroupedStats{Service: "s"},
			"service",
		},
		{
			BucketsAggregationKey{Name: "n"},
			&proto.ClientGroupedStats{Name: "n"},
			"name",
		},
		{
			BucketsAggregationKey{Resource: "r"},
			&proto.ClientGroupedStats{Resource: "r"},
			"resource",
		},
		{
			BucketsAggregationKey{Type: "t"},
			&proto.ClientGroupedStats{Type: "t"},
			"resource",
		},
		{
			BucketsAggregationKey{Synthetics: true},
			&proto.ClientGroupedStats{Synthetics: true},
			"synthetics",
		},
		{
			BucketsAggregationKey{StatusCode: 10},
			&proto.ClientGroupedStats{HTTPStatusCode: 10},
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

			assertDistribPayload(t, wrapPayloads([]*proto.ClientStatsPayload{c1, c2}), <-a.out)
			assertDistribPayload(t, wrapPayload(c3), <-a.out)
			assertDistribPayload(t, wrapPayload(cDefault), <-a.out)
			aggCounts := <-a.out
			assertAggCountsPayload(t, aggCounts)

			tc.res.Hits = 43
			tc.res.Errors = 19
			tc.res.Duration = 403
			assert.ElementsMatch(aggCounts.Stats[0].Stats[0].Stats, []*proto.ClientGroupedStats{
				tc.res,
				// Additional grouped stat object that corresponds to the keyDefault/cDefault.
				// We do not expect this to be aggregated with the non-default key in the test.
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

func TestCountAggregationPeerService(t *testing.T) {
	assert := assert.New(t)
	type tt struct {
		k                BucketsAggregationKey
		res              *proto.ClientGroupedStats
		name             string
		enablePeerSvcAgg bool
	}
	tts := []tt{
		{
			BucketsAggregationKey{Service: "s"},
			&proto.ClientGroupedStats{Service: "s"},
			"service",
			false,
		},
		{
			BucketsAggregationKey{Name: "n"},
			&proto.ClientGroupedStats{Name: "n"},
			"name",
			false,
		},
		{
			BucketsAggregationKey{Resource: "r"},
			&proto.ClientGroupedStats{Resource: "r"},
			"resource",
			false,
		},
		{
			BucketsAggregationKey{Type: "t"},
			&proto.ClientGroupedStats{Type: "t"},
			"resource",
			false,
		},
		{
			BucketsAggregationKey{Synthetics: true},
			&proto.ClientGroupedStats{Synthetics: true},
			"synthetics",
			false,
		},
		{
			BucketsAggregationKey{StatusCode: 10},
			&proto.ClientGroupedStats{HTTPStatusCode: 10},
			"status",
			false,
		},
		{
			BucketsAggregationKey{Service: "s", PeerService: "remote-service"},
			&proto.ClientGroupedStats{Service: "s", PeerService: ""},
			"peer.service disabled",
			false,
		},
		{
			BucketsAggregationKey{Service: "s", PeerService: "remote-service"},
			&proto.ClientGroupedStats{Service: "s", PeerService: "remote-service"},
			"peer.service enabled",
			true,
		},
		{
			BucketsAggregationKey{SpanKind: "client"},
			&proto.ClientGroupedStats{SpanKind: "client"},
			"span.kind",
			false,
		},
	}
	for _, tc := range tts {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestAggregator()
			a.peerSvcAggregation = tc.enablePeerSvcAgg
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

			assertDistribPayload(t, wrapPayloads([]*proto.ClientStatsPayload{c1, c2}), <-a.out)
			assertDistribPayload(t, wrapPayload(c3), <-a.out)
			assertDistribPayload(t, wrapPayload(cDefault), <-a.out)
			aggCounts := <-a.out
			assertAggCountsPayload(t, aggCounts)

			tc.res.Hits = 43
			tc.res.Errors = 19
			tc.res.Duration = 403
			assert.ElementsMatch(aggCounts.Stats[0].Stats[0].Stats, []*proto.ClientGroupedStats{
				tc.res,
				// Additional grouped stat object that corresponds to the keyDefault/cDefault.
				// We do not expect this to be aggregated with the non-default key in the test.
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

func TestCountAggregationPeerTags(t *testing.T) {
	assert := assert.New(t)
	peerTags := []string{"db.instance:a", "db.system:b"}
	type tt struct {
		k                BucketsAggregationKey
		res              *proto.ClientGroupedStats
		name             string
		enablePeerSvcAgg bool
	}
	tts := []tt{
		{
			BucketsAggregationKey{Service: "s", PeerService: "remote-service"},
			&proto.ClientGroupedStats{Service: "s", PeerService: ""},
			"peer.service aggregation disabled",
			false,
		},
		{
			BucketsAggregationKey{Service: "s", PeerService: "remote-service"},
			&proto.ClientGroupedStats{Service: "s", PeerService: "remote-service", PeerTags: peerTags},
			"peer.service aggregation enabled",
			true,
		},
	}
	for _, tc := range tts {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestAggregator()
			a.peerSvcAggregation = tc.enablePeerSvcAgg
			testTime := time.Unix(time.Now().Unix(), 0)

			c1 := payloadWithCounts(testTime, tc.k, 11, 7, 100)
			c2 := payloadWithCounts(testTime, tc.k, 27, 2, 300)
			c3 := payloadWithCounts(testTime, tc.k, 5, 10, 3)
			c1.Stats[0].Stats[0].PeerTags = peerTags
			c2.Stats[0].Stats[0].PeerTags = peerTags
			c3.Stats[0].Stats[0].PeerTags = peerTags
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

			assertDistribPayload(t, wrapPayloads([]*proto.ClientStatsPayload{c1, c2}), <-a.out)
			assertDistribPayload(t, wrapPayload(c3), <-a.out)
			assertDistribPayload(t, wrapPayload(cDefault), <-a.out)
			aggCounts := <-a.out
			assertAggCountsPayload(t, aggCounts)

			tc.res.Hits = 43
			tc.res.Errors = 19
			tc.res.Duration = 403
			assert.ElementsMatch(aggCounts.Stats[0].Stats[0].Stats, []*proto.ClientGroupedStats{
				tc.res,
				// Additional grouped stat object that corresponds to the keyDefault/cDefault.
				// We do not expect this to be aggregated with the non-default key in the test.
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

func TestNewBucketAggregationKeyPeerService(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		assert := assert.New(t)
		r := newBucketAggregationKey(&proto.ClientGroupedStats{Service: "a", PeerService: "remote-test"}, false)
		assert.Equal(BucketsAggregationKey{Service: "a"}, r)
	})
	t.Run("enabled", func(t *testing.T) {
		assert := assert.New(t)
		r := newBucketAggregationKey(&proto.ClientGroupedStats{Service: "a", PeerService: "remote-test"}, true)
		assert.Equal(BucketsAggregationKey{Service: "a", PeerService: "remote-test"}, r)
	})
}

func deepCopy(p *proto.ClientStatsPayload) *proto.ClientStatsPayload {
	new := &proto.ClientStatsPayload{
		Hostname:         p.GetHostname(),
		Env:              p.GetEnv(),
		Version:          p.GetVersion(),
		Lang:             p.GetLang(),
		TracerVersion:    p.GetTracerVersion(),
		RuntimeID:        p.GetRuntimeID(),
		Sequence:         p.GetSequence(),
		AgentAggregation: p.GetAgentAggregation(),
		Service:          p.GetService(),
		ContainerID:      p.GetContainerID(),
		Tags:             p.GetTags(),
	}
	new.Stats = deepCopyStatsBucket(p.Stats)
	return new
}

func deepCopyStatsBucket(s []*proto.ClientStatsBucket) []*proto.ClientStatsBucket {
	if s == nil {
		return nil
	}
	new := make([]*proto.ClientStatsBucket, len(s))
	for i, b := range s {
		new[i] = &proto.ClientStatsBucket{
			Start:          b.GetStart(),
			Duration:       b.GetDuration(),
			AgentTimeShift: b.GetAgentTimeShift(),
		}
		new[i].Stats = deepCopyGroupedStats(b.Stats)
	}
	return new
}

func deepCopyGroupedStats(s []*proto.ClientGroupedStats) []*proto.ClientGroupedStats {
	if s == nil {
		return nil
	}
	new := make([]*proto.ClientGroupedStats, len(s))
	for i, b := range s {
		if b == nil {
			new[i] = nil
			continue
		}

		new[i] = &proto.ClientGroupedStats{
			Service:        b.GetService(),
			Name:           b.GetName(),
			Resource:       b.GetResource(),
			HTTPStatusCode: b.GetHTTPStatusCode(),
			Type:           b.GetType(),
			DBType:         b.GetDBType(),
			Hits:           b.GetHits(),
			Errors:         b.GetErrors(),
			Duration:       b.GetDuration(),
			Synthetics:     b.GetSynthetics(),
			TopLevelHits:   b.GetTopLevelHits(),
			PeerService:    b.GetPeerService(),
			SpanKind:       b.GetSpanKind(),
			PeerTags:       b.GetPeerTags(),
		}
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
