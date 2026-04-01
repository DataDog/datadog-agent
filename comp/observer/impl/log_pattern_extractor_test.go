// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
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

	hashA, _, ok := tc.Process(tagsA, "error connecting to db")
	require.True(t, ok)
	hashB, _, ok := tc.Process(tagsB, "error connecting to db")
	require.True(t, ok)

	assert.NotEqual(t, hashA, hashB, "different tag groups must yield different hashes")
	assert.Equal(t, 2, tc.NumSubClusterers())
}

func TestTaggedPatternClusterer_SameTagGroupSharesSubClusterer(t *testing.T) {
	tc, _ := newTestTaggedClusterer()

	tags := []string{"service:api", "env:prod"}
	hash1, _, ok := tc.Process(tags, "error connecting to db")
	require.True(t, ok)
	hash2, _, ok := tc.Process(tags, "error reading from db")
	require.True(t, ok)

	assert.Equal(t, hash1, hash2)
	assert.Equal(t, 1, tc.NumSubClusterers())
}

func TestTaggedPatternClusterer_GetClusterReturnsCorrectCluster(t *testing.T) {
	tc, _ := newTestTaggedClusterer()

	tags := []string{"service:api"}
	groupHash, cluster, ok := tc.Process(tags, "timeout after 30s")
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
	groupHash, _, ok := tc.Process(tags, "error connecting to db")
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

// --- LogPatternExtractor tests ---

func TestLogPatternExtractor_GetContextByKeyUsesOutputContextKey(t *testing.T) {
	e := NewLogPatternExtractor()
	e.MinPatternsBeforeEmit = 1

	log := &mockLogView{
		content: []byte("GET /users/123 returned 500"),
		status:  "warn",
		tags:    []string{"service:web", "env:prod"},
	}

	res := e.ProcessLog(log)
	require.Len(t, res.Metrics, 1)
	require.NotEmpty(t, res.Metrics[0].ContextKey)

	ctx, ok := e.GetContextByKey(res.Metrics[0].ContextKey)
	require.True(t, ok)
	assert.Equal(t, "log_pattern_extractor", ctx.Source)
	assert.Equal(t, "GET /users/123 returned 500", ctx.Example)
	assert.NotEmpty(t, ctx.Pattern)
	assert.Equal(t, map[string]string{"service": "web", "env": "prod"}, ctx.SplitTags)
}

func TestLogPatternExtractor_DifferentTagGroupsProduceDifferentMetricNames(t *testing.T) {
	e := NewLogPatternExtractor()
	e.MinPatternsBeforeEmit = 1

	// 1 pattern per service (same pattern strings but different IDs)
	logA := &mockLogView{
		content: []byte("GET /users/123 returned 500"),
		status:  "warn",
		tags:    []string{"service:api"},
	}
	logB := &mockLogView{
		content: []byte("GET /users/456 returned 500"),
		status:  "warn",
		tags:    []string{"service:worker"},
	}
	logC := &mockLogView{
		content: []byte("GET /users/124 returned 500"),
		status:  "warn",
		tags:    []string{"service:api"},
	}
	logD := &mockLogView{
		content: []byte("GET /users/457 returned 500"),
		status:  "warn",
		tags:    []string{"service:worker"},
	}

	resA := e.ProcessLog(logA)
	resB := e.ProcessLog(logB)
	e.ProcessLog(logC)
	e.ProcessLog(logD)
	require.Len(t, resA.Metrics, 1)
	require.Len(t, resB.Metrics, 1)
	// Different tag groups → different sub-clusterers → different globalClusterHash → different names.
	require.NotEqual(t, resA.Metrics[0].Name, resB.Metrics[0].Name)
	require.NotEqual(t, resA.Metrics[0].ContextKey, resB.Metrics[0].ContextKey)

	ctxA, ok := e.GetContextByKey(resA.Metrics[0].ContextKey)
	require.True(t, ok)
	ctxB, ok := e.GetContextByKey(resB.Metrics[0].ContextKey)
	require.True(t, ok)

	assert.Equal(t, "GET /users/123 returned 500", ctxA.Example)
	assert.Equal(t, "GET /users/456 returned 500", ctxB.Example)
	assert.Equal(t, ctxA.Pattern, ctxB.Pattern)
	assert.Equal(t, map[string]string{"service": "api"}, ctxA.SplitTags)
	assert.Equal(t, map[string]string{"service": "worker"}, ctxB.SplitTags)
}

func TestLogPatternExtractor_ResetClearsContext(t *testing.T) {
	e := NewLogPatternExtractor()
	e.MinPatternsBeforeEmit = 1

	log := &mockLogView{
		content: []byte("GET /users/123 returned 500"),
		status:  "warn",
		tags:    []string{"service:web"},
	}

	res := e.ProcessLog(log)
	require.Len(t, res.Metrics, 1)

	_, ok := e.GetContextByKey(res.Metrics[0].ContextKey)
	require.True(t, ok)

	e.Reset()

	_, ok = e.GetContextByKey(res.Metrics[0].ContextKey)
	assert.False(t, ok)
}

func TestLogPatternExtractor_SkipsBelowWarnSeverity(t *testing.T) {
	e := NewLogPatternExtractor()

	out := e.ProcessLog(&mockLogView{
		content: []byte("INFO: routine request completed"),
		status:  "info",
		tags:    []string{"service:api"},
	})
	require.Empty(t, out.Metrics)
	require.Empty(t, out.Telemetry)
}

func TestLogPatternExtractor_DeferredEmitUntilMinPatterns(t *testing.T) {
	e := NewLogPatternExtractor()
	status := "warn"
	tags := []string{"service:api"}

	for i := range 4 {
		out := e.ProcessLog(&mockLogView{
			content: []byte(fmt.Sprintf("WARN distinct pattern seed %d not mergeable xyz", i)),
			status:  status,
			tags:    tags,
		})
		require.Empty(t, out.Metrics, "i=%d", i)
	}

	out := e.ProcessLog(&mockLogView{
		content: []byte("WARN distinct pattern seed 4 not mergeable xyz"),
		status:  status,
		tags:    tags,
	})
	require.Len(t, out.Metrics, 1)
}
