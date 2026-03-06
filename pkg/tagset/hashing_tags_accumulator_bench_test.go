// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"testing"

	"github.com/zeebo/xxh3"
)

// containerTags simulates a realistic set of container tags from the Tagger.
var containerTags = []string{
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

// filterTagNames are the names of tags the filter will strip (exclude action).
var filterTagNames = []string{
	"kube_pod_name",
	"pod_phase",
	"image_tag",
	"version",
	"availability_zone",
}

// buildHashedSrc builds a HashedTags (with nameHash) for benchmarks.
func buildHashedSrc() HashedTags {
	return NewHashedTagsFromSlice(containerTags)
}

// buildHashedSrcNoNameHash builds a HashedTags without nameHash to simulate the fallback path.
func buildHashedSrcNoNameHash() HashedTags {
	return HashedTags{hashedTags: newHashedTagsFromSlice(containerTags)}
}

// buildStringFilter returns an IncludeTag closure that hashes the tag name and searches.
func buildStringFilter() func(string) bool {
	hashes := make([]uint64, len(filterTagNames))
	for i, name := range filterTagNames {
		hashes[i] = xxh3.HashString(name)
	}
	// sort for binary search
	sortUint64(hashes)
	return func(tag string) bool {
		pos := indexByte(tag, ':')
		if pos < 0 {
			pos = len(tag)
		}
		h := xxh3.HashString(tag[:pos])
		_, found := binarySearch(hashes, h)
		return !found // keep if NOT in the exclude list
	}
}

// buildHashFilter returns an IncludeTagByNameHash closure.
func buildHashFilter() func(uint64) bool {
	hashes := make([]uint64, len(filterTagNames))
	for i, name := range filterTagNames {
		hashes[i] = xxh3.HashString(name)
	}
	sortUint64(hashes)
	return func(h uint64) bool {
		_, found := binarySearch(hashes, h)
		return !found
	}
}

// sortUint64 sorts a slice of uint64 in ascending order (insertion sort for small slices).
func sortUint64(s []uint64) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// indexByte returns the index of the first occurrence of b in s, or -1.
func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// binarySearch returns (index, found) for v in a sorted slice.
func binarySearch(s []uint64, v uint64) (int, bool) {
	lo, hi := 0, len(s)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if s[mid] < v {
			lo = mid + 1
		} else if s[mid] > v {
			hi = mid
		} else {
			return mid, true
		}
	}
	return lo, false
}

// BenchmarkAppendHashedFiltered/string-path uses the existing IncludeTag (string-based) path.
// BenchmarkAppendHashedFiltered/hash-path uses the new IncludeTagByNameHash path.
func BenchmarkAppendHashedFiltered(b *testing.B) {
	b.Run("string-path", func(b *testing.B) {
		src := buildHashedSrcNoNameHash()
		filter := buildStringFilter()
		acc := NewHashingTagsAccumulator()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			acc.Reset()
			acc.IncludeAll = false
			acc.IncludeTag = filter
			acc.AppendHashed(src)
		}
	})

	b.Run("hash-path", func(b *testing.B) {
		src := buildHashedSrc()
		filter := buildHashFilter()
		acc := NewHashingTagsAccumulator()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			acc.Reset()
			acc.IncludeAll = false
			acc.IncludeTagByNameHash = filter
			acc.AppendHashed(src)
		}
	})
}
