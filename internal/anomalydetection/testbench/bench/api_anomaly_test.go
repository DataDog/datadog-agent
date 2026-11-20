// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/stretchr/testify/require"
	"strings"
)

type anomalyAPIDebugView struct {
	observerimpl.DebugView
	state observerimpl.StateView
}

func (v anomalyAPIDebugView) StateView() observerimpl.StateView { return v.state }
func (v anomalyAPIDebugView) StorageReader() observerdef.StorageReader {
	return nil
}

type anomalyAPIStateView struct {
	observerimpl.StateView
	meta        observerdef.SeriesMeta
	series      observerdef.Series
	anomaly     observerdef.Anomaly
	correlation observerdef.ActiveCorrelation
	scorer      observerimpl.ScorerHistory
}

func (v anomalyAPIStateView) ListSeries(observerdef.SeriesFilter) []observerdef.SeriesMeta {
	return []observerdef.SeriesMeta{v.meta}
}
func (v anomalyAPIStateView) GetSeriesRange(observerdef.SeriesRef, int64, int64, observerdef.Aggregate) *observerdef.Series {
	return &v.series
}
func (v anomalyAPIStateView) MaxTimestamp() int64 { return v.anomaly.Timestamp }
func (v anomalyAPIStateView) Anomalies() []observerdef.Anomaly {
	return []observerdef.Anomaly{v.anomaly}
}
func (v anomalyAPIStateView) ListDetectors() []observerimpl.ComponentStateInfo {
	return []observerimpl.ComponentStateInfo{{Name: v.anomaly.DetectorName, Enabled: true}}
}
func (v anomalyAPIStateView) CorrelationHistory() []observerdef.ActiveCorrelation {
	return []observerdef.ActiveCorrelation{v.correlation}
}
func (v anomalyAPIStateView) ScorerHistory() observerimpl.ScorerHistory {
	return v.scorer
}

type scorerSnapshotStateView struct {
	observerimpl.StateView
	checkLocked func()
}

func (v scorerSnapshotStateView) ScorerHistory() observerimpl.ScorerHistory {
	v.checkLocked()
	return observerimpl.ScorerHistory{Available: true}
}

func (v scorerSnapshotStateView) ListDetectors() []observerimpl.ComponentStateInfo {
	v.checkLocked()
	return nil
}

type scorerSnapshotDebugView struct {
	anomalyAPIDebugView
	checkLocked func()
}

func (v scorerSnapshotDebugView) StorageReader() observerdef.StorageReader {
	v.checkLocked()
	return nil
}

func TestScoresSnapshotPreventsStorageReplacement(t *testing.T) {
	tb := &Bench{}
	checks := 0
	checkLocked := func() {
		t.Helper()
		checks++
		if tb.mu.TryLock() {
			tb.mu.Unlock()
			t.Fatal("scenario replacement must be blocked throughout the snapshot")
		}
	}
	tb.debug = scorerSnapshotDebugView{
		anomalyAPIDebugView: anomalyAPIDebugView{state: scorerSnapshotStateView{checkLocked: checkLocked}},
		checkLocked:         checkLocked,
	}

	recorder := httptest.NewRecorder()
	NewBenchAPI(tb).handleScores(recorder, httptest.NewRequest(http.MethodGet, "/api/scores", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 3, checks)
	require.True(t, tb.mu.TryLock(), "snapshot must release the lock")
	tb.mu.Unlock()
}

func TestScoresAPIUsesOriginalScorerHistory(t *testing.T) {
	anomaly := observerdef.Anomaly{
		Source:       observerdef.SeriesDescriptor{Name: "cpu.user"},
		SourceRef:    &observerdef.QueryHandle{Ref: 42, Aggregate: observerdef.AggregateAverage},
		DetectorName: "scanmw",
		Timestamp:    900,
	}
	history := observerimpl.ScorerHistory{
		Available: true,
		Buckets: []observerdef.AnomalyScoreBucket{
			{Second: 1000, Ewma: 0.1},
			{Second: 1001, Ewma: 0.5},
			{Second: 1002, Ewma: 0.2},
		},
		Events: []severityeventsdef.SeverityEvent{{
			Timestamp: 1001,
			FromLevel: severityeventsdef.SeverityLow,
			ToLevel:   severityeventsdef.SeverityHigh,
		}},
		Inputs: []observerimpl.ScorerHistoryInput{{
			ScoringSecond: 1001,
			Anomaly:       anomaly,
		}},
		Config:       observerimpl.DefaultAnomalyScorerConfig().AnomalyScorerConfig,
		CooldownSecs: 300,
	}
	history.Config.Alpha = 0.2
	history.Config.HighThreshold = 0.3
	history.Config.WindowSecs = 27
	history.Config.DetectorThresholds = map[string][4]float64{"scanmw": {1, 2, 3, 4}}
	history.CooldownSecs = 17
	state := anomalyAPIStateView{
		anomaly: anomaly,
		scorer:  history,
	}
	api := NewBenchAPI(&Bench{debug: anomalyAPIDebugView{state: state}})
	recorder := httptest.NewRecorder()
	api.handleScores(recorder, httptest.NewRequest(http.MethodGet, "/api/scores", nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	var response struct {
		Available    bool                              `json:"available"`
		Buckets      []observerdef.AnomalyScoreBucket  `json:"buckets"`
		Events       []severityeventsdef.SeverityEvent `json:"events"`
		CooldownSecs int64                             `json:"cooldown_secs"`
		Config       observerdef.AnomalyScorerConfig   `json:"config"`
		Inputs       []struct {
			ScoringSecond int64 `json:"scoringSecond"`
			Timestamp     int64 `json:"timestamp"`
		} `json:"inputs"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Available)
	require.Len(t, response.Buckets, 3)
	require.Equal(t, int64(1002), response.Buckets[2].Second)
	require.Len(t, response.Events, 1)
	require.Equal(t, history.CooldownSecs, response.CooldownSecs)
	require.Equal(t, history.Config, response.Config)
	require.Len(t, response.Inputs, 1)
	require.Equal(t, int64(1001), response.Inputs[0].ScoringSecond)
	require.Equal(t, int64(900), response.Inputs[0].Timestamp)
}

func TestAnomalyAPIEndpointsRenderTextAndKeepSourceMetadata(t *testing.T) {
	source := observerdef.SeriesDescriptor{Namespace: "ns", Name: "cpu.user", Host: "web-1", Tags: tagset.CompositeTagsFromSlice([]string{"env:prod"}), Aggregate: observerdef.AggregateAverage}
	anomaly := observerdef.Anomaly{
		Source: source, SourceRef: &observerdef.QueryHandle{Ref: 42, Aggregate: observerdef.AggregateAverage},
		DetectorName: "scanmw", Timestamp: 50,
		DebugInfo: &observerdef.AnomalyDebugInfo{BaselineMedian: 10, CurrentValue: 25, PValue: 1e-8, EffectSize: 0.85, DeviationSigma: 5},
	}
	state := anomalyAPIStateView{
		meta:        observerdef.SeriesMeta{Ref: 42, Namespace: source.Namespace, Name: source.Name, Host: source.Host, Tags: source.Tags},
		series:      observerdef.Series{Namespace: source.Namespace, Name: source.Name, Host: source.Host, Tags: source.Tags, Points: []observerdef.Point{{Timestamp: 50, Value: 25}}},
		anomaly:     anomaly,
		correlation: observerdef.ActiveCorrelation{Pattern: "p", Title: "correlation title", Members: []observerdef.SeriesDescriptor{source}, Anomalies: []observerdef.Anomaly{anomaly}, FirstSeen: 50, LastUpdated: 50},
	}
	tb := &Bench{debug: anomalyAPIDebugView{state: state}}
	api := NewBenchAPI(tb)
	wantTitle := "ScanMW changepoint: cpu.user:avg"
	wantDescription := "cpu.user:avg increased (pre_median=10.0000, post_median=25.0000, p=1.00e-08, effect=0.85, 5.0 MADs)"

	read := func(call func(http.ResponseWriter)) map[string]any {
		t.Helper()
		recorder := httptest.NewRecorder()
		call(recorder)
		require.Equal(t, http.StatusOK, recorder.Code)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &decoded))
		return decoded
	}

	for name, call := range map[string]func(http.ResponseWriter){
		"numeric marker": func(w http.ResponseWriter) { api.handleNumericSeriesData(w, 42, "avg", "42:avg") },
		"named marker": func(w http.ResponseWriter) {
			api.handleSeriesDataForSeries(w, "ns", "cpu.user:avg", "web-1", source.Tags.UnsafeToReadOnlySliceString(), "42:avg")
		},
	} {
		t.Run(name, func(t *testing.T) {
			response := read(call)
			marker := response["anomalies"].([]any)[0].(map[string]any)
			require.Equal(t, wantTitle, marker["title"])
			require.Equal(t, "42:avg", marker["sourceSeriesId"])
			require.Equal(t, "web-1", response["host"])
			require.Equal(t, []any{"env:prod"}, response["tags"])
		})
	}

	for name, call := range map[string]func(http.ResponseWriter){
		"anomaly": func(w http.ResponseWriter) {
			api.handleAnomalies(w, httptest.NewRequest(http.MethodGet, "/api/anomalies", nil))
		},
		"correlation": func(w http.ResponseWriter) {
			api.handleCorrelations(w, httptest.NewRequest(http.MethodGet, "/api/correlations", nil))
		},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			call(recorder)
			require.Equal(t, http.StatusOK, recorder.Code)
			var entries []map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &entries))
			require.Len(t, entries, 1)
			entry := entries[0]
			if name == "correlation" {
				require.Equal(t, "correlation title", entry["title"])
				entry = entry["anomalies"].([]any)[0].(map[string]any)
			}
			require.Equal(t, wantTitle, entry["title"])
			require.Equal(t, wantDescription, entry["description"])
			require.Equal(t, "web-1", entry["host"])
			require.Equal(t, []any{"env:prod"}, entry["tags"])
		})
	}

	logAnomaly := observerdef.Anomaly{Type: observerdef.AnomalyTypeLog, Source: observerdef.SeriesDescriptor{Name: "log.errors", Tags: tagset.CompositeTagsFromSlice([]string{"service:web"})}, DetectorName: "log_detector", Timestamp: 51}
	tb.logAnomalies = []observerdef.Anomaly{logAnomaly}
	recorder := httptest.NewRecorder()
	api.handleLogAnomalies(recorder, httptest.NewRequest(http.MethodGet, "/api/log-anomalies", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	var logEntries []map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &logEntries))
	require.Len(t, logEntries, 1)
	require.Equal(t, "Anomaly detected: log_detector: log.errors", logEntries[0]["title"])
	require.Equal(t, "", logEntries[0]["description"])
	require.Equal(t, []any{"service:web"}, logEntries[0]["tags"])
}

func TestSeriesAPIUsesStoredMetricTypeForEverySource(t *testing.T) {
	for _, namespace := range []string{"parquet", "check", "dogstatsd", "log_metrics_extractor"} {
		for _, metricType := range []metrics.MetricType{observerdef.UnknownType, metrics.GaugeType, metrics.CountType} {
			t.Run(namespace+"/"+metricType.String(), func(t *testing.T) {
				state := anomalyAPIStateView{meta: observerdef.SeriesMeta{
					Ref: 42, Namespace: namespace, Name: "requests", MetricType: metricType,
				}}
				api := NewBenchAPI(&Bench{debug: anomalyAPIDebugView{state: state}})
				response := httptest.NewRecorder()
				api.handleSeriesList(response, httptest.NewRequest(http.MethodGet, "/api/series", nil))
				require.Equal(t, http.StatusOK, response.Code)
				var series []struct {
					MetricTypes []string `json:"metricTypes"`
				}
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &series))
				require.Len(t, series, 2) // average and count views share the input type
				want := metricType.String()
				if metricType == observerdef.UnknownType {
					want = "Unknown"
				}
				for _, entry := range series {
					require.Equal(t, []string{want}, entry.MetricTypes)
				}
			})
		}
	}
}

func TestEffectiveScoringSecond(t *testing.T) {
	for _, tc := range []struct {
		name        string
		anomalyTS   int64
		prevData    int64
		dataTime    int64
		windowSecs  int64
		behavior    string
		wantScoring int64
		wantAction  string
	}{
		{"current", 505, 500, 510, 100, "clamp", 505, ""},
		{"detection clamp", 100, 500, 510, 0, "clamp", 501, "clamped"},
		{"reordered inside window", 450, 500, 510, 100, "clamp", 450, "reordered"},
		{"reordered clamped outside window", 100, 500, 510, 100, "clamp", 411, "clamped"},
		{"reordered ignored outside window", 100, 500, 510, 100, "ignore", 0, "ignored"},
		{"reordered inside window ignore", 450, 500, 510, 100, "ignore", 450, "reordered"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scoring, action := effectiveScoringSecond(tc.anomalyTS, tc.prevData, tc.dataTime, tc.windowSecs, tc.behavior)
			require.Equal(t, tc.wantScoring, scoring)
			require.Equal(t, tc.wantAction, action)
		})
	}
}

func TestNewDelayStats(t *testing.T) {
	require.Nil(t, newDelayStats(nil))
	require.Nil(t, newDelayStats([]int64{}))

	stats := newDelayStats([]int64{300, 0, 100, 200})
	require.Equal(t, 4, stats.Count)
	require.Equal(t, 3, stats.LateCount)
	require.Equal(t, int64(0), stats.MinSecs)
	require.Equal(t, int64(200), stats.MedianSecs)
	require.Equal(t, int64(300), stats.MaxSecs)
}

func TestReplayScoresFromArrivalsClamp(t *testing.T) {
	// One current anomaly at 505 and one late at 100, fed before Advance(510).
	anomalies := []observerdef.Anomaly{
		{Timestamp: 100, Source: observerdef.SeriesDescriptor{Name: "metric.late"}, DetectorName: "bocpd"},
		{Timestamp: 505, Source: observerdef.SeriesDescriptor{Name: "metric.current"}, DetectorName: "bocpd"},
	}
	history := observerimpl.ScorerHistory{
		Available: true,
		Inputs: []observerimpl.ScorerHistoryInput{
			{ScoringSecond: 411, Anomaly: anomalies[0]},
			{ScoringSecond: 505, Anomaly: anomalies[1]},
		},
		Arrivals: anomalies,
		Advances: []observerimpl.ScorerHistoryAdvance{
			{DataTime: 500, ArrivalEnd: 0},
			{DataTime: 510, ArrivalEnd: 2},
		},
	}
	bench := &Bench{debug: anomalyAPIDebugView{}}
	req := replayRequest{
		AnomalyScorerConfig: observerdef.AnomalyScorerConfig{
			Alpha: 0.5, SaturationK: 1, WindowSecs: 15,
			ReorderingWindowSecs: 100, LateAnomalyBehavior: "clamp",
		},
		CooldownSecs: 0,
	}
	resp := NewBenchAPI(bench).replayScoresFromArrivals(req, history, scorerReplayReordered)
	require.True(t, resp.Available)
	require.Equal(t, int64(100), resp.ReorderWindowSecs)
	require.True(t, resp.HistoryAvailable)
	require.Len(t, resp.Inputs, 2)
	require.Equal(t, int64(411), resp.Inputs[0].ScoringSecond)
	require.Equal(t, "clamped", resp.Inputs[0].LateAction)
	require.Equal(t, int64(505), resp.Inputs[1].ScoringSecond)
	require.Equal(t, "", resp.Inputs[1].LateAction)
	require.NotNil(t, resp.InputDelay)
	require.Equal(t, 2, resp.InputDelay.Count)
	require.Equal(t, int64(5), resp.InputDelay.MinSecs)
	require.Equal(t, int64(410), resp.InputDelay.MaxSecs)
	require.Equal(t, 1, resp.OutsideWindowCount)
}

func TestReplayScoresFromArrivalsUnavailable(t *testing.T) {
	// Arrival history missing: the mode is unavailable.
	state := anomalyAPIStateView{
		anomaly: observerdef.Anomaly{Timestamp: 505, Source: observerdef.SeriesDescriptor{Name: "metric.current"}, DetectorName: "bocpd"},
		scorer:  observerimpl.ScorerHistory{Available: false},
	}
	api := NewBenchAPI(&Bench{debug: anomalyAPIDebugView{state: state}})
	body := `{"alpha":0.5,"saturation_k":1,"window_secs":15,"cooldown_secs":0,"mode":"reordered"}`
	recorder := httptest.NewRecorder()
	api.handleScoresReplay(recorder, httptest.NewRequest(http.MethodPost, "/api/scores/replay", strings.NewReader(body)))
	require.Equal(t, http.StatusOK, recorder.Code)
	var resp scorerScoreResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.False(t, resp.Available)
	require.Equal(t, scorerReplayReordered, resp.Mode)
}
