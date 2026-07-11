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

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

func TestLogPatternExtractor_MetricOutputCarriesInlineContext(t *testing.T) {
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	e.config.MinClusterSizeBeforeEmit = 1

	log := &mockLogView{
		content: "GET /users/123 returned 500",
		status:  "warn",
		tags:    []string{"service:web", "env:prod"},
	}

	res := e.ProcessLog(log)
	require.Len(t, res.Metrics, 1)
	require.NotNil(t, res.Metrics[0].Context)

	ctx := res.Metrics[0].Context
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
		content: "GET /users/123 returned 500",
		status:  "warn",
		tags:    []string{"service:api"},
	}
	logB := &mockLogView{
		content: "GET /users/456 returned 500",
		status:  "warn",
		tags:    []string{"service:worker"},
	}
	logC := &mockLogView{
		content: "GET /users/124 returned 500",
		status:  "warn",
		tags:    []string{"service:api"},
	}
	logD := &mockLogView{
		content: "GET /users/457 returned 500",
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
	require.NotNil(t, resA.Metrics[0].Context)
	require.NotNil(t, resB.Metrics[0].Context)

	ctxA := resA.Metrics[0].Context
	ctxB := resB.Metrics[0].Context

	assert.Equal(t, "GET /users/123 returned 500", ctxA.Example)
	assert.Equal(t, "GET /users/456 returned 500", ctxB.Example)
	assert.NotEmpty(t, ctxA.Pattern)
	assert.NotEmpty(t, ctxB.Pattern)
	assert.Equal(t, map[string]string{"service": "api"}, ctxA.SplitTags)
	assert.Equal(t, map[string]string{"service": "worker"}, ctxB.SplitTags)
}

func TestLogPatternExtractor_DifferentHostnamesProduceDifferentMetricNamesWhenNoHostTag(t *testing.T) {
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	e.config.MinClusterSizeBeforeEmit = 1

	msg := "GET /users/123 returned 500"
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
	require.NotNil(t, resA.Metrics[0].Context)
	require.NotNil(t, resB.Metrics[0].Context)
	assert.Equal(t, map[string]string{"service": "api", "env": "prod", "host": "host-a"}, resA.Metrics[0].Context.SplitTags)
	assert.Equal(t, map[string]string{"service": "api", "env": "prod", "host": "host-b"}, resB.Metrics[0].Context.SplitTags)
}

func TestLogPatternExtractor_ResetClearsClusterState(t *testing.T) {
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	e.config.MinClusterSizeBeforeEmit = 1

	log := &mockLogView{
		content: "GET /users/123 returned 500",
		status:  "warn",
		tags:    []string{"service:web"},
	}

	res := e.ProcessLog(log)
	require.Len(t, res.Metrics, 1)
	require.NotNil(t, res.Metrics[0].Context)

	e.Reset()

	// After reset the tagged clusterer is cleared; the same log starts a fresh cluster.
	require.Empty(t, e.taggedClusterer.GetAllClusters(), "Reset must clear cluster state")
}

func TestLogPatternExtractor_SkipsBelowWarnSeverity(t *testing.T) {
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())

	out := e.ProcessLog(&mockLogView{
		content: "INFO: routine request completed",
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
			content: fmt.Sprintf("WARN distinct pattern seed %d not mergeable xyz", i),
			status:  status,
			tags:    tags,
		})
		require.Empty(t, out.Metrics, "i=%d", i)
	}

	out := e.ProcessLog(&mockLogView{
		content: "WARN distinct pattern seed 4 not mergeable xyz",
		status:  status,
		tags:    tags,
	})
	require.Len(t, out.Metrics, 1)
}

func TestLogPatternExtractor_ZeroConfigAppliesGCDefaults(t *testing.T) {
	e := NewLogPatternExtractor(LogPatternExtractorConfig{})
	defaults := DefaultLogPatternExtractorConfig()
	assert.Equal(t, defaults.ClusterTimeToLiveSec, e.config.ClusterTimeToLiveSec)
	assert.Equal(t, defaults.GarbageCollectionIntervalSec, e.config.GarbageCollectionIntervalSec)
}

func TestLogPatternExtractor_GarbageCollectRemovesStaleClusterAndContext(t *testing.T) {
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	e.config.MinClusterSizeBeforeEmit = 1
	e.config.ClusterTimeToLiveSec = 10
	// GC scheduling uses wall-clock seconds; 0 means the next ProcessLog can run
	// maybeGarbageCollect without waiting an extra second (tests run in one Unix second).
	e.config.GarbageCollectionIntervalSec = 0

	tags := []string{"service:api"}
	// Distinct messages so the second log does not refresh the first cluster's LastSeenUnix.
	msg1 := "WARN distinct pattern seed 700 not mergeable xyz"
	msg2 := "WARN distinct pattern seed 701 not mergeable xyz"

	// t=1000: create cluster A, emit metric and pattern context.
	const tsMs1 = 1_000_000 // unix sec = 1000
	res1 := e.ProcessLog(&mockLogView{
		content:     msg1,
		status:      "warn",
		tags:        tags,
		timestampMs: tsMs1,
	})
	require.Len(t, res1.Metrics, 1)
	require.Empty(t, res1.EvictedMetricNames, "no GC on first log")
	metricName1 := res1.Metrics[0].Name
	require.NotNil(t, res1.Metrics[0].Context, "pattern context should be inline on first metric")

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
	require.Equal(t, []string{metricName1}, res2.EvictedMetricNames, "GC should report evicted metric names for storage cleanup")
	require.NotNil(t, res2.Metrics[0].Context)
	require.NotEqual(t, metricName1, res2.Metrics[0].Name)

	// Only cluster B should remain in the tagged clusterer.
	remaining := e.taggedClusterer.GetAllClusters()
	require.Len(t, remaining, 1, "stale cluster should be removed from tagged clusterer")
}

func TestLogPatternExtractor_DisableOptimizationsSkipsGarbageCollection(t *testing.T) {
	cfg := DefaultLogPatternExtractorConfig()
	cfg.DisableOptimizations = true
	e := NewLogPatternExtractor(cfg)
	require.Zero(t, e.config.ClusterTimeToLiveSec, "disable optimizations must clear TTL")
	require.Zero(t, e.config.GarbageCollectionIntervalSec, "disable optimizations must clear GC interval")

	tags := []string{"service:api"}
	// Structurally different lines so the pattern clusterer keeps two clusters (unlike
	// two strings that differ only by a numeric token, which often merge into one template).
	msg1 := `10.143.180.25 - - [27/Aug/2020:00:27:02 +0000] "POST /api/v1/series HTTP/1.1" 202 16`
	msg2 := `2020-08-27 02:32:42 ERROR (connector.go:34) - Failed to connected to redis`

	const tsMs1 = 1_000_000 // unix sec = 1000
	res1 := e.ProcessLog(&mockLogView{
		content:     msg1,
		status:      "warn",
		tags:        tags,
		timestampMs: tsMs1,
	})
	require.Len(t, res1.Metrics, 1)
	require.NotNil(t, res1.Metrics[0].Context)

	// Same timeline as TestLogPatternExtractor_GarbageCollectRemovesStaleClusterAndContext, where GC
	// would evict cluster A — but with DisableOptimizations, TTL is off so A stays.
	const tsMs2 = 1_015_000 // unix sec = 1015
	res2 := e.ProcessLog(&mockLogView{
		content:     msg2,
		status:      "warn",
		tags:        tags,
		timestampMs: tsMs2,
	})
	require.Len(t, res2.Metrics, 1)
	require.Empty(t, res2.EvictedMetricNames, "GC must not run when optimizations are disabled")
	require.NotNil(t, res2.Metrics[0].Context)

	remaining := e.taggedClusterer.GetAllClusters()
	require.Len(t, remaining, 2, "both clusters should still exist when GC is disabled")
}

func TestLogPatternExtractor_GCEvictedContextKeysTwoTagSetsOneCluster(t *testing.T) {
	// Two different tag-sets that share the same split dimensions (same sub-clusterer)
	// produce two context keys for the same cluster. GC should evict both when the cluster
	// becomes stale. Non-split tags (e.g. "version:") differ so the context keys differ.
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	e.config.MinClusterSizeBeforeEmit = 1
	e.config.ClusterTimeToLiveSec = 10
	e.config.GarbageCollectionIntervalSec = 0
	// Pin NextGarbageCollectionTime far in the future so GC does not fire during
	// the setup phase. GarbageCollectionIntervalSec=0 would otherwise trigger GC
	// on every ProcessLog call, evicting the cluster before both tags are ingested.
	e.NextGarbageCollectionTime = 1 << 62

	msg := "WARN disk usage above threshold"
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
		content:     "WARN distinct pattern seed 999 not mergeable xyz",
		status:      "warn",
		tags:        []string{"service:api"},
		timestampMs: 1_015_000,
	})

	// With the new metric-name eviction design, one cluster → one metric name evicted.
	// RemoveSeriesByMetricName removes all tag variants in a single storage call.
	assert.Len(t, res.EvictedMetricNames, 1, "one cluster → one metric name evicted (all tag variants removed by storage)")
}

func TestLogPatternExtractor_NoGCBeforeInterval(t *testing.T) {
	e := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	e.config.MinClusterSizeBeforeEmit = 1
	e.config.ClusterTimeToLiveSec = 10
	e.config.GarbageCollectionIntervalSec = 3600 // far in the future

	msg := "WARN connection refused to db host *"
	res1 := e.ProcessLog(&mockLogView{content: msg, status: "warn", tags: nil, timestampMs: 1_000_000})
	require.Len(t, res1.Metrics, 1)

	// GC interval not elapsed: no evictions even though cluster would be stale.
	res2 := e.ProcessLog(&mockLogView{content: msg, status: "warn", tags: nil, timestampMs: 2_000_000})
	assert.Empty(t, res2.EvictedMetricNames)
}

func TestLogPatternExtractor_LRUCapEvictsAndDropsContext(t *testing.T) {
	// Configure tight cap with MinClusterSizeBeforeEmit=1 so each new shape
	// emits a metric (and therefore a context entry) on its first appearance.
	cfg := DefaultLogPatternExtractorConfig()
	cfg.MinClusterSizeBeforeEmit = 1
	cfg.MaxPatternsPerGroup = 2
	cfg.MaxTagGroups = -1 // disable group cap so we test only per-group LRU
	e := NewLogPatternExtractor(cfg)

	tags := []string{"service:api"}
	// Three distinct shapes (different token counts → different signatures →
	// different clusters; they don't merge).
	msgs := []string{
		"WARN alpha",
		"WARN beta gamma",
		"WARN x y z w",
	}

	var metricNames []string
	for i, m := range msgs {
		res := e.ProcessLog(&mockLogView{
			content:     m,
			status:      "warn",
			tags:        tags,
			timestampMs: int64(1_000_000 + i*1_000), // 1s apart so LastSeenUnix differs
		})
		require.Len(t, res.Metrics, 1, "each distinct shape should emit a metric (i=%d)", i)
		require.NotNil(t, res.Metrics[0].Context)
		metricNames = append(metricNames, res.Metrics[0].Name)

		switch i {
		case 0, 1:
			require.Empty(t, res.EvictedMetricNames, "no eviction below or at cap (i=%d)", i)
		case 2:
			// Third shape pushes over the cap of 2; the oldest (metricNames[0]) is evicted.
			require.Equal(t, []string{metricNames[0]}, res.EvictedMetricNames,
				"oldest cluster's metric name surfaced for storage cleanup")
		}
	}

	require.Equal(t, 1, e.taggedClusterer.NumSubClusterers(), "single tag group across all messages")
	require.Len(t, e.taggedClusterer.GetAllClusters(), 2, "cap holds at MaxPatternsPerGroup=2")
}

func TestLogPatternExtractor_TagGroupCapEvictsLRUGroup(t *testing.T) {
	cfg := DefaultLogPatternExtractorConfig()
	cfg.MinClusterSizeBeforeEmit = 1
	cfg.MaxPatternsPerGroup = -1 // disable per-group cap
	cfg.MaxTagGroups = 2
	e := NewLogPatternExtractor(cfg)

	processAt := func(service string, msg string, tsMs int64) string {
		res := e.ProcessLog(&mockLogView{
			content:     msg,
			status:      "warn",
			tags:        []string{"service:" + service},
			timestampMs: tsMs,
		})
		require.Len(t, res.Metrics, 1)
		return res.Metrics[0].Name
	}

	// Two groups, two metrics.
	mA := processAt("a", "WARN alpha", 1_000_000)
	mB := processAt("b", "WARN beta", 1_001_000)

	// Touch A again so B is the LRU group.
	_ = processAt("a", "WARN alpha", 1_002_000)

	// Adding a third group must evict B's lone cluster.
	res := e.ProcessLog(&mockLogView{
		content:     "WARN gamma",
		status:      "warn",
		tags:        []string{"service:c"},
		timestampMs: 1_003_000,
	})
	require.Len(t, res.Metrics, 1)
	require.Contains(t, res.EvictedMetricNames, mB, "LRU group's metric name surfaced for storage cleanup")
	assert.NotContains(t, res.EvictedMetricNames, mA, "surviving group's metric not evicted")
}

// TestEngine_LogPatternLRUEvictionFreesStorage is the end-to-end proof that
// the structural leak is fixed: when the extractor's LRU evicts a cluster,
// the engine no longer just drops its contextRefs entry — it also calls
// storage.RemoveSeriesByKeys so the per-series tags slice + columnar arrays
// + sample buffer are actually freed. Before this fix, timeSeriesStorage.series
// grew monotonically for the lifetime of the agent, regardless of LRU caps.
func TestEngine_LogPatternLRUEvictionFreesStorage(t *testing.T) {
	cfg := DefaultLogPatternExtractorConfig()
	cfg.MinClusterSizeBeforeEmit = 1
	cfg.MaxPatternsPerGroup = 2
	cfg.MaxTagGroups = -1
	extractor := NewLogPatternExtractor(cfg)

	storage := newTimeSeriesStorage()
	e := newEngine(engineConfig{
		storage:    storage,
		extractors: []observerdef.LogMetricsExtractor{extractor},
	})

	tags := []string{"service:api"}
	msgs := []string{
		"WARN alpha",
		"WARN beta gamma",
		"WARN x y z w",
	}

	for i, m := range msgs {
		e.IngestLog("src", &logObs{
			content:     m,
			status:      "warn",
			tags:        tags,
			timestampMs: int64(1_000_000 + i*1_000),
		})
	}

	// Without the storage-side eviction, count would be 3 (every shape ever
	// seen leaves a series behind). With it, the LRU eviction during the 3rd
	// ingest removes cluster #1 from storage before cluster #3's series is
	// added, so count is 2.
	require.Equal(t, 2, storage.TotalSeriesCount(""),
		"LRU eviction must shrink storage; before the fix storage grew unboundedly")

	// Surviving series must have context stored on them.
	for _, meta := range storage.ListSeries(observerdef.SeriesFilter{Namespace: extractor.Name()}) {
		require.NotNil(t, storage.GetContext(meta.Ref),
			"surviving series must have inline MetricContext (ref=%d)", meta.Ref)
	}
}

// TestEngine_LogPatternLRUEvictionFreesDetectorState extends
// TestEngine_LogPatternLRUEvictionFreesStorage to cover the detector side.
// Storage frees the evicted series via RemoveSeriesByKeys, but stateful
// detectors keep parallel per-series maps (BOCPD posterior, ScanMW/ScanWelch
// segment trackers, seriesDetectorAdapter.lastVisibleCount) keyed by the
// SeriesRef they observed during Detect(). Without engine.fanOutSeriesRemoval
// those maps grow with the cumulative number of series ever seen, defeating
// the LRU caps on the upstream extractors. This test checks that BOCPD's map
// shrinks back in lockstep with storage.
func TestEngine_LogPatternLRUEvictionFreesDetectorState(t *testing.T) {
	cfg := DefaultLogPatternExtractorConfig()
	cfg.MinClusterSizeBeforeEmit = 1
	cfg.MaxPatternsPerGroup = 2
	cfg.MaxTagGroups = -1
	extractor := NewLogPatternExtractor(cfg)

	bocpd := NewBOCPDDetector(BOCPDConfig{})
	scanmw := NewScanMWDetector()
	scanwelch := NewScanWelchDetector()

	// Stateless detector that does NOT implement SeriesRemover. Registering it
	// alongside the stateful ones exercises the fanOutSeriesRemoval type-assertion
	// fast-path: detectors without per-series state must be silently skipped, never
	// invoked with RemoveSeries (which would panic since they don't implement it),
	// and never block the eviction broadcast to the stateful detectors that follow.
	stateless := &statelessTestDetector{name: "stateless"}

	storage := newTimeSeriesStorage()
	e := newEngine(engineConfig{
		storage:    storage,
		extractors: []observerdef.LogMetricsExtractor{extractor},
		detectors: []observerdef.Detector{
			bocpd,
			scanmw,
			scanwelch,
			stateless,
		},
	})

	tags := []string{"service:api"}
	msgs := []string{
		"WARN alpha",
		"WARN beta gamma",
	}
	for i, m := range msgs {
		e.IngestLog("src", &logObs{
			content:     m,
			status:      "warn",
			tags:        tags,
			timestampMs: int64(1_000_000 + i*1_000),
		})
	}

	// Drive Detect() so the detectors observe the series and populate their
	// per-series state maps. dataTime needs to be ahead of the last point.
	bocpd.Detect(storage, 1_001_000)
	scanmw.Detect(storage, 1_001_000)
	scanwelch.Detect(storage, 1_001_000)

	bocpdBefore := len(bocpd.series)
	scanmwBefore := len(scanmw.series)
	scanwelchBefore := len(scanwelch.series)
	require.Greater(t, bocpdBefore, 0, "BOCPD should have observed the series before eviction")
	require.Greater(t, scanmwBefore, 0, "ScanMW should have observed the series before eviction")
	require.Greater(t, scanwelchBefore, 0, "ScanWelch should have observed the series before eviction")

	// Trigger LRU eviction by ingesting a third pattern that pushes the
	// oldest cluster out. After the fix, the engine fans the freed refs
	// out to every detector and the per-series maps shrink accordingly.
	e.IngestLog("src", &logObs{
		content:     "WARN x y z w",
		status:      "warn",
		tags:        tags,
		timestampMs: 1_002_000,
	})

	// Storage shrunk to two series (LRU cap), so detector maps must now
	// have at most two entries per agg too. Without the fan-out, they
	// would still hold three entries (one per series ever observed).
	require.Equal(t, 2, storage.TotalSeriesCount(""), "LRU should keep storage bounded")

	// Each detector defaults to 2 aggregations (Average, Count). Before the
	// fan-out fix, the maps held 3 series × 2 aggs = 6 entries even though
	// only 2 series remained live. After the fix the engine drops the evicted
	// ref's entries, so we expect at most 2 × 2 = 4 (and certainly fewer than
	// the pre-eviction count, which had to grow first to detect the leak).
	// Before the fan-out fix the maps held bocpdBefore entries (2 series ×
	// 2 aggs = 4) and stayed there even after one of the series was evicted,
	// because storage cleanup didn't propagate to detector state. After the
	// fix, fanOutSeriesRemoval drops exactly the evicted ref's entries.
	require.Less(t, len(bocpd.series), bocpdBefore,
		"BOCPD per-series map must shrink when storage evicts a series; without the fan-out it stays at %d", bocpdBefore)
	require.LessOrEqual(t, len(bocpd.series), 2*len(bocpd.config.Aggregations),
		"BOCPD per-series map must not exceed live series × aggregations")
	require.Less(t, len(scanmw.series), scanmwBefore,
		"ScanMW per-series map must shrink when storage evicts a series; without the fan-out it stays at %d", scanmwBefore)
	require.LessOrEqual(t, len(scanmw.series), 2*len(scanmw.Aggregations),
		"ScanMW per-series map must not exceed live series × aggregations")
	require.Less(t, len(scanwelch.series), scanwelchBefore,
		"ScanWelch per-series map must shrink when storage evicts a series; without the fan-out it stays at %d", scanwelchBefore)
	require.LessOrEqual(t, len(scanwelch.series), 2*len(scanwelch.Aggregations),
		"ScanWelch per-series map must not exceed live series \u00d7 aggregations")

	// Sanity check the stateless detector: registering it alongside the
	// stateful ones means the eviction fan-out above iterated over it. The
	// SeriesRemover type-assertion in fanOutSeriesRemoval is what keeps that
	// safe — if it ever regresses (e.g. someone replaces the optional check
	// with a hard call), this test panics on the runtime type-assertion
	// failure during the eviction triggered above.
	_ = stateless
}

// statelessTestDetector is a minimal observerdef.Detector that intentionally
// does NOT implement observerdef.SeriesRemover. Used to verify that the
// engine's eviction fan-out doesn't assume every detector tracks per-series
// state.
type statelessTestDetector struct {
	name string
}

func (s *statelessTestDetector) Name() string { return s.name }
func (s *statelessTestDetector) Detect(_ observerdef.StorageReader, _ int64) observerdef.DetectionResult {
	return observerdef.DetectionResult{}
}
