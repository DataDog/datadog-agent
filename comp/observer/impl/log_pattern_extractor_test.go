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

func TestLogPatternExtractor_GetContextByKeyUsesOutputContextKey(t *testing.T) {
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	e.config.MinClusterSizeBeforeEmit = 1

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
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	e.config.MinClusterSizeBeforeEmit = 1

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

func TestLogPatternExtractor_DifferentHostnamesProduceDifferentMetricNamesWhenNoHostTag(t *testing.T) {
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	e.config.MinClusterSizeBeforeEmit = 1

	msg := []byte("GET /users/123 returned 500")
	tags := []string{"service:api", "env:prod"}

	logA := &mockLogView{
		content:  msg,
		status:   "warn",
		tags:     tags,
		hostname: "host-a",
	}
	logB := &mockLogView{
		content:  msg,
		status:   "warn",
		tags:     tags,
		hostname: "host-b",
	}

	resA := e.ProcessLog(logA)
	resB := e.ProcessLog(logB)
	require.Len(t, resA.Metrics, 1)
	require.Len(t, resB.Metrics, 1)
	require.NotEqual(t, resA.Metrics[0].Name, resB.Metrics[0].Name)
	require.NotEqual(t, resA.Metrics[0].ContextKey, resB.Metrics[0].ContextKey)

	ctxA, ok := e.GetContextByKey(resA.Metrics[0].ContextKey)
	require.True(t, ok)
	ctxB, ok := e.GetContextByKey(resB.Metrics[0].ContextKey)
	require.True(t, ok)
	assert.Equal(t, map[string]string{"service": "api", "env": "prod", "host": "host-a"}, ctxA.SplitTags)
	assert.Equal(t, map[string]string{"service": "api", "env": "prod", "host": "host-b"}, ctxB.SplitTags)
}

func TestLogPatternExtractor_ResetClearsContext(t *testing.T) {
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	e.config.MinClusterSizeBeforeEmit = 1

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
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())

	out := e.ProcessLog(&mockLogView{
		content: []byte("INFO: routine request completed"),
		status:  "info",
		tags:    []string{"service:api"},
	})
	require.Empty(t, out.Metrics)
	require.Empty(t, out.Telemetry)
}

func TestLogPatternExtractor_DeferredEmitUntilMinPatterns(t *testing.T) {
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
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

func TestLogPatternExtractor_GarbageCollectRemovesStaleClusterAndContext(t *testing.T) {
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	e.config.MinClusterSizeBeforeEmit = 1
	e.ClusterTimeToLiveSec = 10
	// GC scheduling uses wall-clock seconds; 0 means the next ProcessLog can run
	// maybeGarbageCollect without waiting an extra second (tests run in one Unix second).
	e.GarbageCollectionIntervalSec = 0

	tags := []string{"service:api"}
	// Distinct messages so the second log does not refresh the first cluster's LastSeenUnix.
	msg1 := []byte("WARN distinct pattern seed 700 not mergeable xyz")
	msg2 := []byte("WARN distinct pattern seed 701 not mergeable xyz")

	// t=1000: create cluster A, emit metric and pattern context.
	const tsMs1 = 1_000_000 // unix sec = 1000
	res1 := e.ProcessLog(&mockLogView{
		content:     msg1,
		status:      "warn",
		tags:        tags,
		timestampMs: tsMs1,
	})
	require.Len(t, res1.Metrics, 1)
	require.Empty(t, res1.EvictedContextKeys, "no GC on first log")
	ctxKey1 := res1.Metrics[0].ContextKey
	_, ok := e.GetContextByKey(ctxKey1)
	require.True(t, ok, "pattern context should exist before GC")

	// t=1015: GC runs first (cutoff 1015-10=1005); cluster A last seen 1000 is stale.
	// Then a new log creates cluster B.
	const tsMs2 = 1_015_000 // unix sec = 1015
	res2 := e.ProcessLog(&mockLogView{
		content:     msg2,
		status:      "warn",
		tags:        tags,
		timestampMs: tsMs2,
	})
	require.Len(t, res2.Metrics, 1)
	require.Equal(t, []string{ctxKey1}, res2.EvictedContextKeys, "GC should report evicted context keys for the engine")
	ctxKey2 := res2.Metrics[0].ContextKey

	_, ok = e.GetContextByKey(ctxKey1)
	assert.False(t, ok, "stale cluster pattern context should be removed by GC")
	_, ok = e.GetContextByKey(ctxKey2)
	require.True(t, ok, "active cluster pattern context should remain")

	require.NotEqual(t, ctxKey1, ctxKey2)

	// Only cluster B should remain in the tagged clusterer.
	remaining := e.taggedClusterer.GetAllClusters()
	require.Len(t, remaining, 1, "stale cluster should be removed from tagged clusterer")
}

func TestLogPatternExtractor_GCEvictedContextKeysTwoTagSetsOneCluster(t *testing.T) {
	// Two different tag-sets that share the same split dimensions (same sub-clusterer)
	// produce two context keys for the same cluster. GC should evict both when the cluster
	// becomes stale. Non-split tags (e.g. "version:") differ so the context keys differ.
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	e.config.MinClusterSizeBeforeEmit = 1
	e.ClusterTimeToLiveSec = 10
	e.GarbageCollectionIntervalSec = 0
	// Pin NextGarbageCollectionTime far in the future so GC does not fire during
	// the setup phase. GarbageCollectionIntervalSec=0 would otherwise trigger GC
	// on every ProcessLog call, evicting the cluster before both tags are ingested.
	e.NextGarbageCollectionTime = 1 << 62

	msg := []byte("WARN disk usage above threshold")
	const tsMs = int64(1_000_000) // unix sec = 1000

	// Same pattern, same split-dimension tag group (service:api) → same sub-clusterer.
	// Different non-split tag "version:" → different context keys, but one cluster.
	resA := e.ProcessLog(&mockLogView{content: msg, status: "warn", tags: []string{"service:api", "version:1"}, timestampMs: tsMs})
	require.Len(t, resA.Metrics, 1, "version:1 must emit a metric")
	resB := e.ProcessLog(&mockLogView{content: msg, status: "warn", tags: []string{"service:api", "version:2"}, timestampMs: tsMs})
	require.Len(t, resB.Metrics, 1, "version:2 must emit a metric")
	require.Equal(t, resA.Metrics[0].Name, resB.Metrics[0].Name,
		"identical messages with same split-dims must share the same cluster metric name")

	// Allow GC to run on the next call.
	e.NextGarbageCollectionTime = 0

	// Trigger GC: cutoff = 1015-10 = 1005, cluster last seen at 1000 is stale.
	res := e.ProcessLog(&mockLogView{
		content:     []byte("WARN distinct pattern seed 999 not mergeable xyz"),
		status:      "warn",
		tags:        []string{"service:api"},
		timestampMs: 1_015_000,
	})

	assert.Len(t, res.EvictedContextKeys, 2, "two tag variants → two context keys evicted")
}

func TestLogPatternExtractor_NoGCBeforeInterval(t *testing.T) {
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	e.config.MinClusterSizeBeforeEmit = 1
	e.ClusterTimeToLiveSec = 10
	e.GarbageCollectionIntervalSec = 3600 // far in the future

	msg := []byte("WARN connection refused to db host *")
	res1 := e.ProcessLog(&mockLogView{content: msg, status: "warn", tags: nil, timestampMs: 1_000_000})
	require.Len(t, res1.Metrics, 1)

	// GC interval not elapsed: no evictions even though cluster would be stale.
	res2 := e.ProcessLog(&mockLogView{content: msg, status: "warn", tags: nil, timestampMs: 2_000_000})
	assert.Empty(t, res2.EvictedContextKeys)
}
