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
	cfg.MinTokenMatchRatio = 0.9
	cfg.maxPatternsPerGroup = 2
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

	// The third pattern evicts the oldest series before admitting its replacement.
	require.Equal(t, 2, storage.TotalSeriesCount(),
		"LRU eviction must shrink storage; before the fix storage grew unboundedly")

	// Surviving series must have context stored on them.
	for _, meta := range storage.ListSeries(observerdef.SeriesFilter{Namespace: extractor.Name()}) {
		_, ok := storage.GetContext(meta.Ref)
		require.True(t, ok,
			"surviving series must have inline MetricContext (ref=%d)", meta.Ref)
	}
}

// Eviction must release detector state as well as storage series.
func TestEngine_LogMetricsPatternLRUEvictionFreesDetectorState(t *testing.T) {
	cfg := DefaultLogMetricsExtractorConfig()
	cfg.MinTokenMatchRatio = 0.9
	cfg.maxPatternsPerGroup = 2
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

	// Cleanup must skip detectors that do not implement SeriesRemover.
	stateless := &logMetricsStatelessTestDetector{name: "stateless"}

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

	// Populate detector state before triggering eviction.
	bocpd.Detect(storage, 1_002, observerdef.DetectionFilter{})
	scanmw.Detect(storage, 1_002, observerdef.DetectionFilter{})
	scanwelch.Detect(storage, 1_002, observerdef.DetectionFilter{})

	bocpdBefore := len(bocpd.series)
	scanmwBefore := len(scanmw.series)
	scanwelchBefore := len(scanwelch.series)
	require.Greater(t, bocpdBefore, 0, "BOCPD should have observed the series before eviction")
	require.Greater(t, scanmwBefore, 0, "ScanMW should have observed the series before eviction")
	require.Greater(t, scanwelchBefore, 0, "ScanWelch should have observed the series before eviction")

	// A third pattern evicts the oldest series and its detector state.
	e.IngestLog("src", &logObs{
		content:     "WARN x y z w",
		status:      "warn",
		tags:        tags,
		timestampMs: 1_002_000,
	})

	// Storage and detector maps must both remain bounded by the surviving series.
	require.Equal(t, 2, storage.TotalSeriesCount(), "LRU should keep storage bounded")

	// Each detector retains at most one entry per surviving series and aggregation.
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

	// The registered stateless detector must survive the same cleanup pass.
	_ = stateless
}

// logMetricsStatelessTestDetector has no per-series state or SeriesRemover method.
type logMetricsStatelessTestDetector struct {
	name string
}

func (s *logMetricsStatelessTestDetector) Name() string { return s.name }
func (*logMetricsStatelessTestDetector) Ready() bool    { return true }
func (s *logMetricsStatelessTestDetector) Detect(_ observerdef.StorageReader, _ int64, _ observerdef.DetectionFilter) observerdef.DetectionResult {
	return observerdef.DetectionResult{}
}
