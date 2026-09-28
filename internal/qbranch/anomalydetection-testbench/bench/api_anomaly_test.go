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
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/stretchr/testify/require"
)

type anomalyAPIDebugView struct {
	observerimpl.DebugView
	state observerimpl.StateView
}

func (v anomalyAPIDebugView) StateView() observerimpl.StateView { return v.state }

type anomalyAPIStateView struct {
	observerimpl.StateView
	meta        observerdef.SeriesMeta
	series      observerdef.Series
	anomaly     observerdef.Anomaly
	correlation observerdef.ActiveCorrelation
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

func TestAnomalyAPIEndpointsRenderTextAndKeepSourceMetadata(t *testing.T) {
	source := observerdef.SeriesDescriptor{Namespace: "ns", Name: "cpu.user", Host: "web-1", Tags: tagset.CompositeTagsFromSlice([]string{"env:prod"}), Aggregate: observerdef.AggregateAverage}
	anomaly := observerdef.Anomaly{
		Source: source, SourceRef: &observerdef.QueryHandle{Ref: 42, Aggregate: observerdef.AggregateAverage},
		DetectorName: "scanmw", Timestamp: 50, Title: "stale title", Description: "stale description",
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
