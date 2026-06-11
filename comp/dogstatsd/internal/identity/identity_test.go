// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build test

package identity

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestMilestone1SampleIdentityContracts(t *testing.T) {
	origin := taggertypes.OriginInfo{
		ContainerIDFromSocket: "container-a",
		Cardinality:           "high",
		LocalData: origindetection.LocalData{
			ProcessID: 4242,
		},
	}
	sample := metrics.MetricSample{
		Name:       "identity.metric",
		Host:       "host-a",
		Tags:       []string{"service:web", "env:prod", "env:prod"},
		Mtype:      metrics.DistributionType,
		NoIndex:    true,
		Source:     metrics.MetricSourceDogstatsd,
		OriginInfo: origin,
		ListenerID: "udp-127.0.0.1:8125",
	}

	builder := NewBuilder()
	ids := builder.Resolve(sample)

	assert.Equal(t, ClientSeriesIdentity{Name: sample.Name, Tags: sample.Tags}, ids.Client)
	assert.Equal(t, ids.Client, ids.Shard.Client)
	assert.Equal(t, "host-a", ids.Shard.Host)
	assert.ElementsMatch(t, []string{"env:prod", "service:web"}, strings.Fields(ids.Shard.DisplayTags), "shared series display tags are deduplicated for display")
	assert.Equal(t, EffectiveBackendIdentitySeed{
		Name:       sample.Name,
		Host:       sample.Host,
		MetricTags: sample.Tags,
		MetricType: sample.Mtype,
		NoIndex:    sample.NoIndex,
		Source:     sample.Source,
		OriginInfo: sample.OriginInfo,
	}, ids.BackendSeed)
	assert.Equal(t, LineageIdentity{
		ListenerID: sample.ListenerID,
		Source:     sample.Source,
		OriginInfo: sample.OriginInfo,
	}, ids.Lineage)
}

func TestMilestone1IdentityBoundaries(t *testing.T) {
	base := metrics.MetricSample{
		Name:       "identity.metric",
		Host:       "host-a",
		Tags:       []string{"env:prod", "service:web", "env:prod"},
		Mtype:      metrics.GaugeType,
		Source:     metrics.MetricSourceDogstatsd,
		OriginInfo: taggertypes.OriginInfo{ContainerIDFromSocket: "container-a", Cardinality: "low"},
		ListenerID: "udp-127.0.0.1:8125",
	}
	reorderedTags := base
	reorderedTags.Tags = []string{"service:web", "env:prod"}
	changedHost := base
	changedHost.Host = "host-b"
	changedLineage := base
	changedLineage.OriginInfo = taggertypes.OriginInfo{ContainerIDFromSocket: "container-b", Cardinality: "high"}
	changedLineage.ListenerID = "uds-unixgram-7"
	changedLineage.Mtype = metrics.CounterType
	changedTags := base
	changedTags.Tags = []string{"env:prod", "service:api"}

	builder := NewBuilder()
	baseIDs := builder.Resolve(base)
	reorderedIDs := builder.Resolve(reorderedTags)
	changedHostIDs := builder.Resolve(changedHost)
	changedLineageIDs := builder.Resolve(changedLineage)
	changedTagsIDs := builder.Resolve(changedTags)

	assert.Equal(t, baseIDs.Shard.ContextKey, reorderedIDs.Shard.ContextKey, "shared series identity deduplicates and order-normalizes client tags")

	assert.NotEqual(t, baseIDs.Shard.ContextKey, changedHostIDs.Shard.ContextKey, "shared series identity includes host")

	assert.Equal(t, baseIDs.Shard.ContextKey, changedLineageIDs.Shard.ContextKey, "shared series identity intentionally ignores lineage and type")
	assert.NotEqual(t, baseIDs.BackendSeed.MetricType, changedLineageIDs.BackendSeed.MetricType, "backend seed records type for later aggregator identity resolution")
	assert.NotEqual(t, baseIDs.Lineage, changedLineageIDs.Lineage, "lineage identity keeps origin/listener changes visible")

	assert.NotEqual(t, baseIDs.Shard.ContextKey, changedTagsIDs.Shard.ContextKey, "shared series identity includes client tags")
}

func TestCompactIdentityCacheReusesShardContext(t *testing.T) {
	t.Setenv("DD_DOGSTATSD_EXPERIMENTAL_COMPACT_IDENTITIES", "true")
	t.Setenv("DD_DOGSTATSD_EXPERIMENTAL_COMPACT_IDENTITIES_SIZE", "4")

	builder := NewBuilderWithScope(7)
	sample := metrics.MetricSample{
		Name:              "compact.metric",
		Host:              "host-a",
		Tags:              []string{"env:prod", "service:dogstatsd"},
		Mtype:             metrics.CounterType,
		Source:            metrics.MetricSourceDogstatsd,
		DogStatsDTagsetID: 42,
	}

	first := builder.ResolveShardHotPath(sample)
	second := builder.ResolveShardHotPath(sample)

	assert.NotZero(t, first.CompactID)
	assert.Equal(t, first.CompactID, second.CompactID)
	assert.Equal(t, first.Shard.ContextKey, second.Shard.ContextKey)
	assert.NotNil(t, first.CompactState)
	assert.Same(t, first.CompactState, second.CompactState)
	assert.Equal(t, uint64(7)<<48, first.CompactID&(uint64(0xffff)<<48))
}

func TestCompactIdentityCacheRequiresParserTagsetID(t *testing.T) {
	t.Setenv("DD_DOGSTATSD_EXPERIMENTAL_COMPACT_IDENTITIES", "true")

	builder := NewBuilderWithScope(1)
	ctx := builder.ResolveShardHotPath(metrics.MetricSample{
		Name:   "compact.metric",
		Host:   "host-a",
		Tags:   []string{"env:prod"},
		Mtype:  metrics.GaugeType,
		Source: metrics.MetricSourceDogstatsd,
	})

	assert.Zero(t, ctx.CompactID)
	assert.NotZero(t, ctx.Shard.ContextKey)
}

func TestMilestone9BackendSeedMatchesMetricSampleContextKeys(t *testing.T) {
	sample := metrics.MetricSample{
		Name:       "backend.metric",
		Host:       "host-a",
		Tags:       []string{"env:prod", "service:dogstatsd", "env:prod"},
		Mtype:      metrics.CounterType,
		NoIndex:    true,
		Source:     metrics.MetricSourceDogstatsd,
		OriginInfo: taggertypes.OriginInfo{ContainerIDFromSocket: "container-a", Cardinality: "high"},
	}
	seed := BackendSeed(sample)

	sampleKey, sampleTaggerKey, sampleMetricKey := metricContextKeys(&sample)
	seedKey, seedTaggerKey, seedMetricKey := metricContextKeys(&seed)

	assert.Equal(t, sampleKey, seedKey, "backend seed should be consumable through the same MetricSampleContext contract as the original sample")
	assert.Equal(t, sampleTaggerKey, seedTaggerKey)
	assert.Equal(t, sampleMetricKey, seedMetricKey)
	assert.Equal(t, sample.GetName(), seed.GetName())
	assert.Equal(t, sample.GetHost(), seed.GetHost())
	assert.Equal(t, sample.GetMetricType(), seed.GetMetricType())
	assert.Equal(t, sample.IsNoIndex(), seed.IsNoIndex())
	assert.Equal(t, sample.GetSource(), seed.GetSource())
}

func metricContextKeys(ctx metrics.MetricSampleContext) (ckey.ContextKey, ckey.TagsKey, ckey.TagsKey) {
	taggerBuffer := tagset.NewHashingTagsAccumulator()
	metricBuffer := tagset.NewHashingTagsAccumulator()
	ctx.GetTags(taggerBuffer, metricBuffer, nooptagger.NewComponent())
	return ckey.NewKeyGenerator().GenerateWithTags2(ctx.GetName(), ctx.GetHost(), taggerBuffer, metricBuffer)
}

func BenchmarkCompactIdentityShardContext(b *testing.B) {
	sample := metrics.MetricSample{
		Name:              "compact.metric",
		Host:              "host-a",
		Tags:              []string{"env:prod", "service:dogstatsd", "region:us-east-1", "team:agent"},
		Mtype:             metrics.CounterType,
		Source:            metrics.MetricSourceDogstatsd,
		DogStatsDTagsetID: 1,
	}

	b.Run("baseline_shard", func(b *testing.B) {
		builder := NewBuilder()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = builder.Shard(sample)
		}
	})

	b.Run("compact_shard", func(b *testing.B) {
		b.Setenv("DD_DOGSTATSD_EXPERIMENTAL_COMPACT_IDENTITIES", "true")
		b.Setenv("DD_DOGSTATSD_EXPERIMENTAL_COMPACT_IDENTITIES_SIZE", "128")
		builder := NewBuilderWithScope(1)
		_ = builder.ResolveShardHotPath(sample)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = builder.ResolveShardHotPath(sample)
		}
	})
}

func BenchmarkMilestone9BackendSeedContextKey(b *testing.B) {
	sample := metrics.MetricSample{
		Name:   "backend.metric",
		Host:   "host-a",
		Tags:   []string{"env:prod", "service:dogstatsd", "region:us-east-1"},
		Mtype:  metrics.CounterType,
		Source: metrics.MetricSourceDogstatsd,
	}
	seed := BackendSeed(sample)

	b.Run("metric_sample_context", func(b *testing.B) {
		benchmarkContextKey(b, &sample)
	})
	b.Run("backend_seed_context", func(b *testing.B) {
		benchmarkContextKey(b, &seed)
	})
}

func benchmarkContextKey(b *testing.B, ctx metrics.MetricSampleContext) {
	taggerComponent := nooptagger.NewComponent()
	keyGenerator := ckey.NewKeyGenerator()
	taggerBuffer := tagset.NewHashingTagsAccumulator()
	metricBuffer := tagset.NewHashingTagsAccumulator()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.GetTags(taggerBuffer, metricBuffer, taggerComponent)
		_, _, _ = keyGenerator.GenerateWithTags2(ctx.GetName(), ctx.GetHost(), taggerBuffer, metricBuffer)
		taggerBuffer.Reset()
		metricBuffer.Reset()
	}
}

func BenchmarkMilestone1Builder(b *testing.B) {
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
			Mtype:      metrics.DistributionType,
			Source:     metrics.MetricSourceDogstatsd,
			OriginInfo: taggertypes.OriginInfo{ContainerIDFromSocket: "container-" + strconv.Itoa(i%128)},
			ListenerID: "udp-127.0.0.1:8125",
		}
	}

	b.Run("shard", func(b *testing.B) {
		builder := NewBuilder()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = builder.Shard(samples[i%len(samples)])
		}
	})

	b.Run("resolve", func(b *testing.B) {
		builder := NewBuilder()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = builder.Resolve(samples[i%len(samples)])
		}
	})
}

func BenchmarkMilestone2ResolvedContextReuse(b *testing.B) {
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
			Mtype:      metrics.DistributionType,
			Source:     metrics.MetricSourceDogstatsd,
			OriginInfo: taggertypes.OriginInfo{ContainerIDFromSocket: "container-" + strconv.Itoa(i%128)},
			ListenerID: "udp-127.0.0.1:8125",
		}
	}

	b.Run("duplicate_series_work", func(b *testing.B) {
		statsBuilder := NewBuilder()
		shardBuilder := NewBuilder()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sample := samples[i%len(samples)]
			_ = statsBuilder.Shard(sample)
			_ = shardBuilder.Shard(sample)
		}
	})

	b.Run("resolved_hot_path_once", func(b *testing.B) {
		builder := NewBuilder()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = builder.ResolveHotPath(samples[i%len(samples)])
		}
	})
}
