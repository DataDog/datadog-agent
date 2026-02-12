// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractCommonTags_Overlapping(t *testing.T) {
	members := []seriesCompact{
		{Tags: []string{"host:web1", "env:prod", "region:us"}},
		{Tags: []string{"host:web2", "env:prod", "region:us"}},
		{Tags: []string{"host:web3", "env:prod", "region:us"}},
	}
	common, residuals := extractCommonTags(members)
	assert.Equal(t, map[string]string{"env": "prod", "region": "us"}, common)
	assert.Equal(t, []string{"host:web1"}, residuals[0])
	assert.Equal(t, []string{"host:web2"}, residuals[1])
	assert.Equal(t, []string{"host:web3"}, residuals[2])
}

func TestExtractCommonTags_Disjoint(t *testing.T) {
	members := []seriesCompact{
		{Tags: []string{"host:web1", "env:prod"}},
		{Tags: []string{"host:web2", "region:eu"}},
	}
	common, residuals := extractCommonTags(members)
	assert.Empty(t, common)
	assert.Len(t, residuals[0], 2)
	assert.Len(t, residuals[1], 2)
}

func TestExtractCommonTags_Empty(t *testing.T) {
	common, residuals := extractCommonTags(nil)
	assert.Empty(t, common)
	assert.Nil(t, residuals)
}

func TestExtractCommonTags_SingleMember(t *testing.T) {
	members := []seriesCompact{
		{Tags: []string{"host:web1", "env:prod"}},
	}
	common, residuals := extractCommonTags(members)
	// All tags are "common" when there's only one member
	assert.Equal(t, map[string]string{"host": "web1", "env": "prod"}, common)
	assert.Empty(t, residuals[0])
}

func TestBuildTrie_Basic(t *testing.T) {
	members := []string{"cgroup.v2.cpu.stat.user_usec", "cgroup.v2.cpu.stat.system_usec"}
	universe := []string{"cgroup.v2.cpu.stat.user_usec", "cgroup.v2.cpu.stat.system_usec", "cgroup.v2.memory.current"}
	root := buildTrie(members, universe)

	// Root should have 1 child: "cgroup"
	assert.Len(t, root.children, 1)
	cgroupNode := root.children["cgroup"]
	require.NotNil(t, cgroupNode)
	assert.Equal(t, 2, cgroupNode.memberCount)
	assert.Equal(t, 3, cgroupNode.universeCount)
}

func TestCompressFromTrie_WildcardAtHighThreshold(t *testing.T) {
	// All 3 universe names are members -> precision = 1.0 at cgroup.v2 level
	members := []string{"cgroup.v2.cpu.stat.user_usec", "cgroup.v2.cpu.stat.system_usec", "cgroup.v2.memory.current"}
	universe := members
	root := buildTrie(members, universe)
	patterns := compressFromTrie(root, 0.75)

	// Should produce a single wildcard pattern
	require.Len(t, patterns, 1)
	assert.Equal(t, "cgroup.*", patterns[0].Pattern)
	assert.Equal(t, 3, patterns[0].Matched)
	assert.Equal(t, 3, patterns[0].Universe)
	assert.Equal(t, 1.0, patterns[0].Precision)
}

func TestCompressFromTrie_SplitAtLowPrecision(t *testing.T) {
	// Only 2 of 4 are members -> precision = 0.5, below threshold 0.75
	members := []string{"cgroup.v2.cpu.stat.user_usec", "cgroup.v2.cpu.stat.system_usec"}
	universe := []string{
		"cgroup.v2.cpu.stat.user_usec",
		"cgroup.v2.cpu.stat.system_usec",
		"cgroup.v2.memory.current",
		"cgroup.v2.io.rbytes",
	}
	root := buildTrie(members, universe)
	patterns := compressFromTrie(root, 0.75)

	// Should recurse deeper; cpu.stat has 2/2 = 1.0 precision
	found := false
	for _, p := range patterns {
		if p.Pattern == "cgroup.v2.cpu.stat.*" || p.Pattern == "cgroup.v2.cpu.*" {
			found = true
			assert.Equal(t, 2, p.Matched)
		}
	}
	assert.True(t, found, "expected a cpu wildcard pattern, got: %v", patterns)
}

func TestCompressGroup_EndToEnd(t *testing.T) {
	members := []seriesCompact{
		{Namespace: "parquet", Name: "cgroup.v2.cpu.stat.user_usec:avg", Tags: []string{"container_id:abc123"}},
		{Namespace: "parquet", Name: "cgroup.v2.cpu.stat.system_usec:avg", Tags: []string{"container_id:abc123"}},
		{Namespace: "parquet", Name: "cgroup.v2.memory.current:avg", Tags: []string{"container_id:abc123"}},
	}
	universe := []seriesCompact{
		{Namespace: "parquet", Name: "cgroup.v2.cpu.stat.user_usec:avg", Tags: []string{"container_id:abc123"}},
		{Namespace: "parquet", Name: "cgroup.v2.cpu.stat.system_usec:avg", Tags: []string{"container_id:abc123"}},
		{Namespace: "parquet", Name: "cgroup.v2.memory.current:avg", Tags: []string{"container_id:abc123"}},
		{Namespace: "parquet", Name: "cgroup.v2.io.rbytes:avg", Tags: []string{"container_id:abc123"}},
		{Namespace: "parquet", Name: "system.net.bytes_rcvd:avg", Tags: []string{"container_id:abc123"}},
	}

	result := CompressGroup("time_cluster", "group-0", "CPU+Memory Spike", members, universe, 0.75)

	assert.Equal(t, "time_cluster", result.CorrelatorName)
	assert.Equal(t, "group-0", result.GroupID)
	assert.Equal(t, "CPU+Memory Spike", result.Title)
	assert.Equal(t, map[string]string{"container_id": "abc123"}, result.CommonTags)
	assert.Equal(t, 3, result.SeriesCount)
	assert.Len(t, result.MemberSources, 3)
	assert.NotEmpty(t, result.Patterns)
}

func TestCompressGroup_SingleMember(t *testing.T) {
	members := []seriesCompact{
		{Namespace: "parquet", Name: "system.cpu.user:avg", Tags: nil},
	}
	universe := []seriesCompact{
		{Namespace: "parquet", Name: "system.cpu.user:avg", Tags: nil},
		{Namespace: "parquet", Name: "system.cpu.system:avg", Tags: nil},
	}

	result := CompressGroup("lead_lag", "g1", "", members, universe, 0.75)

	assert.Equal(t, 1, result.SeriesCount)
	assert.NotEmpty(t, result.Patterns)
}

func TestCompressGroup_NoCommonTags(t *testing.T) {
	members := []seriesCompact{
		{Namespace: "parquet", Name: "a.b.c:avg", Tags: []string{"host:h1"}},
		{Namespace: "parquet", Name: "a.b.d:avg", Tags: []string{"host:h2"}},
	}
	universe := members

	result := CompressGroup("surprise", "g2", "test", members, universe, 0.75)

	assert.Empty(t, result.CommonTags)
	assert.Equal(t, 2, result.SeriesCount)
}

func TestCompressGroup_AllIdenticalNames(t *testing.T) {
	members := []seriesCompact{
		{Namespace: "parquet", Name: "cpu.user:avg", Tags: []string{"host:h1"}},
		{Namespace: "parquet", Name: "cpu.user:avg", Tags: []string{"host:h2"}},
	}
	universe := members

	result := CompressGroup("test", "g3", "identical", members, universe, 0.75)

	// The unique metric name set has only 1 entry
	assert.NotEmpty(t, result.Patterns)
	assert.Equal(t, 2, result.SeriesCount)
}

func TestCompressGroup_Empty(t *testing.T) {
	result := CompressGroup("test", "g0", "empty", nil, nil, 0.75)
	assert.Equal(t, 0, result.SeriesCount)
	assert.Empty(t, result.Patterns)
	assert.Empty(t, result.MemberSources)
}

func TestStripAggSuffix(t *testing.T) {
	assert.Equal(t, "cpu.user", stripAggSuffix("cpu.user:avg"))
	assert.Equal(t, "cpu.user", stripAggSuffix("cpu.user:count"))
	assert.Equal(t, "cpu.user", stripAggSuffix("cpu.user:sum"))
	assert.Equal(t, "cpu.user", stripAggSuffix("cpu.user:min"))
	assert.Equal(t, "cpu.user", stripAggSuffix("cpu.user:max"))
	assert.Equal(t, "cpu.user", stripAggSuffix("cpu.user"))
	assert.Equal(t, "cpu.user:p99", stripAggSuffix("cpu.user:p99"))
}

func TestSplitTag(t *testing.T) {
	k, v := splitTag("host:web1")
	assert.Equal(t, "host", k)
	assert.Equal(t, "web1", v)

	k, v = splitTag("novalue")
	assert.Equal(t, "novalue", k)
	assert.Equal(t, "", v)

	k, v = splitTag("host:web1:extra")
	assert.Equal(t, "host", k)
	assert.Equal(t, "web1:extra", v)
}
