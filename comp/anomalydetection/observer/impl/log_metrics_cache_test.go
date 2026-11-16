// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/stretchr/testify/require"
)

func TestEngine_LogMetricsPatternLRUEvictionFreesStorage(t *testing.T) {
	cfg := DefaultLogMetricsExtractorConfig()
	cfg.MaxPatternsPerGroup = 2
	extractor := NewLogMetricsExtractor(cfg)

	storage := newTimeSeriesStorage()
	e := mustNewEngine(engineConfig{
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
	require.Equal(t, 2, storage.TotalSeriesCount(),
		"LRU eviction must shrink storage; before the fix storage grew unboundedly")

	// Surviving series must have context stored on them.
	for _, meta := range storage.ListSeries(observerdef.SeriesFilter{Namespace: extractor.Name()}) {
		_, ok := storage.GetContext(meta.Ref)
		require.True(t, ok,
			"surviving series must have inline MetricContext (ref=%d)", meta.Ref)
	}
}

// TestEngine_LogMetricsPatternLRUEvictionFreesDetectorState extends
// TestEngine_LogMetricsPatternLRUEvictionFreesStorage to cover the detector side.
// Storage frees the evicted series via RemoveSeriesByKeys, but stateful
// detectors keep parallel per-series maps (BOCPD posterior, ScanMW/ScanWelch
// segment trackers, seriesDetectorAdapter.lastVisibleCount) keyed by the
// SeriesRef they observed during Detect(). Without engine.fanOutSeriesRemoval
// those maps grow with the cumulative number of series ever seen, defeating
// the LRU caps on the upstream extractors. This test checks that BOCPD's map
// shrinks back in lockstep with storage.
func TestEngine_LogMetricsPatternLRUEvictionFreesDetectorState(t *testing.T) {
	cfg := DefaultLogMetricsExtractorConfig()
	cfg.MaxPatternsPerGroup = 2
	extractor := NewLogMetricsExtractor(cfg)

	bocpdConfig := DefaultBOCPDConfig()
	bocpdConfig.WarmupPoints = 2
	bocpdConfig.MaxRunLength = 2
	bocpd := NewBOCPDDetector(bocpdConfig)
	scanmwConfig := DefaultScanMWConfig()
	scanmwConfig.MinPoints, scanmwConfig.MinSegment = 2, 1
	scanmw := NewScanMWDetector(scanmwConfig)
	scanwelchCfg := DefaultScanWelchConfig()
	scanwelchCfg.MinPoints = 2
	scanwelchCfg.MinSegment = 1
	scanwelch := NewScanWelchDetector(scanwelchCfg)

	// Stateless detector that does NOT implement SeriesRemover. Registering it
	// alongside the stateful ones exercises the fanOutSeriesRemoval type-assertion
	// fast-path: detectors without per-series state must be silently skipped, never
	// invoked with RemoveSeries (which would panic since they don't implement it),
	// and never block the eviction broadcast to the stateful detectors that follow.
	stateless := &statelessTestDetector{name: "stateless"}

	storage := newTimeSeriesStorage()
	e := mustNewEngine(engineConfig{
		storage:    storage,
		extractors: []observerdef.LogMetricsExtractor{extractor},
		detectors: []detectorRegistration{
			{detector: bocpd},
			{detector: scanmw},
			{detector: scanwelch},
			{detector: stateless},
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
	for _, meta := range storage.ListSeries(observerdef.WorkloadSeriesFilter()) {
		storage.AddWithKeyAndHostComposite(meta.Namespace, meta.Name, meta.Host, 1, 1_002, meta.Tags, testStorageKeyForMetric(meta.Namespace, &metricObs{name: meta.Name, host: meta.Host, tags: meta.Tags}), meta.MetricType)
	}

	// Drive Detect() so the detectors observe the series and populate their
	// per-series state maps. dataTime needs to be ahead of the last point.
	bocpd.Detect(storage, 1_002, observerdef.DetectionFilter{})
	scanmw.Detect(storage, 1_002, observerdef.DetectionFilter{})
	scanwelch.Detect(storage, 1_002, observerdef.DetectionFilter{})

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
	require.Equal(t, 2, storage.TotalSeriesCount(), "LRU should keep storage bounded")

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
	require.LessOrEqual(t, len(bocpd.series), 2*len(defaultAggregations),
		"BOCPD per-series map must not exceed live series × aggregations")
	require.Less(t, len(scanmw.series), scanmwBefore,
		"ScanMW per-series map must shrink when storage evicts a series; without the fan-out it stays at %d", scanmwBefore)
	require.LessOrEqual(t, len(scanmw.series), 2*len(defaultAggregations),
		"ScanMW per-series map must not exceed live series × aggregations")
	require.Less(t, len(scanwelch.series), scanwelchBefore,
		"ScanWelch per-series map must shrink when storage evicts a series; without the fan-out it stays at %d", scanwelchBefore)
	require.LessOrEqual(t, len(scanwelch.series), 2*len(defaultAggregations),
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
func (*statelessTestDetector) Ready() bool    { return true }
func (s *statelessTestDetector) Detect(_ observerdef.StorageReader, _ int64, _ observerdef.DetectionFilter) observerdef.DetectionResult {
	return observerdef.DetectionResult{}
}
