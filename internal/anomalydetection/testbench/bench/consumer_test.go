// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

func TestStreamingStorageUsesProductionPolicies(t *testing.T) {
	settings, err := observerimpl.ParseSettingsFromJSON(map[string]json.RawMessage{
		"bocpd": json.RawMessage(`{"enabled":true,"max_run_length":240}`),
	})
	require.NoError(t, err)
	cfg, err := streamingStorageCfg(settings, false)
	require.NoError(t, err)
	require.Equal(t, 240, cfg.MaxPointsPerSeries)
	production, err := observerimpl.StorageConfigForComponents(settings)
	require.NoError(t, err)
	// These flags capture already-emitted results; they do not change inputs
	// or detector-facing storage. Every other policy must match production.
	cfg.MaxCorrelations = production.MaxCorrelations
	cfg.TrackCorrelationHistory = production.TrackCorrelationHistory
	cfg.TrackAnomalyHistory = production.TrackAnomalyHistory
	cfg.TrackDetectorOutputHistory = production.TrackDetectorOutputHistory
	require.Equal(t, production, cfg)
	require.Equal(t, int64(3616), cfg.PointRetentionSecs)
	require.Positive(t, cfg.InactiveSeriesTTLSeconds)
	require.Positive(t, cfg.InactiveSeriesCheckIntervalSeconds)

}

func TestStreamingStorageWithoutDetectorsRemainsBounded(t *testing.T) {
	settings := observerimpl.ComponentSettings{Enabled: make(map[string]bool)}
	for _, entry := range observerimpl.TestbenchCatalogEntries() {
		settings.Enabled[entry.Name] = false
	}
	cfg, err := streamingStorageCfg(settings, false)
	require.NoError(t, err)
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

func TestLoadTestbenchParamsInheritsReplayDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "params.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"components":{"bocpd":{"enabled":true}}}`), 0o600))
	settings, err := LoadTestbenchParams(path)
	require.NoError(t, err)
	// Mirror the CLI's final defaulting pass. An enabled-only JSON entry must
	// match toggling BOCPD directly, including the replay warmup configuration.
	settings = observerimpl.ApplyTestbenchDefaults(settings)
	expected := observerimpl.ApplyTestbenchDefaults(observerimpl.ComponentSettings{
		Enabled: map[string]bool{"bocpd": true},
	})
	require.Equal(t, expected, settings)
}

func TestLoadTestbenchParamsAnomalyCutoff(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want *bool
	}{
		{"default", `{"components":{}}`, nil},
		{"enabled", `{"anomaly_cutoff":true,"components":{}}`, ptrBool(true)},
		{"disabled", `{"anomaly_cutoff":false,"components":{}}`, ptrBool(false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "params.json")
			require.NoError(t, os.WriteFile(path, []byte(tc.body), 0o600))
			settings, err := LoadTestbenchParams(path)
			require.NoError(t, err)
			require.Equal(t, tc.want, settings.MetricPipelineAnomalyCutoff)
			settings = observerimpl.ApplyTestbenchDefaults(settings)
			require.NotNil(t, settings.MetricPipelineAnomalyCutoff)
			require.True(t, *settings.MetricPipelineAnomalyCutoff || tc.want != nil && !*tc.want)
		})
	}
}

func ptrBool(v bool) *bool { return &v }

type workflowResetRecorder struct {
	observerimpl.DebugView
	settings []observerimpl.ComponentSettings
}

func (r *workflowResetRecorder) Reset(settings observerimpl.ComponentSettings, _ observerimpl.StorageConfig) error {
	r.settings = append(r.settings, settings)
	return nil
}

func TestWorkflowSurvivesTestbenchReset(t *testing.T) {
	for _, pipeline := range []bool{false, true} {
		recorder := &workflowResetRecorder{}
		tb := &Bench{
			debug:    recorder,
			settings: observerimpl.ComponentSettings{UseMetricPipeline: pipeline, Enabled: map[string]bool{}},
		}
		for _, streaming := range []bool{false, true} {
			tb.config.StreamParquet = streaming
			require.NoError(t, tb.resetAllState())
			tb.settings.Enabled["log_pattern_extractor"] = false
			require.NoError(t, tb.resetAllState())
		}
		require.Len(t, recorder.settings, 4)
		for _, settings := range recorder.settings {
			require.Equal(t, pipeline, settings.UseMetricPipeline)
		}
		for _, entry := range tb.catalogEntries() {
			if pipeline {
				require.NotEqual(t, "detector", entry.Kind, "inactive legacy detectors are not UI controls")
			}
		}
	}
}
