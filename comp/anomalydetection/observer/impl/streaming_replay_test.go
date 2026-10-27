// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

type streamAdvanceRecorder struct {
	times []int64
}

func (d *streamAdvanceRecorder) Name() string { return "stream_advance_recorder" }
func (*streamAdvanceRecorder) Ready() bool    { return true }
func (d *streamAdvanceRecorder) Detect(_ observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	d.times = append(d.times, dataTime)
	return observerdef.DetectionResult{}
}

type streamMetricExtractor struct{}

func (e *streamMetricExtractor) Name() string { return "stream_metric_extractor" }
func (e *streamMetricExtractor) ProcessLog(_ observerdef.LogView) observerdef.LogMetricsExtractorOutput {
	return observerdef.LogMetricsExtractorOutput{Metrics: []observerdef.MetricOutput{{Name: "log.count", Value: 1}}}
}

func TestSynchronousLogStreamAdvancesAndFlushes(t *testing.T) {
	detector := &streamAdvanceRecorder{}
	obs := &observerImpl{engine: newEngine(engineConfig{
		storage:   newTimeSeriesStorage(),
		detectors: []observerdef.Detector{detector},
	})}

	obs.IngestLogAndAdvance("parquet", &mockLogView{timestampMs: 10_000})
	obs.IngestLogAndAdvance("parquet", &mockLogView{timestampMs: 12_000})
	obs.FinishReplayStream()

	require.Equal(t, []int64{9, 11, 12}, detector.times)
	require.Equal(t, int64(12), obs.engine.lastAnalyzedDataTime)
}

func TestSynchronousLogStreamUsesBoundedStorage(t *testing.T) {
	storage := newTimeSeriesStorage()
	obs := &observerImpl{engine: newEngine(engineConfig{
		storage:    storage,
		extractors: []observerdef.LogMetricsExtractor{&streamMetricExtractor{}},
	})}

	for second := int64(0); second < 500; second++ {
		obs.IngestLogAndAdvance("parquet", &mockLogView{timestampMs: second * 1000})
	}
	obs.FinishReplayStream()

	require.Equal(t, int64(storagePointRetentionSecs+1), storage.TotalSampleCount(""))
}

// Slowly sampled metrics must warm up before retention starts discarding their
// history, and storage must stay bounded after a long stream.
func TestStreamingStorageWarmsSparseBOCPD(t *testing.T) {
	for _, warmup := range []int{40, 60} {
		cfg := DefaultBOCPDConfig()
		cfg.WarmupPoints = warmup
		settings := ComponentSettings{configs: map[string]any{"bocpd": cfg}}
		storageCfg := StorageConfigForComponents(settings)
		storage := newTimeSeriesStorageWith(storageCfg)
		detector := NewBOCPDDetector(cfg)
		for i := 1; i <= 1000; i++ {
			ts := int64(i * 15)
			storage.Add("parquet", "requests", float64(100+i%3), ts, nil)
			detector.Detect(storage, ts)
			if i == warmup {
				require.True(t, detector.Ready(), "warmup=%d", warmup)
			}
		}
		require.LessOrEqual(t, storage.TotalSampleCount(""), int64(cfg.MaxRunLength+1))
		require.GreaterOrEqual(t, storage.TotalSampleCount(""), int64(warmup))
	}
}

func TestStreamingStorageUsesConfiguredEnabledDetectors(t *testing.T) {
	settings, err := ParseSettingsFromJSON(map[string]json.RawMessage{
		"bocpd":         json.RawMessage(`{"enabled":true,"warmup_points":80,"max_run_length":240}`),
		"holt_residual": json.RawMessage(`{"enabled":false,"warmup_points":1000}`),
	})
	require.NoError(t, err)
	cfg := StorageConfigForComponents(settings)
	require.Equal(t, 240, cfg.MaxPointsPerSeries)
	require.Equal(t, int64(3616), cfg.PointRetentionSecs)
}
