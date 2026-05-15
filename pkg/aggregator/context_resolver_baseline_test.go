// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build test

package aggregator

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	taggertypespkg "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestMilestone0AggregatorClientIdentityBaseline(t *testing.T) {
	matcher := filterlistimpl.NewNoopTagMatcher()
	resolver := newContextResolver(nooptagger.NewComponent(), tags.NewStore(true, "milestone0"), "milestone0")

	base := &metrics.MetricSample{
		Name:       "identity.metric",
		Host:       "host-a",
		Mtype:      metrics.GaugeType,
		Tags:       []string{"env:prod", "service:web", "env:prod"},
		SampleRate: 1,
	}
	reordered := base.Copy()
	reordered.Tags = []string{"service:web", "env:prod"}
	otherHost := base.Copy()
	otherHost.Host = "host-b"
	otherTags := base.Copy()
	otherTags.Tags = []string{"env:prod", "service:api"}

	baseKey := resolver.trackContext(base, 0, matcher)
	assert.Equal(t, baseKey, resolver.trackContext(reordered, 1, matcher), "aggregator identity deduplicates and order-normalizes client tags")
	assert.NotEqual(t, baseKey, resolver.trackContext(otherHost, 2, matcher), "aggregator identity includes host")
	assert.NotEqual(t, baseKey, resolver.trackContext(otherTags, 3, matcher), "aggregator identity includes client tags")

	ctx, found := resolver.get(baseKey)
	require.True(t, found)
	assert.Equal(t, "identity.metric", ctx.Name)
	assert.Equal(t, "host-a", ctx.Host)
	metrics.AssertCompositeTagsEqual(t, ctx.Tags(), tagset.CompositeTagsFromSlice([]string{"env:prod", "service:web"}))
}

func TestMilestone0AggregatorOriginAndFilterBaseline(t *testing.T) {
	configmock.New(t).SetWithoutSource("metric_tag_filterlist_adp_only", false)
	fakeTagger := setupTagger(t)

	t.Run("origin tags participate in effective context identity", func(t *testing.T) {
		resolver := newContextResolver(fakeTagger, tags.NewStore(true, "milestone0-origin"), "milestone0-origin")
		matcher := filterlistimpl.NewNoopTagMatcher()

		s1 := &metrics.MetricSample{
			Name:       "identity.origin",
			Mtype:      metrics.GaugeType,
			Tags:       []string{"version:1"},
			SampleRate: 1,
			OriginInfo: taggertypespkg.OriginInfo{ContainerIDFromSocket: "container_id://container1", Cardinality: "low"},
		}
		s2 := &metrics.MetricSample{
			Name:       "identity.origin",
			Mtype:      metrics.GaugeType,
			Tags:       []string{"version:1"},
			SampleRate: 1,
			OriginInfo: taggertypespkg.OriginInfo{ContainerIDFromSocket: "container_id://container2", Cardinality: "low"},
		}

		key1 := resolver.trackContext(s1, 0, matcher)
		key2 := resolver.trackContext(s2, 0, matcher)

		assert.NotEqual(t, key1, key2, "different tagger-origin tags produce different effective context keys")
		ctx1, found := resolver.get(key1)
		require.True(t, found)
		metrics.AssertCompositeTagsEqual(t, ctx1.Tags(), tagset.CompositeTagsFromSlice([]string{"env:prod", "image_name:image", "pod_name:thing1", "version:1"}))
	})

	t.Run("distribution filterlist can intentionally collapse contexts", func(t *testing.T) {
		resolver := newContextResolver(fakeTagger, tags.NewStore(true, "milestone0-filter"), "milestone0-filter")
		matcher := filterlistimpl.NewTagMatcher(map[string]filterlistimpl.MetricTagList{
			"identity.filtered": {Tags: []string{"env", "pod_name", "client"}, Action: "exclude"},
		}, logmock.New(t))

		dist1 := &metrics.MetricSample{
			Name:       "identity.filtered",
			Mtype:      metrics.DistributionType,
			Tags:       []string{"client:a", "version:1"},
			SampleRate: 1,
			OriginInfo: taggertypespkg.OriginInfo{ContainerIDFromSocket: "container_id://container1", Cardinality: "low"},
		}
		dist2 := &metrics.MetricSample{
			Name:       "identity.filtered",
			Mtype:      metrics.DistributionType,
			Tags:       []string{"client:b", "version:1"},
			SampleRate: 1,
			OriginInfo: taggertypespkg.OriginInfo{ContainerIDFromSocket: "container_id://container2", Cardinality: "low"},
		}
		gauge1 := dist1.Copy()
		gauge1.Mtype = metrics.GaugeType
		gauge2 := dist2.Copy()
		gauge2.Mtype = metrics.GaugeType

		distKey1 := resolver.trackContext(dist1, 0, matcher)
		distKey2 := resolver.trackContext(dist2, 0, matcher)
		assert.Equal(t, distKey1, distKey2, "DistributionType participates in metric_tag_filterlist aggregation")

		ctx, found := resolver.get(distKey1)
		require.True(t, found)
		metrics.AssertCompositeTagsEqual(t, ctx.Tags(), tagset.CompositeTagsFromSlice([]string{"image_name:image", "version:1"}))

		assert.NotEqual(t, resolver.trackContext(gauge1, 0, matcher), resolver.trackContext(gauge2, 0, matcher), "GaugeType does not apply metric_tag_filterlist aggregation")
	})
}

func BenchmarkMilestone0ContextResolverGuardrails(b *testing.B) {
	b.Run("client_tags_high_cardinality", func(b *testing.B) {
		samples := makeMilestone0ContextSamples(8192, func(i int) metrics.MetricSample {
			return metrics.MetricSample{
				Name:       "identity.metric",
				Host:       "host-a",
				Mtype:      metrics.GaugeType,
				Tags:       []string{"env:prod", "service:web", "instance:" + strconv.Itoa(i)},
				SampleRate: 1,
			}
		})
		benchmarkMilestone0ContextResolver(b, nooptagger.NewComponent(), filterlistimpl.NewNoopTagMatcher(), samples)
	})

	b.Run("origin_tagger_tags", func(b *testing.B) {
		origins := []string{"container_id://container1", "container_id://container2", "container_id://container3"}
		samples := makeMilestone0ContextSamples(8192, func(i int) metrics.MetricSample {
			return metrics.MetricSample{
				Name:       "identity.origin",
				Mtype:      metrics.GaugeType,
				Tags:       []string{"version:" + strconv.Itoa(i%128)},
				SampleRate: 1,
				OriginInfo: taggertypespkg.OriginInfo{ContainerIDFromSocket: origins[i%len(origins)], Cardinality: "low"},
			}
		})
		benchmarkMilestone0ContextResolver(b, setupTagger(b), filterlistimpl.NewNoopTagMatcher(), samples)
	})

	b.Run("distribution_tag_filter", func(b *testing.B) {
		configmock.New(b).SetWithoutSource("metric_tag_filterlist_adp_only", false)
		matcher := filterlistimpl.NewTagMatcher(map[string]filterlistimpl.MetricTagList{
			"identity.filtered": {Tags: []string{"env", "pod_name", "client"}, Action: "exclude"},
		}, logmock.New(b))
		samples := makeMilestone0ContextSamples(8192, func(i int) metrics.MetricSample {
			return metrics.MetricSample{
				Name:       "identity.filtered",
				Mtype:      metrics.DistributionType,
				Tags:       []string{"client:" + strconv.Itoa(i), "version:1"},
				SampleRate: 1,
				OriginInfo: taggertypespkg.OriginInfo{ContainerIDFromSocket: "container_id://container" + strconv.Itoa((i%3)+1), Cardinality: "low"},
			}
		})
		benchmarkMilestone0ContextResolver(b, setupTagger(b), matcher, samples)
	})
}

func makeMilestone0ContextSamples(count int, sample func(int) metrics.MetricSample) []metrics.MetricSample {
	samples := make([]metrics.MetricSample, count)
	for i := range samples {
		samples[i] = sample(i)
	}
	return samples
}

func benchmarkMilestone0ContextResolver(b *testing.B, taggerComponent tagger.Component, matcher filterlist.TagMatcher, samples []metrics.MetricSample) {
	resolver := newContextResolver(taggerComponent, tags.NewStore(true, "milestone0-bench"), "milestone0-bench")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolver.trackContext(&samples[i%len(samples)], 0, matcher)
	}
}
