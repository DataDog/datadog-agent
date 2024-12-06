// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package writer

import (
	"compress/gzip"
	"math"
	"math/rand"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"testing"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/stretchr/testify/assert"
	"github.com/tinylib/msgp/msgp"
)

const (
	testHostname = "agent-test-host"
	testEnv      = "testing"
)

func assertPayload(assert *assert.Assertions, testSets []*pb.StatsPayload, payloads []*payload) {
	expectedHeaders := map[string]string{
		"X-Datadog-Reported-Languages": strings.Join(info.Languages(), "|"),
		"Content-Type":                 "application/msgpack",
		"Content-Encoding":             "gzip",
		"Dd-Api-Key":                   "123",
	}
	var decoded []*pb.StatsPayload
	for _, p := range payloads {
		var statsPayload pb.StatsPayload
		r, err := gzip.NewReader(p.body)
		assert.NoError(err)
		err = msgp.Decode(r, &statsPayload)
		assert.NoError(err)
		for k, v := range expectedHeaders {
			assert.Equal(v, p.headers[k])
		}
		decoded = append(decoded, &statsPayload)
	}
	// Sorting payloads as the sender can alter their order.
	sort.Slice(decoded, func(i, j int) bool {
		return decoded[i].AgentEnv < decoded[j].AgentEnv
	})
	for i, p := range decoded {
		assert.Equal(testSets[i].String(), p.String())
	}
}

func TestStatsWriter(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		assert := assert.New(t)
		sw, srv := testStatsWriter()
		go sw.Run()

		testSets := []*pb.StatsPayload{
			{
				AgentHostname: "1",
				AgentEnv:      "1",
				AgentVersion:  "agent-version",
				Stats: []*pb.ClientStatsPayload{{
					Hostname: testHostname,
					Env:      testEnv,
					Stats: []*pb.ClientStatsBucket{
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
				Stats: []*pb.ClientStatsPayload{{
					Hostname: testHostname,
					Env:      testEnv,
					Stats: []*pb.ClientStatsBucket{
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
					},
				}},
			},
		}
		sw.Write(testSets[0])
		sw.Write(testSets[1])
		sw.Stop()
		assertPayload(assert, testSets, srv.Payloads())
	})

	t.Run("buildPayloads", func(t *testing.T) {
		assert := assert.New(t)
		sw, srv := testStatsWriter()
		srv.Close()
		// This gives us a total of 45 entries. 3 per span, 5
		// spans per stat bucket. Each buckets have the same
		// time window (start: 0, duration 1e9).
		stats := &pb.StatsPayload{
			AgentHostname: "agenthost",
			AgentEnv:      "agentenv",
			AgentVersion:  "agent-version",
			Stats: []*pb.ClientStatsPayload{
				{
					Hostname:         testHostname,
					Env:              testEnv,
					Version:          "version",
					Lang:             "lang",
					TracerVersion:    "tracer-version",
					RuntimeID:        "runtime-id",
					Sequence:         34,
					AgentAggregation: "aggregation",
					Service:          "service",
					ContainerID:      "container-id",
					Stats: []*pb.ClientStatsBucket{
						testutil.RandomBucket(5),
						testutil.RandomBucket(5),
						testutil.RandomBucket(5),
					},
				},
			},
		}

		baseClientPayload := &pb.ClientStatsPayload{
			Hostname:         stats.Stats[0].GetHostname(),
			Env:              stats.Stats[0].GetEnv(),
			Version:          stats.Stats[0].GetVersion(),
			Lang:             stats.Stats[0].GetLang(),
			TracerVersion:    stats.Stats[0].GetTracerVersion(),
			RuntimeID:        stats.Stats[0].GetRuntimeID(),
			Sequence:         stats.Stats[0].GetSequence(),
			AgentAggregation: stats.Stats[0].GetAgentAggregation(),
			Service:          stats.Stats[0].GetService(),
			ContainerID:      stats.Stats[0].GetContainerID(),
		}

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
			assert.Equal(baseClientPayload.String(), actual.String())
		}
		assert.Equal(extractCounts([]*pb.StatsPayload{stats}), extractCounts(payloads))
		for _, p := range payloads {
			assert.True(p.SplitPayload)
			assert.Equal("agentenv", p.AgentEnv)
			assert.Equal("agenthost", p.AgentHostname)
			assert.Equal("agent-version", p.AgentVersion)
		}
	})

	t.Run("no-split", func(t *testing.T) {
		rand.Seed(1)
		assert := assert.New(t)

		sw, srv := testStatsWriter()
		srv.Close()
		// This gives us a total of 45 entries. 3 per span, 5 spans per
		// stat bucket. Each bucket has the same time window (start:
		// 0, duration 1e9).
		stats := &pb.ClientStatsPayload{
			Hostname: testHostname,
			Env:      testEnv,
			Stats: []*pb.ClientStatsBucket{
				testutil.RandomBucket(5),
				testutil.RandomBucket(5),
				testutil.RandomBucket(5),
			},
		}

		payloads := sw.buildPayloads(&pb.StatsPayload{Stats: []*pb.ClientStatsPayload{stats}}, 1337)
		assert.Equal(1, len(payloads))
		s := payloads[0].Stats
		assert.False(payloads[0].SplitPayload)
		assert.Equal(3, len(s[0].Stats))
		assert.Equal(5, len(s[0].Stats[0].Stats))
		assert.Equal(5, len(s[0].Stats[1].Stats))
		assert.Equal(5, len(s[0].Stats[2].Stats))
	})

	t.Run("container-tags", func(t *testing.T) {
		assert := assert.New(t)
		sw, srv := testStatsWriter()
		srv.Close()
		stats := &pb.StatsPayload{
			AgentHostname: "agenthost",
			AgentEnv:      "agentenv",
			AgentVersion:  "agent-version",
			Stats: []*pb.ClientStatsPayload{
				{
					Hostname:         testHostname,
					Env:              testEnv,
					Version:          "version",
					Lang:             "lang",
					TracerVersion:    "tracer-version",
					RuntimeID:        "runtime-id",
					Sequence:         34,
					AgentAggregation: "aggregation",
					Service:          "service",
					ContainerID:      "container-id",
					Tags:             []string{"tag1", "tag2"},
					Stats: []*pb.ClientStatsBucket{
						testutil.RandomBucket(5),
					},
				},
			},
		}

		payloads := sw.buildPayloads(stats, 12)
		assert.Equal(1, len(payloads))
		assert.Equal("container-id", payloads[0].Stats[0].ContainerID)
		assert.Equal([]string{"tag1", "tag2"}, payloads[0].Stats[0].Tags)
	})
}

func TestStatsResetBuffer(t *testing.T) {
	w, _ := testStatsSyncWriter()

	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	assert.Less(t, m.HeapInuse, uint64(50*1e6))

	bigPayload := &pb.StatsPayload{
		AgentHostname: string(make([]byte, 50*1e6)),
	}

	w.payloads = append(w.payloads, bigPayload)

	runtime.GC()
	runtime.ReadMemStats(&m)
	assert.Greater(t, m.HeapInuse, uint64(50*1e6))

	w.resetBuffer()
	runtime.GC()
	runtime.ReadMemStats(&m)
	assert.Less(t, m.HeapInuse, uint64(50*1e6))
}

func TestStatsSyncWriter(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		assert := assert.New(t)
		sw, srv := testStatsSyncWriter()
		go sw.Run()
		testSets := []*pb.StatsPayload{
			{
				Stats: []*pb.ClientStatsPayload{{
					Hostname: testHostname,
					Env:      testEnv,
					Stats: []*pb.ClientStatsBucket{
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
					},
				}},
			},
			{
				Stats: []*pb.ClientStatsPayload{{
					Hostname: testHostname,
					Env:      testEnv,
					Stats: []*pb.ClientStatsBucket{
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
					},
				}},
			},
		}
		sw.Write(testSets[0])
		sw.Write(testSets[1])
		err := sw.FlushSync()
		assert.Nil(err)
		sw.Stop()
		srv.Close()
		assertPayload(assert, testSets, srv.Payloads())
	})

	t.Run("stop", func(t *testing.T) {
		assert := assert.New(t)
		sw, srv := testStatsSyncWriter()
		go sw.Run()

		testSets := []*pb.StatsPayload{
			{
				Stats: []*pb.ClientStatsPayload{{
					Hostname: testHostname,
					Env:      testEnv,
					Stats: []*pb.ClientStatsBucket{
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
					},
				}},
			},
			{
				Stats: []*pb.ClientStatsPayload{{
					Hostname: testHostname,
					Env:      testEnv,
					Stats: []*pb.ClientStatsBucket{
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
						testutil.RandomBucket(3),
					},
				}},
			},
		}
		sw.Write(testSets[0])
		sw.Write(testSets[1])
		sw.Stop()
		srv.Close()
		assertPayload(assert, testSets, srv.Payloads())
	})
}

func TestStatsWriterUpdateAPIKey(t *testing.T) {
	assert := assert.New(t)
	sw, srv := testStatsSyncWriter()
	go sw.Run()
	defer sw.Stop()

	url, err := url.Parse(srv.URL + pathStats)
	assert.NoError(err)

	assert.Len(sw.senders, 1)
	assert.Equal("123", sw.senders[0].cfg.apiKey)
	assert.Equal(url, sw.senders[0].cfg.url)

	sw.UpdateAPIKey("invalid", "foo")
	assert.Equal("123", sw.senders[0].cfg.apiKey)
	assert.Equal(url, sw.senders[0].cfg.url)

	sw.UpdateAPIKey("123", "foo")
	assert.Equal("foo", sw.senders[0].cfg.apiKey)
	assert.Equal(url, sw.senders[0].cfg.url)
	srv.Close()
}

func testStatsWriter() (*DatadogStatsWriter, *testServer) {
	srv := newTestServer()
	cfg := &config.AgentConfig{
		Endpoints:     []*config.Endpoint{{Host: srv.URL, APIKey: "123"}},
		StatsWriter:   &config.WriterConfig{ConnectionLimit: 20, QueueSize: 20},
		ContainerTags: func(_ string) ([]string, error) { return nil, nil },
	}
	return NewStatsWriter(cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}), srv
}

func testStatsSyncWriter() (*DatadogStatsWriter, *testServer) {
	srv := newTestServer()
	cfg := &config.AgentConfig{
		Endpoints:           []*config.Endpoint{{Host: srv.URL, APIKey: "123"}},
		StatsWriter:         &config.WriterConfig{ConnectionLimit: 20, QueueSize: 20},
		SynchronousFlushing: true,
	}
	return NewStatsWriter(cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}), srv
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

func getKey(b *pb.ClientGroupedStats, start, duration uint64) key {
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

func extractCounts(stats []*pb.StatsPayload) map[key]counts {
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
