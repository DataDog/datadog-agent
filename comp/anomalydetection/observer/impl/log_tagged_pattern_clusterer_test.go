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

// --- TagGroupByKeyRegistry tests ---

func TestTagGroupByKeyRegistry_RegisterReturnsSameHashForSameCombo(t *testing.T) {
	r := NewTagGroupByKeyRegistry()
	c := TagGroupByKey{Source: "dd-agent", Service: "api", Env: "prod", Host: "host-1"}
	h1 := r.Register(c)
	h2 := r.Register(c)
	assert.Equal(t, h1, h2)
}

func TestTagGroupByKeyRegistry_RegisterReturnsDifferentHashForDifferentCombo(t *testing.T) {
	r := NewTagGroupByKeyRegistry()
	h1 := r.Register(TagGroupByKey{Service: "api", Env: "prod"})
	h2 := r.Register(TagGroupByKey{Service: "api", Env: "staging"})
	assert.NotEqual(t, h1, h2)
}

func TestTagGroupByKeyRegistry_LookupReturnsRegisteredCombo(t *testing.T) {
	r := NewTagGroupByKeyRegistry()
	c := TagGroupByKey{Source: "dd-agent", Service: "worker", Env: "prod", Host: "host-2"}
	hash := r.Register(c)
	got, ok := r.Lookup(hash)
	require.True(t, ok)
	assert.Equal(t, c, got)
}

func TestTagGroupByKeyRegistry_LookupReturnsFalseForUnknownHash(t *testing.T) {
	r := NewTagGroupByKeyRegistry()
	_, ok := r.Lookup(0xdeadbeef)
	assert.False(t, ok)
}

func TestTagGroupByKeyRegistry_EmptyCombosAreDistinct(t *testing.T) {
	r := NewTagGroupByKeyRegistry()
	// Two empty groups share the same hash (all fields absent).
	h1 := r.Register(TagGroupByKey{})
	h2 := r.Register(TagGroupByKey{})
	assert.Equal(t, h1, h2)
	got, ok := r.Lookup(h1)
	require.True(t, ok)
	assert.Equal(t, TagGroupByKey{}, got)
}

// --- TagGroupByKey.AsMap tests ---

func TestTagGroupByKey_AsMapOmitsEmptyFields(t *testing.T) {
	c := TagGroupByKey{Service: "api", Env: "prod"}
	m := c.AsMap()
	assert.Equal(t, map[string]string{"service": "api", "env": "prod"}, m)
	assert.NotContains(t, m, "source")
	assert.NotContains(t, m, "host")
}

func TestTagGroupByKey_AsMapReturnsNilWhenAllEmpty(t *testing.T) {
	c := TagGroupByKey{}
	assert.Nil(t, c.AsMap())
}

func TestTagGroupByKey_AsMapAllFields(t *testing.T) {
	c := TagGroupByKey{Source: "s", Service: "svc", Env: "e", Host: "h"}
	assert.Equal(t, map[string]string{
		"source":  "s",
		"service": "svc",
		"env":     "e",
		"host":    "h",
	}, c.AsMap())
}

func TestTagsForPatternGrouping_AppendsHostFromHostnameWhenMissing(t *testing.T) {
	tags := []string{"service:api", "env:prod"}
	got := tagsForPatternGrouping(tags, "host-a")
	assert.Equal(t, []string{"service:api", "env:prod", "host:host-a"}, got)
	assert.Equal(t, TagGroupByKey{Service: "api", Env: "prod", Host: "host-a"}, extractTagGroupByKey(got))
}

func TestTagsForPatternGrouping_ExplicitHostTagWins(t *testing.T) {
	tags := []string{"service:api", "host:from-tag"}
	got := tagsForPatternGrouping(tags, "from-gethostname")
	assert.Equal(t, tags, got)
	assert.Equal(t, TagGroupByKey{Service: "api", Host: "from-tag"}, extractTagGroupByKey(got))
}

func TestTagsForPatternGrouping_EmptyHostnameUnchanged(t *testing.T) {
	tags := []string{"service:api"}
	got := tagsForPatternGrouping(tags, "")
	assert.Equal(t, tags, got)
}

// --- TaggedPatternClusterer tests ---

func newTestTaggedClusterer() (*TaggedPatternClusterer, *TagGroupByKeyRegistry) {
	reg := NewTagGroupByKeyRegistry()
	tc := NewTaggedPatternClusterer(reg)
	return tc, reg
}

func TestTaggedPatternClusterer_RoutesByTagGroup(t *testing.T) {
	tc, _ := newTestTaggedClusterer()

	tagsA := []string{"service:api", "env:prod"}
	tagsB := []string{"service:worker", "env:prod"}

	hashA, _, ok := tc.Process(tagsA, "error connecting to db", 1000)
	require.True(t, ok)
	hashB, _, ok := tc.Process(tagsB, "error connecting to db", 1000)
	require.True(t, ok)

	assert.NotEqual(t, hashA, hashB, "different tag groups must yield different hashes")
	assert.Equal(t, 2, tc.NumSubClusterers())
}

func TestTaggedPatternClusterer_SameTagGroupSharesSubClusterer(t *testing.T) {
	tc, _ := newTestTaggedClusterer()

	tags := []string{"service:api", "env:prod"}
	hash1, _, ok := tc.Process(tags, "error connecting to db", 1000)
	require.True(t, ok)
	hash2, _, ok := tc.Process(tags, "error reading from db", 1001)
	require.True(t, ok)

	assert.Equal(t, hash1, hash2)
	assert.Equal(t, 1, tc.NumSubClusterers())
}

func TestTaggedPatternClusterer_GetClusterReturnsCorrectCluster(t *testing.T) {
	tc, _ := newTestTaggedClusterer()

	tags := []string{"service:api"}
	groupHash, cluster, ok := tc.Process(tags, "timeout after 30s", 1000)
	require.True(t, ok)

	got, err := tc.GetCluster(groupHash, cluster.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, cluster.ID, got.ID)
}

func TestTaggedPatternClusterer_GetClusterUnknownGroupReturnsError(t *testing.T) {
	tc, _ := newTestTaggedClusterer()
	_, err := tc.GetCluster(0xdeadbeefcafe, 0)
	assert.Error(t, err)
}

func TestTaggedPatternClusterer_ResetDropsSubClusterers(t *testing.T) {
	tc, reg := newTestTaggedClusterer()

	tags := []string{"service:api"}
	groupHash, _, ok := tc.Process(tags, "error connecting to db", 1000)
	require.True(t, ok)
	require.Equal(t, 1, tc.NumSubClusterers())

	tc.Reset()
	assert.Equal(t, 0, tc.NumSubClusterers())

	// Registry must still resolve the hash after reset.
	group, found := reg.Lookup(groupHash)
	require.True(t, found)
	assert.Equal(t, "api", group.Service)
}

func TestTaggedPatternClusterer_GlobalClusterHashIsStable(t *testing.T) {
	h1 := globalClusterHash(42, 7)
	h2 := globalClusterHash(42, 7)
	assert.Equal(t, h1, h2)

	h3 := globalClusterHash(42, 8)
	assert.NotEqual(t, h1, h3)

	h4 := globalClusterHash(43, 7)
	assert.NotEqual(t, h1, h4)
}

func TestExtractTagGroupByKey_PopulatesKnownKeys(t *testing.T) {
	tags := []string{"source:dd-agent", "service:api", "env:prod", "host:h1", "version:1.2"}
	g := extractTagGroupByKey(tags)
	assert.Equal(t, TagGroupByKey{Source: "dd-agent", Service: "api", Env: "prod", Host: "h1"}, g)
}

func TestExtractTagGroupByKey_MissingKeysAreEmpty(t *testing.T) {
	g := extractTagGroupByKey([]string{"service:api"})
	assert.Equal(t, TagGroupByKey{Service: "api"}, g)
}

func TestExtractTagGroupByKey_MalformedTagsIgnored(t *testing.T) {
	g := extractTagGroupByKey([]string{"nocolon", "service:api"})
	assert.Equal(t, TagGroupByKey{Service: "api"}, g)
}

func TestTaggedPatternClusterer_MaxClustersPerGroupPropagated(t *testing.T) {
	tc := NewTaggedPatternClusterer(NewTagGroupByKeyRegistry())
	tc.MaxClustersPerGroup = 2
	tags := []string{"source:s", "service:svc", "env:prod", "host:h"}

	// 3 distinct (length-varying) shapes → first eviction surfaces via DrainLRUEvictions.
	tc.Process(tags, "alpha", 1000)
	tc.Process(tags, "beta gamma", 1001)
	require.Empty(t, tc.DrainLRUEvictions(), "no eviction at cap")

	_, _, ok := tc.Process(tags, "x y z", 1002)
	require.True(t, ok)
	evicted := tc.DrainLRUEvictions()
	require.Len(t, evicted, 1, "one cluster evicted by per-group cap")
	require.Equal(t, int64(0), evicted[0].ClusterID)
}

func TestTaggedPatternClusterer_MaxTagGroupsEvictsLRUGroup(t *testing.T) {
	tc := NewTaggedPatternClusterer(NewTagGroupByKeyRegistry())
	tc.MaxTagGroups = 2

	tagsA := []string{"service:a"}
	tagsB := []string{"service:b"}
	tagsC := []string{"service:c"}

	// Touch group A first (oldest), then B; then route to C and expect A evicted.
	hashA, _, _ := tc.Process(tagsA, "alpha one", 1000)
	hashA2, _, _ := tc.Process(tagsA, "two distinct shape pattern", 1001) // second cluster within A
	require.Equal(t, hashA, hashA2)
	require.Equal(t, 1, tc.NumSubClusterers())

	hashB, _, _ := tc.Process(tagsB, "beta", 1100)
	require.NotEqual(t, hashA, hashB)
	require.Equal(t, 2, tc.NumSubClusterers())
	require.Empty(t, tc.DrainLRUEvictions(), "at cap, no eviction yet")

	// Now touch A again to make B the LRU group.
	tc.Process(tagsA, "alpha one", 1200)
	require.Empty(t, tc.DrainLRUEvictions())

	// Adding C must evict B (least-recently touched at unixSec=1100).
	hashC, _, _ := tc.Process(tagsC, "gamma", 1300)
	require.NotEqual(t, hashB, hashC)
	require.Equal(t, 2, tc.NumSubClusterers(), "cap holds at MaxTagGroups")
	evicted := tc.DrainLRUEvictions()
	require.NotEmpty(t, evicted, "the LRU group's clusters are surfaced as evictions")
	for _, ev := range evicted {
		require.Equal(t, hashB, ev.GroupHash, "all evictions tagged with the evicted group's hash")
	}
}

// TestTaggedPatternClusterer_EmptyMessageFromNewGroupDoesNotEvict regresses
// the bug where an empty (or whitespace-only) first message from a brand-new
// tag group, while at MaxTagGroups capacity, would trigger eviction of an
// active group BEFORE the inner PatternClusterer.Process rejected the empty
// message. The active group's clusters were lost, an empty sub-clusterer
// stayed wedged into tc.subClusterers counting against the cap, and a stream
// of empty logs from new containers could evict real pattern state and
// suppress later anomalies.
//
// The fix is two-phase create in Process: build a transient sub-clusterer
// without committing it; only insert + evict-LRU after sub.Process returns
// ok. This test confirms an empty message from a new group at-cap is a
// no-op for both the existing groups and the eviction queue.
func TestTaggedPatternClusterer_EmptyMessageFromNewGroupDoesNotEvict(t *testing.T) {
	tc := NewTaggedPatternClusterer(NewTagGroupByKeyRegistry())
	tc.MaxTagGroups = 2

	tagsA := []string{"service:a"}
	tagsB := []string{"service:b"}
	tagsC := []string{"service:c"} // would be the new “third” group at-cap

	hashA, _, okA := tc.Process(tagsA, "alpha", 1000)
	require.True(t, okA)
	hashB, _, okB := tc.Process(tagsB, "beta", 1100)
	require.True(t, okB)
	require.Equal(t, 2, tc.NumSubClusterers(), "both groups resident at cap")
	require.Empty(t, tc.DrainLRUEvictions())

	for _, msg := range []string{"", "   ", "\t\n"} {
		hashC, _, okC := tc.Process(tagsC, msg, 1200)
		require.False(t, okC, "empty/whitespace messages must be rejected by inner Process")
		require.Equal(t, uint64(0), hashC, "rejected calls return zero hash")
		require.Equal(t, 2, tc.NumSubClusterers(),
			"empty msg from new group at-cap must NOT create a sub-clusterer (would steal a slot from A or B)")
		require.Empty(t, tc.DrainLRUEvictions(),
			"empty msg from new group at-cap must NOT evict an existing group")
	}

	// Both original groups must still be reachable after the empty stream.
	gotA, _, _ := tc.Process(tagsA, "alpha", 1300)
	require.Equal(t, hashA, gotA, "group A must survive a stream of empty new-group messages")
	gotB, _, _ := tc.Process(tagsB, "beta", 1301)
	require.Equal(t, hashB, gotB, "group B must survive a stream of empty new-group messages")
}

func TestTaggedPatternClusterer_DrainLRUEvictionsIsOneShot(t *testing.T) {
	tc := NewTaggedPatternClusterer(NewTagGroupByKeyRegistry())
	tc.MaxClustersPerGroup = 1
	tags := []string{"service:s"}
	tc.Process(tags, "alpha", 1000)
	tc.Process(tags, "beta gamma", 1001) // triggers eviction of cluster id=0
	first := tc.DrainLRUEvictions()
	require.Len(t, first, 1)
	second := tc.DrainLRUEvictions()
	require.Empty(t, second, "drain returns nothing on the second call")
}

func TestTaggedPatternClusterer_ResetClearsLRUState(t *testing.T) {
	tc := NewTaggedPatternClusterer(NewTagGroupByKeyRegistry())
	tc.MaxClustersPerGroup = 1
	tc.MaxTagGroups = 1
	tc.Process([]string{"service:a"}, "alpha", 1000)
	tc.Process([]string{"service:b"}, "beta", 1001) // evicts service:a's group
	tc.Reset()
	require.Empty(t, tc.DrainLRUEvictions(), "Reset must clear pending evictions")
	require.Equal(t, 0, tc.NumSubClusterers())
}
