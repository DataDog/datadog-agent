// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build test

package serverimpl

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func batchShardContextKey(sample metrics.MetricSample) ckey.ContextKey {
	keyGenerator := ckey.NewKeyGenerator()
	tags := tagset.NewHashingTagsAccumulatorWithTags(sample.Tags)
	return keyGenerator.Generate(sample.Name, sample.Host, tags)
}

func TestMilestone0BatchShardIdentityBaseline(t *testing.T) {
	base := metrics.MetricSample{
		Name:       "identity.metric",
		Host:       "host-a",
		Tags:       []string{"env:prod", "service:web", "env:prod"},
		Mtype:      metrics.GaugeType,
		Value:      1,
		SampleRate: 0.5,
		Timestamp:  123,
		OriginInfo: taggertypes.OriginInfo{ContainerIDFromSocket: "container-a", Cardinality: "low"},
		ListenerID: "udp-127.0.0.1:8125",
		Source:     metrics.MetricSourceDogstatsd,
	}

	reorderedTags := base
	reorderedTags.Tags = []string{"service:web", "env:prod"}
	changedOrigin := base
	changedOrigin.OriginInfo = taggertypes.OriginInfo{ContainerIDFromSocket: "container-b", Cardinality: "high"}
	changedOrigin.ListenerID = "uds-unixgram-7"
	changedOrigin.Mtype = metrics.CounterType
	changedOrigin.SampleRate = 1
	changedOrigin.Timestamp = 0
	changedHost := base
	changedHost.Host = "host-b"
	changedTags := base
	changedTags.Tags = []string{"env:prod", "service:api"}

	baseKey := batchShardContextKey(base)
	assert.Equal(t, baseKey, batchShardContextKey(reorderedTags), "batch shard identity deduplicates and order-normalizes client tags")
	assert.Equal(t, baseKey, batchShardContextKey(changedOrigin), "batch shard identity currently ignores origin/listener/type/rate/timestamp fields")
	assert.NotEqual(t, baseKey, batchShardContextKey(changedHost), "batch shard identity includes host")
	assert.NotEqual(t, baseKey, batchShardContextKey(changedTags), "batch shard identity includes client tags")

	for _, shards := range []int{1, 2, 8, 32} {
		t.Run(fmt.Sprintf("shards_%d", shards), func(t *testing.T) {
			generator := newShardGenerator()
			assert.Equal(t, fastrange(baseKey, shards), generator.Generate(base, shards), "shard generator must remain equivalent to context-key fastrange")
			assert.Equal(t, generator.Generate(base, shards), generator.Generate(reorderedTags, shards))
			assert.Equal(t, generator.Generate(base, shards), generator.Generate(changedOrigin, shards))
		})
	}
}

func TestMilestone0ParseEnrichMapperHostAndLineageBaseline(t *testing.T) {
	deps := fulfillDepsWithConfigYaml(t, `
dogstatsd_port: __random__
dogstatsd_mapper_profiles:
  - name: test
    prefix: 'test.'
    mappings:
      - match: "test.job.duration.*.*"
        name: "test.job.duration"
        tags:
          job_type: "$1"
          job_name: "$2"
`)
	s := deps.Server.(*dsdServer)
	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)

	samples, err := s.parseMetricMessage(nil, parser,
		[]byte("test.job.duration.api.sync:42|g|@0.25|#client:tag,host:custom-host,dd.internal.card:high"),
		"container-from-uds", 4242, "udp-127.0.0.1:8125", false, nil)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	sample := samples[0]
	assert.Equal(t, "test.job.duration", sample.Name, "mapper may rewrite the metric name")
	assert.Equal(t, "custom-host", sample.Host, "host: is extracted from client tags into the sample host")
	assert.ElementsMatch(t, []string{"client:tag", "job_type:api", "job_name:sync"}, sample.Tags, "mapper tags are appended to client tags after metadata extraction")
	assert.Equal(t, 0.25, sample.SampleRate)
	assert.Equal(t, "container-from-uds", sample.OriginInfo.ContainerIDFromSocket)
	assert.Equal(t, uint32(4242), sample.OriginInfo.LocalData.ProcessID)
	assert.Equal(t, "high", sample.OriginInfo.Cardinality)
	assert.Equal(t, "udp-127.0.0.1:8125", sample.ListenerID)
}

func TestMilestone0TimestampAndHistToDistRoutingBaseline(t *testing.T) {
	deps := fulfillDepsWithConfigOverride(t, map[string]interface{}{
		"dogstatsd_port":                        listeners.RandomPortName,
		"dogstatsd_no_aggregation_pipeline":     true,
		"histogram_copy_to_distribution":        true,
		"histogram_copy_to_distribution_prefix": "dist.",
	})
	s := deps.Server.(*dsdServer)
	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
	var b batcherMock

	s.parsePackets(&b, parser, genTestPackets([]byte("timed.metric:1|g|#env:prod|T1658328888\nhist.metric:2|h|#env:prod")), metrics.MetricSampleBatch{}, nil)

	require.Len(t, b.lateSamples, 1, "timestamped samples route to the no-aggregation/late-sample path")
	assert.Equal(t, "timed.metric", b.lateSamples[0].Name)
	assert.Equal(t, float64(1658328888), b.lateSamples[0].Timestamp)

	require.Len(t, b.samples, 2, "histogram_copy_to_distribution appends an additional distribution sample")
	assert.Equal(t, "hist.metric", b.samples[0].Name)
	assert.Equal(t, metrics.HistogramType, b.samples[0].Mtype)
	assert.Equal(t, "dist.hist.metric", b.samples[1].Name)
	assert.Equal(t, metrics.DistributionType, b.samples[1].Mtype)
	assert.ElementsMatch(t, b.samples[0].Tags, b.samples[1].Tags)
}

func BenchmarkMilestone0ShardKeyGenerator(b *testing.B) {
	samples := make([]metrics.MetricSample, 8192)
	for i := range samples {
		samples[i] = metrics.MetricSample{
			Name: "identity.metric",
			Host: "host-" + strconv.Itoa(i%64),
			Tags: []string{
				"env:prod",
				"service:dogstatsd",
				"instance:" + strconv.Itoa(i),
				"region:us-east-" + strconv.Itoa(i%3),
			},
		}
	}

	for _, shards := range []int{1, 2, 8, 32} {
		b.Run(fmt.Sprintf("shards_%d", shards), func(b *testing.B) {
			generator := newShardGenerator()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = generator.Generate(samples[i%len(samples)], shards)
			}
		})
	}
}

func BenchmarkMilestone0ParsePacketsGuardrails(b *testing.B) {
	benchmarks := []struct {
		name      string
		overrides map[string]interface{}
		packet    []byte
	}{
		{
			name:   "client_tags_low_cardinality",
			packet: buildMilestone0Packet(512, func(i int) string { return "daemon:666|g|#env:prod,service:api" }),
		},
		{
			name:   "client_tags_high_cardinality",
			packet: buildMilestone0Packet(512, func(i int) string { return "daemon:666|g|#env:prod,instance:" + strconv.Itoa(i) }),
		},
		{
			name: "timestamped_no_aggregation",
			overrides: map[string]interface{}{
				"dogstatsd_no_aggregation_pipeline": true,
			},
			packet: buildMilestone0Packet(512, func(i int) string { return "timed.metric:666|g|#env:prod|T1658328888" }),
		},
		{
			name: "histogram_copy_to_distribution",
			overrides: map[string]interface{}{
				"histogram_copy_to_distribution":        true,
				"histogram_copy_to_distribution_prefix": "dist.",
			},
			packet: buildMilestone0Packet(512, func(i int) string { return "hist.metric:666|h|#env:prod,bucket:" + strconv.Itoa(i%16) }),
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			benchmarkMilestone0ParsePackets(b, bm.overrides, bm.packet)
		})
	}
}

func buildMilestone0Packet(count int, line func(int) string) []byte {
	var builder strings.Builder
	for i := 0; i < count; i++ {
		if i > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(line(i))
	}
	return []byte(builder.String())
}

func benchmarkMilestone0ParsePackets(b *testing.B, overrides map[string]interface{}, rawPacket []byte) {
	if overrides == nil {
		overrides = make(map[string]interface{})
	}
	overrides["dogstatsd_port"] = listeners.RandomPortName
	deps, s := fulfillDepsWithInactiveServer(b, overrides)
	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
	s.sharedPacketPool = packets.NewPool(deps.Config, deps.Config.GetInt("dogstatsd_buffer_size"), s.packetsTelemetry)
	s.sharedPacketPoolManager = packets.NewPoolManager[packets.Packet](s.sharedPacketPool)
	samples := make([]metrics.MetricSample, 0, 512)
	packetSet := packets.Packets{nil}
	batcher := countingBatcher{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		packet := s.sharedPacketPoolManager.Get()
		packet.Contents = rawPacket
		packet.Origin = packets.NoOrigin
		packetSet[0] = packet
		samples = s.parsePackets(&batcher, parser, packetSet, samples, nil)
	}
}

type countingBatcher struct {
	samples     int
	lateSamples int
	events      int
	checks      int
}

func (b *countingBatcher) appendSample(metrics.MetricSample)             { b.samples++ }
func (b *countingBatcher) appendLateSample(metrics.MetricSample)         { b.lateSamples++ }
func (b *countingBatcher) appendEvent(*event.Event)                      { b.events++ }
func (b *countingBatcher) appendServiceCheck(*servicecheck.ServiceCheck) { b.checks++ }
func (b *countingBatcher) flush()                                        {}
