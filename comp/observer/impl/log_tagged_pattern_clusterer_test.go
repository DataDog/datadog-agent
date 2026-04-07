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
