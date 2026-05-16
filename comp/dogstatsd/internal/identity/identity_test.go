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
	assert.Equal(t, ids.Client, ids.DebugView.Client)
	assert.Equal(t, ids.Client, ids.Shard.Client)
	assert.Equal(t, "host-a", ids.Shard.Host)
	assert.ElementsMatch(t, []string{"env:prod", "service:web"}, strings.Fields(ids.DebugView.DisplayTags), "debug-view display tags are deduplicated for display")
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

	assert.Equal(t, baseIDs.DebugView.Key, reorderedIDs.DebugView.Key, "debug view key deduplicates and order-normalizes client tags")
	assert.Equal(t, baseIDs.Shard.ContextKey, reorderedIDs.Shard.ContextKey, "shard identity deduplicates and order-normalizes client tags")

	assert.Equal(t, baseIDs.DebugView.Key, changedHostIDs.DebugView.Key, "debug view compatibility key intentionally ignores host")
	assert.NotEqual(t, baseIDs.Shard.ContextKey, changedHostIDs.Shard.ContextKey, "shard identity includes host")

	assert.Equal(t, baseIDs.DebugView.Key, changedLineageIDs.DebugView.Key, "debug view compatibility key intentionally ignores lineage and type")
	assert.Equal(t, baseIDs.Shard.ContextKey, changedLineageIDs.Shard.ContextKey, "shard identity intentionally ignores lineage and type")
	assert.NotEqual(t, baseIDs.BackendSeed.MetricType, changedLineageIDs.BackendSeed.MetricType, "backend seed records type for later aggregator identity resolution")
	assert.NotEqual(t, baseIDs.Lineage, changedLineageIDs.Lineage, "lineage identity keeps origin/listener changes visible")

	assert.NotEqual(t, baseIDs.DebugView.Key, changedTagsIDs.DebugView.Key, "debug view key includes client tags")
	assert.NotEqual(t, baseIDs.Shard.ContextKey, changedTagsIDs.Shard.ContextKey, "shard identity includes client tags")
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

	b.Run("debug", func(b *testing.B) {
		builder := NewBuilder()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = builder.DebugView(samples[i%len(samples)])
		}
	})

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

	b.Run("separate_debug_and_shard", func(b *testing.B) {
		debugBuilder := NewBuilder()
		shardBuilder := NewBuilder()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sample := samples[i%len(samples)]
			_ = debugBuilder.DebugView(sample)
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
