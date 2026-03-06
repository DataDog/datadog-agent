// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/zeebo/xxh3"
)

// realisticContainerTags is a representative set of container tags from the Tagger,
// similar to what AppendHashed processes in the hot path.
var realisticContainerTags = []string{
	"env:production",
	"service:web-frontend",
	"version:1.2.3",
	"kube_namespace:default",
	"kube_deployment:web-frontend",
	"kube_pod_name:web-frontend-abc123",
	"pod_phase:running",
	"kube_node:node-01",
	"region:us-east-1",
	"availability_zone:us-east-1a",
	"container_name:web",
	"image_name:web-frontend",
	"image_tag:latest",
	"cluster_name:prod-cluster",
	"host:node-01.ec2.internal",
}

// filterMetricTagList has a realistic exclude list for a distribution metric.
var filterMetricTagList = map[string]MetricTagList{
	"my.distribution.metric": {
		Tags:   []string{"kube_pod_name", "pod_phase", "image_tag", "version", "availability_zone"},
		Action: "exclude",
	},
}

// BenchmarkAppendHashedStringFilter benchmarks the existing string-based IncludeTag path.
// IncludeTagByNameHash is intentionally left nil so AppendHashed falls through to the
// string path — equivalent to the pre-optimization "before" baseline.
func BenchmarkAppendHashedStringFilter(b *testing.B) {
	matcher := newTagMatcher(filterMetricTagList)
	keepTag, _ := matcher.ShouldStripTags("my.distribution.metric")

	src := tagset.NewHashedTagsFromSlice(realisticContainerTags)

	acc := tagset.NewHashingTagsAccumulator()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		acc.Reset()
		acc.IncludeAll = false
		acc.IncludeTag = keepTag
		// IncludeTagByNameHash intentionally not set → string path
		acc.AppendHashed(src)
	}
}

// BenchmarkAppendHashedNameHashFilter benchmarks the new hash-based IncludeTagByNameHash path
// (uses HashedTags with nameHash — the "after" optimized path).
func BenchmarkAppendHashedNameHashFilter(b *testing.B) {
	matcher := newTagMatcher(filterMetricTagList)
	keepTag, _ := matcher.ShouldStripTags("my.distribution.metric")
	keepHash, _ := matcher.ShouldStripTagsByNameHash("my.distribution.metric")

	// Build source with nameHash populated.
	src := tagset.NewHashedTagsFromSlice(realisticContainerTags)

	acc := tagset.NewHashingTagsAccumulator()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		acc.Reset()
		acc.IncludeAll = false
		acc.IncludeTag = keepTag
		acc.IncludeTagByNameHash = keepHash
		acc.AppendHashed(src)
	}
}

// BenchmarkShouldStripTags benchmarks the ShouldStripTags lookup itself.
func BenchmarkShouldStripTags(b *testing.B) {
	matcher := newTagMatcher(filterMetricTagList)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = matcher.ShouldStripTags("my.distribution.metric")
	}
}

// BenchmarkShouldStripTagsByNameHash benchmarks the ShouldStripTagsByNameHash lookup.
func BenchmarkShouldStripTagsByNameHash(b *testing.B) {
	matcher := newTagMatcher(filterMetricTagList)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = matcher.ShouldStripTagsByNameHash("my.distribution.metric")
	}
}

// BenchmarkKeepTagString benchmarks the per-tag string-based keep decision.
func BenchmarkKeepTagString(b *testing.B) {
	matcher := newTagMatcher(filterMetricTagList)
	keepTag, _ := matcher.ShouldStripTags("my.distribution.metric")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tag := range realisticContainerTags {
			_ = keepTag(tag)
		}
	}
}

// BenchmarkKeepTagHash benchmarks the per-tag hash-based keep decision.
func BenchmarkKeepTagHash(b *testing.B) {
	matcher := newTagMatcher(filterMetricTagList)
	keepHash, _ := matcher.ShouldStripTagsByNameHash("my.distribution.metric")

	// Pre-compute name hashes (as done by NewHashedTagsFromSlice).
	hashes := make([]uint64, len(realisticContainerTags))
	for i, tag := range realisticContainerTags {
		hashes[i] = xxh3.HashString(tagName(tag))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, h := range hashes {
			_ = keepHash(h)
		}
	}
}
