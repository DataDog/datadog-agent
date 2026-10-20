// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"encoding/json"
	"fmt"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	"github.com/stretchr/testify/require"
)

func TestPassthroughCorrelations(t *testing.T) {
	anomalies := []observerdef.Anomaly{
		{DetectorName: "scanmw", Timestamp: 30, Source: observerdef.SeriesDescriptor{Name: "late"}},
		{DetectorName: "bocpd", Timestamp: 20, Source: observerdef.SeriesDescriptor{Name: "first"}},
		{DetectorName: "scanmw", Timestamp: 10, Source: observerdef.SeriesDescriptor{Name: "early"}},
	}

	correlations := passthroughCorrelations(anomalies)
	require.Len(t, correlations, 3)
	require.Equal(t, "passthrough_bocpd_0", correlations[0].Pattern)
	require.Equal(t, int64(20), correlations[0].FirstSeen)
	require.Equal(t, "passthrough_scanmw_0", correlations[1].Pattern)
	require.Equal(t, int64(10), correlations[1].FirstSeen)
	require.Equal(t, "passthrough_scanmw_1", correlations[2].Pattern)
	require.Equal(t, int64(30), correlations[2].FirstSeen)
	for _, correlation := range correlations {
		require.Len(t, correlation.Anomalies, 1)
		require.Equal(t, correlation.FirstSeen, correlation.LastUpdated)
	}
}

func TestStreamingStorageUsesProductionRetention(t *testing.T) {
	settings, err := observerimpl.ParseSettingsFromJSON(map[string]json.RawMessage{
		"bocpd": json.RawMessage(`{"enabled":true,"max_run_length":240}`),
	})
	require.NoError(t, err)
	cfg := streamingStorageCfg(settings, false)
	require.Equal(t, 240, cfg.MaxPointsPerSeries)
	production := observerimpl.StorageConfigForComponents(settings)
	require.Equal(t, production.MaxPointsPerSeries, cfg.MaxPointsPerSeries)
	require.Equal(t, production.PointRetentionSecs, cfg.PointRetentionSecs)
	require.Equal(t, int64(3616), cfg.PointRetentionSecs)
	require.Positive(t, cfg.MaxSeries)
}

func TestStreamingStorageWithoutDetectorsRemainsBounded(t *testing.T) {
	settings := observerimpl.ComponentSettings{Enabled: make(map[string]bool)}
	for _, entry := range observerimpl.TestbenchCatalogEntries() {
		settings.Enabled[entry.Name] = false
	}
	cfg := streamingStorageCfg(settings, false)
	require.Positive(t, cfg.PointRetentionSecs)
	require.Positive(t, cfg.MaxSeries)
}

// Embedding the interface makes unexpected preload/reset calls fail this test.
type orderedReplayRecorder struct {
	observerimpl.DebugView
	events []string
}

func (r *orderedReplayRecorder) IngestMetricSync(_ string, m observerdef.MetricView, _ uint64) {
	r.events = append(r.events, fmt.Sprintf("metric:%d", m.GetTimestampUnix()))
}
func (r *orderedReplayRecorder) IngestLogAndAdvance(_ string, l observerdef.LogView) {
	r.events = append(r.events, fmt.Sprintf("log:%d", l.GetTimestampUnixMilli()))
}
func (*orderedReplayRecorder) AddTelemetry(string, float64, int64, []string) {}

func TestRetainedReplayInterleavesRawObservations(t *testing.T) {
	recorder := &orderedReplayRecorder{}
	bench := &Bench{debug: recorder, replayKeyGenerator: observerimpl.NewSliceKeyGenerator()}
	for _, ts := range []int64{30, 10, 20} {
		view := newParquetMetricView("requests", 1, nil, ts, "gauge", 1)
		bench.rawMetrics = append(bench.rawMetrics, &view)
	}
	for _, ts := range []int64{20002, 5000, 20001, 40000} {
		bench.rawLogs = append(bench.rawLogs, &logDataView{data: &recorderdef.LogData{TimestampMs: ts}})
	}
	bench.feedRawObservations()
	want := []string{"log:5000", "metric:10", "metric:20", "log:20001", "log:20002", "metric:30", "log:40000"}
	require.Equal(t, want, recorder.events)
	// Component toggles must replay the same observations from the beginning.
	recorder.events = nil
	bench.feedRawObservations()
	require.Equal(t, want, recorder.events)
}
