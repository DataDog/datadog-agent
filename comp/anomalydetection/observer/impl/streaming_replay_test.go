// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/require"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

type streamAdvanceRecorder struct {
	times []int64
}

func (d *streamAdvanceRecorder) Name() string { return "stream_advance_recorder" }
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

	obs.IngestLogSync("parquet", &mockLogView{timestampMs: 10_000})
	obs.IngestLogSync("parquet", &mockLogView{timestampMs: 12_000})
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
		obs.IngestLogSync("parquet", &mockLogView{timestampMs: second * 1000})
	}
	obs.FinishReplayStream()

	require.Equal(t, int64(storagePointRetentionSecs+1), storage.TotalSampleCount(""))
}
