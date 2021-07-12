// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package writer

import (
	"compress/gzip"
	"math"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/tinylib/msgp/msgp"
)

const (
	testHostname = "agent-test-host"
	testEnv      = "testing"
)

func assertPayload(assert *assert.Assertions, testSets []pb.StatsPayload, payloads []*payload) {
	expectedHeaders := map[string]string{
		"X-Datadog-Reported-Languages": strings.Join(info.Languages(), "|"),
		"Content-Type":                 "application/msgpack",
		"Content-Encoding":             "gzip",
		"Dd-Api-Key":                   "123",
	}
	var decoded []pb.StatsPayload
	for _, p := range payloads {
		var statsPayload pb.StatsPayload
		r, err := gzip.NewReader(p.body)
		assert.NoError(err)
		err = msgp.Decode(r, &statsPayload)
		assert.NoError(err)
		for k, v := range expectedHeaders {
			assert.Equal(v, p.headers[k])
		}
		decoded = append(decoded, statsPayload)
	}
	// Sorting payloads as the sender can alter their order.
	sort.Slice(decoded, func(i, j int) bool {
		return decoded[i].AgentEnv < decoded[j].AgentEnv
	})
	for i, p := range decoded {
		assert.Equal(testSets[i], p)
	}
}

func TestStatsWriter(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		assert := assert.New(t)
		sw, statsChannel, srv := testStatsWriter()
		go sw.Run()

		testSets := []pb.StatsPayload{
			{
				AgentHostname: "1",
				AgentEnv:      "1",
				AgentVersion:  "agent-version",
				Stats: []pb.ClientStatsPayload{{
					Hostname: testHostname,
					Env:      testEnv,
					Stats: []pb.ClientStatsBucket{
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
					},
				}},
			},
			{
				AgentHostname: "2",
				AgentEnv:      "2",
				AgentVersion:  "agent-version",
				Stats: []pb.ClientStatsPayload{{
					Hostname: testHostname,
					Env:      testEnv,
					Stats: []pb.ClientStatsBucket{
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
					},
				}},
			},
		}
		statsChannel <- testSets[0]
		statsChannel <- testSets[1]
		sw.Stop()
		assertPayload(assert, testSets, srv.Payloads())
	})

	t.Run("buildPayloads", func(t *testing.T) {
		assert := assert.New(t)
		sw, _, _ := testStatsWriter()
		// This gives us a total of 45 entries. 3 per span, 5
		// spans per stat bucket. Each buckets have the same
		// time window (start: 0, duration 1e9).
		stats := pb.StatsPayload{
			AgentHostname: "agenthost",
			AgentEnv:      "agentenv",
			AgentVersion:  "agent-version",
			Stats: []pb.ClientStatsPayload{{
				Hostname:         testHostname,
				Env:              testEnv,
				Version:          "version",
				Lang:             "lang",
				TracerVersion:    "tracer-version",
				RuntimeID:        "runtime-id",
				Sequence:         34,
				AgentAggregation: "aggregation",
				Service:          "service",
				Stats: []pb.ClientStatsBucket{
					testutil.RandomBucket(5),
					testutil.RandomBucket(5),
					testutil.RandomBucket(5),
				}},
			},
		}
		baseClientPayload := stats.Stats[0]
		baseClientPayload.Stats = nil
		expectedNbEntries := 15
		expectedNbPayloads := int(math.Ceil(float64(expectedNbEntries) / 12))
		// Compute our expected number of entries by payload
		expectedNbEntriesByPayload := make([]int, expectedNbPayloads)
		for i := 0; i < expectedNbEntries; i++ {
			expectedNbEntriesByPayload[i%expectedNbPayloads]++
		}

		payloads := sw.buildPayloads(stats, 12)

		assert.Equal(expectedNbPayloads, len(payloads))
		for i := 0; i < expectedNbPayloads; i++ {
			assert.Equal(1, len(payloads[i].Stats))
			assert.Equal(1, len(payloads[i].Stats[0].Stats))
			assert.Equal(expectedNbEntriesByPayload[i], len(payloads[i].Stats[0].Stats[0].Stats))
			actual := payloads[i].Stats[0]
			actual.Stats = nil
			assert.Equal(baseClientPayload, actual)
		}
		assert.Equal(extractCounts([]pb.StatsPayload{stats}), extractCounts(payloads))
		for _, p := range payloads {
			assert.Equal("agentenv", p.AgentEnv)
			assert.Equal("agenthost", p.AgentHostname)
			assert.Equal("agent-version", p.AgentVersion)
		}
	})

	t.Run("no-split", func(t *testing.T) {
		rand.Seed(1)
		assert := assert.New(t)

		sw, _, _ := testStatsWriter()
		// This gives us a tota of 45 entries. 3 per span, 5 spans per
		// stat bucket. Each buckets have the same time window (start:
		// 0, duration 1e9).
		stats := pb.ClientStatsPayload{
			Hostname: testHostname,
			Env:      testEnv,
			Stats: []pb.ClientStatsBucket{
				testutil.RandomBucket(5),
				testutil.RandomBucket(5),
				testutil.RandomBucket(5),
			},
		}

		payloads := sw.buildPayloads(pb.StatsPayload{Stats: []pb.ClientStatsPayload{stats}}, 1337)
		assert.Equal(1, len(payloads))
		s := payloads[0].Stats
		assert.Equal(3, len(s[0].Stats))
		assert.Equal(5, len(s[0].Stats[0].Stats))
		assert.Equal(5, len(s[0].Stats[1].Stats))
		assert.Equal(5, len(s[0].Stats[2].Stats))
	})
}

func TestStatsSyncWriter(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		assert := assert.New(t)
		sw, statsChannel, srv := testStatsSyncWriter()
		go sw.Run()
		testSets := []pb.StatsPayload{
			{
				Stats: []pb.ClientStatsPayload{{
					Hostname: testHostname,
					Env:      testEnv,
					Stats: []pb.ClientStatsBucket{
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
					},
				}}},
			{
				Stats: []pb.ClientStatsPayload{{
					Hostname: testHostname,
					Env:      testEnv,
					Stats: []pb.ClientStatsBucket{
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
					},
				}}},
		}
		statsChannel <- testSets[0]
		statsChannel <- testSets[1]
		err := sw.FlushSync()
		assert.Nil(err)
		assertPayload(assert, testSets, srv.Payloads())
	})

	t.Run("stop", func(t *testing.T) {
		assert := assert.New(t)
		sw, statsChannel, srv := testStatsSyncWriter()
		go sw.Run()

		testSets := []pb.StatsPayload{
			{
				Stats: []pb.ClientStatsPayload{{
					Hostname: testHostname,
					Env:      testEnv,
					Stats: []pb.ClientStatsBucket{
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
					},
				}}},
			{
				Stats: []pb.ClientStatsPayload{{
					Hostname: testHostname,
					Env:      testEnv,
					Stats: []pb.ClientStatsBucket{
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
					},
				}}},
		}
		statsChannel <- testSets[0]
		statsChannel <- testSets[1]
		sw.Stop()
		assertPayload(assert, testSets, srv.Payloads())
	})
}

func testStatsWriter() (*StatsWriter, chan pb.StatsPayload, *testServer) {
	srv := newTestServer()
	// We use a blocking channel to make sure that sends get received on the
	// other end.
	in := make(chan pb.StatsPayload)
	cfg := &config.AgentConfig{
		Endpoints:   []*config.Endpoint{{Host: srv.URL, APIKey: "123"}},
		StatsWriter: &config.WriterConfig{ConnectionLimit: 20, QueueSize: 20},
	}
	return NewStatsWriter(cfg, in), in, srv
}

func testStatsSyncWriter() (*StatsWriter, chan pb.StatsPayload, *testServer) {
	srv := newTestServer()
	// We use a blocking channel to make sure that sends get received on the
	// other end.
	in := make(chan pb.StatsPayload)
	cfg := &config.AgentConfig{
		Endpoints:           []*config.Endpoint{{Host: srv.URL, APIKey: "123"}},
		StatsWriter:         &config.WriterConfig{ConnectionLimit: 20, QueueSize: 20},
		SynchronousFlushing: true,
	}
	return NewStatsWriter(cfg, in), in, srv
}

type key struct {
	stats.Aggregation
	start    uint64
	duration uint64
}

type counts struct {
	errors   uint64
	hits     uint64
	duration uint64
}

func getKey(b pb.ClientGroupedStats, start, duration uint64) key {
	return key{
		start:    start,
		duration: duration,
		Aggregation: stats.Aggregation{
			BucketsAggregationKey: stats.BucketsAggregationKey{
				Resource:   b.Resource,
				Service:    b.Service,
				Type:       b.Type,
				StatusCode: b.HTTPStatusCode,
				Synthetics: b.Synthetics,
			},
		},
	}
}

func extractCounts(stats []pb.StatsPayload) map[key]counts {
	counts := make(map[key]counts)
	for _, s := range stats {
		for _, p := range s.Stats {
			for _, b := range p.Stats {
				for _, g := range b.Stats {
					k := getKey(g, b.Start, b.Duration)
					c := counts[k]
					c.duration += g.Duration
					c.hits += g.Hits
					c.errors += g.Errors
				}
			}
		}
	}
	return counts
}
