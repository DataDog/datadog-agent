// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"fmt"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/require"
)

func TestApplicationSeverityIsNotTransportSeverity(t *testing.T) {
	for _, tc := range []struct {
		content, status, want string
		known                 bool
	}{
		{`{"level":"INFO","message":"error count is zero"}`, "error", "info", true},
		{`{"levelname":"ERROR","message":"write failed"}`, "info", "error", true},
		{`{"severity":"fatal"}`, "info", "fatal", true},
		{`2026-11-12 ERROR: write failed`, "info", "error", true},
		{`{"message":"ERROR: fake level inside message"}`, "error", "", false},
		{`routine unlevelled stderr`, "error", "", false},
		{`invalid json {`, "error", "", false},
	} {
		t.Run(tc.content, func(t *testing.T) {
			got, known := applicationLogSeverity(&logObs{content: tc.content, status: tc.status})
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.known, known)
		})
	}
}

func TestLogFailureColdStartRepetitionFreshnessAndRearm(t *testing.T) {
	d := NewLogFailureDetector()
	e := newErrorCountTestEngine(d)
	// Qualification/muting must not hide direct failures, even at observer startup.
	e.baseline = newBaselineController(BaselineConfig{Enabled: true, DurationSec: 600, MuteNoisyMetrics: true}, detectorNames(e.detectors))
	ingest := func(sec int64, n int) {
		for i := 0; i < n; i++ {
			e.IngestLog("logs", &logObs{timestampMs: sec * 1000, content: `{"level":"error"}`, tags: []string{"service:api"}})
		}
	}
	ingest(100, 10)
	require.Empty(t, e.Advance(104).anomalies, "one occupied interval is insufficient, regardless of volume")
	require.False(t, d.Ready())
	ingest(105, 1)
	a := e.Advance(109).anomalies
	require.Len(t, a, 1)
	require.Equal(t, observer.EvidenceFailure, a[0].Evidence)
	require.NotNil(t, a[0].SourceRef)
	require.Empty(t, e.Advance(110).anomalies, "no renewal without fresh errors")
	ingest(111, 1)
	require.Len(t, e.Advance(114).anomalies, 1, "fresh arrivals renew qualified evidence")
	require.Empty(t, e.Advance(150).anomalies, "old buckets expire")
	ingest(155, 10)
	require.Empty(t, e.Advance(159).anomalies, "old episode cannot qualify new isolated batch")
	ingest(160, 1)
	require.Len(t, e.Advance(164).anomalies, 1, "a later repeated failure rearms")
	refs := []observer.SeriesRef{a[0].SourceRef.Ref}
	d.RemoveSeries(refs)
	require.Empty(t, d.lastPositive)
	d.Reset()
	require.False(t, d.Ready())
	require.Empty(t, d.lastPositive)
}

func TestLogFailureDoesNotCombineServices(t *testing.T) {
	d := NewLogFailureDetector()
	e := newErrorCountTestEngine(d)
	for i, service := range []string{"a", "b", "c"} {
		e.IngestLog("logs", &logObs{timestampMs: int64(100+i*5) * 1000, content: `{"level":"ERROR"}`, tags: []string{"service:" + service}})
		require.Empty(t, e.Advance(int64(104+i*5)).anomalies)
	}
}

func TestLogFailureExperimentConfig(t *testing.T) {
	for name, parse := range map[string]func(map[string]json.RawMessage) (ComponentSettings, error){
		"core": ParseSettingsFromJSON, "replay": ParseTestbenchSettingsFromJSON,
	} {
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				raw  string
				want LogFailureConfig
			}{
				{`{"enabled":true}`, DefaultLogFailureConfig()},
				{`{"enabled":true,"min_errors":5}`, LogFailureConfig{MinErrors: 5, MinIntervals: 2, WindowSeconds: 30}},
				{`{"enabled":true,"min_errors":4,"min_intervals":3,"window_seconds":15}`, LogFailureConfig{MinErrors: 4, MinIntervals: 3, WindowSeconds: 15}},
			} {
				settings, err := parse(map[string]json.RawMessage{LogFailureDetectorName: json.RawMessage(tc.raw)})
				require.NoError(t, err)
				_, _, _, extractors, components, err := defaultCatalog().Instantiate(settings)
				require.NoError(t, err)
				require.Equal(t, tc.want, components[LogFailureDetectorName].instance.(*LogFailureDetector).config)
				require.NotNil(t, errorCountBucketizer(extractors))
			}
		})
	}
	// Agent settings continue to use the evaluated rule without new tuning keys.
	catalog := defaultCatalog()
	_, _, _, _, components, err := catalog.Instantiate(settingsFromAgentConfig(catalog, configmock.New(t)))
	require.NoError(t, err)
	require.Equal(t, DefaultLogFailureConfig(), components[LogFailureDetectorName].instance.(*LogFailureDetector).config)
	for _, raw := range []string{
		`{"min_errors":0}`, `{"min_errors":-1}`, `{"min_errors":1.5}`, `{"min_errors":"3"}`, `{"min_errors":null}`,
		`{"min_intervals":0}`, `{"min_intervals":7}`, `{"window_seconds":0}`, `{"window_seconds":31}`, `{"window_seconds":305}`,
		`{"min_error":3}`, `{"bucket_seconds":10}`, `null`,
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := ParseSettingsFromJSON(map[string]json.RawMessage{LogFailureDetectorName: json.RawMessage(raw)})
			require.Error(t, err)
		})
	}
	_, _, _, _, _, err = catalog.Instantiate(ComponentSettings{configs: map[string]any{LogFailureDetectorName: LogFailureConfig{}}})
	require.Error(t, err, "programmatic settings also validate before constructing the detector")
}

func TestLogFailureAgentOptInAndRequiredExtractor(t *testing.T) {
	catalog := defaultCatalog()
	settings := settingsFromAgentConfig(catalog, configmock.NewFromYAML(t, `
anomaly_detection:
  detectors:
    log_failure:
      enabled: true
`))
	for _, pipeline := range []bool{false, true} {
		settings.UseMetricPipeline = pipeline
		detectors, _, _, extractors, _, err := catalog.Instantiate(settings)
		require.NoError(t, err)
		require.True(t, hasLogFailureDetector(detectors))
		require.NotNil(t, errorCountBucketizer(extractors))
		require.NotContains(t, settings.Enabled, LogErrorCountExtractorName, "dependency resolution must not mutate caller settings")
	}
	settings.Enabled[LogErrorCountExtractorName] = false
	_, _, _, _, _, err := catalog.Instantiate(settings)
	require.ErrorContains(t, err, "explicit disable")
}

func TestLogFailureConfiguredQualificationWindowAndReset(t *testing.T) {
	cfg := LogFailureConfig{MinErrors: 4, MinIntervals: 3, WindowSeconds: 15}
	d := newLogFailureDetector(cfg)
	e := newErrorCountTestEngine(d)
	for _, step := range []struct {
		sec    int64
		errors int
		want   int
	}{
		{100, 3, 0},
		{105, 1, 0}, // Enough errors, but only two intervals.
		{110, 1, 1}, // Five errors across three intervals qualify.
		{115, 1, 0}, // The first interval expires; three errors remain.
	} {
		for i := 0; i < step.errors; i++ {
			e.IngestLog("logs", &logObs{timestampMs: step.sec * 1000, content: `{"level":"ERROR"}`, tags: []string{"service:api"}})
		}
		anomalies := e.Advance(step.sec + 4).anomalies
		require.Len(t, anomalies, step.want)
		if len(anomalies) > 0 {
			require.Equal(t, float64(5), anomalies[0].DebugInfo.CurrentValue)
			require.Equal(t, float64(4), anomalies[0].DebugInfo.Threshold)
			require.Equal(t, "repeated_application_errors", anomalies[0].DebugInfo.FailureRule)
			_, description := observer.FormatAnomaly(anomalies[0])
			require.Contains(t, description, "at least 3 five-second intervals in the last 15 seconds")
		}
	}
	d.Reset()
	require.Equal(t, cfg, d.config)
	require.Empty(t, d.lastPositive)
	require.Empty(t, d.lastRead)
	require.False(t, d.Ready())
	require.Empty(t, d.Detect(e.storage, 119, observer.DetectionFilter{}).Anomalies, "reset must retain configured thresholds")
}

func TestLogFailureConfiguredWindowStorageRetention(t *testing.T) {
	settings, err := ParseSettingsFromJSON(map[string]json.RawMessage{
		"bocpd":                json.RawMessage(`{"enabled":false}`),
		LogFailureDetectorName: json.RawMessage(`{"enabled":true,"min_errors":60,"min_intervals":60,"window_seconds":300}`),
	})
	require.NoError(t, err)
	storageCfg, err := StorageConfigForComponents(settings)
	require.NoError(t, err)
	require.Equal(t, 60, storageCfg.MaxPointsPerSeries)
	detectors, _, _, extractors, _, err := defaultCatalog().Instantiate(settings)
	require.NoError(t, err)
	e := mustNewEngine(engineConfig{storage: newTimeSeriesStorageWith(storageCfg), detectors: detectors, extractors: extractors})
	for i := 0; i < 60; i++ {
		sec := int64(100 + i*5)
		e.IngestLog("logs", &logObs{timestampMs: sec * 1000, content: `{"level":"ERROR"}`})
		anomalies := e.Advance(sec + 4).anomalies
		if i < 59 {
			require.Empty(t, anomalies)
		} else {
			require.Len(t, anomalies, 1, "configured window must remain available under bounded production storage")
		}
	}
}

func TestLogFailureStorageReplayParityAndAggregationScope(t *testing.T) {
	liveDetector := NewLogFailureDetector()
	live := newErrorCountTestEngine(liveDetector)
	retained := newErrorCountTestEngine()
	var want []observer.Anomaly
	for sec := int64(100); sec < 130; sec += 5 {
		for i := 0; i < 3; i++ {
			l := &logObs{timestampMs: sec * 1000, content: `{"level":"error"}`, tags: []string{"service:api"}}
			live.IngestLog("logs", l)
			retained.IngestLog("logs", l)
		}
		want = append(want, live.Advance(sec+4).anomalies...)
		retained.Advance(sec + 4)
	}
	replay := NewLogFailureDetector()
	var got []observer.Anomaly
	for sec := int64(104); sec < 134; sec += 5 {
		got = append(got, replay.Detect(retained.storage, sec, observer.DetectionFilter{}).Anomalies...)
	}
	require.NotEmpty(t, want)
	require.Equal(t, want, got, "preloaded future buckets cannot affect an earlier decision")
	countOnly := NewLogFailureDetector()
	require.Empty(t, countOnly.Detect(retained.storage, 129, observer.DetectionFilter{Aggregations: []observer.Aggregate{observer.AggregateCount}}).Anomalies)
	require.False(t, countOnly.Ready())
	other := newTimeSeriesStorage()
	other.Add("application", logErrorCountMetric, 3, 104, nil)
	other.Add("application", logErrorCountMetric, 3, 109, nil)
	require.Empty(t, NewLogFailureDetector().Detect(other, 109, observer.DetectionFilter{}).Anomalies, "ordinary metrics cannot impersonate extracted errors")
}

func BenchmarkApplicationLogSeverity(b *testing.B) {
	for _, input := range []struct{ name, body string }{
		{"json", `{"level":"error","message":"request failed","service":"api","timestamp":"2026-11-12T12:00:00Z","request_id":"1234","status_code":503}`},
		{"text", "2026-11-12T12:00:00Z ERROR: request failed"},
		{"unknown", "routine container stderr output without application severity"},
	} {
		b.Run(input.name, func(b *testing.B) {
			l := &logObs{content: input.body, status: "error"}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				applicationLogSeverity(l)
			}
		})
	}
}

func BenchmarkLogFailureAdvance256(b *testing.B) {
	s := newTimeSeriesStorage()
	for i := 0; i < 256; i++ {
		tags := []string{fmt.Sprintf("service:s%d", i)}
		for sec := int64(104); sec <= 129; sec += 5 {
			s.Add(LogErrorCountExtractorName, logErrorCountMetric, 1, sec, tags)
		}
	}
	d := NewLogFailureDetector()
	d.Detect(s, 129, observer.DetectionFilter{})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Detect(s, 129, observer.DetectionFilter{})
	}
}

func BenchmarkLogFailureFresh256(b *testing.B) {
	d := NewLogFailureDetector()
	s := newTimeSeriesStorage()
	for i := 0; i < 256; i++ {
		for sec := int64(100); sec < 130; sec += 5 {
			s.Add(LogErrorCountExtractorName, logErrorCountMetric, 1, sec, []string{fmt.Sprintf("service:%d", i)})
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Exercise qualification and emission while retaining populated storage and caches.
		clear(d.lastPositive)
		clear(d.lastRead)
		d.Detect(s, 129, observer.DetectionFilter{})
	}
}

func TestDirectLogFailureDoesNotAllocateStatisticalErrorModels(t *testing.T) {
	b := NewBOCPDDetector(BOCPDConfig{WarmupPoints: 2})
	e := newErrorCountTestEngine(b, NewLogFailureDetector())
	for sec := int64(100); sec < 160; sec += 5 {
		e.storage.Add("check", "ordinary", 10, sec, nil)
		e.IngestLog("logs", &logObs{timestampMs: sec * 1000, content: `{"level":"ERROR"}`, tags: []string{"service:api"}})
		e.Advance(sec + 4)
	}
	require.NotEmpty(t, b.series, "ordinary metrics must still be modeled")
	for key := range b.series {
		require.Equal(t, "check", e.storage.GetSeriesMeta(key.ref).Namespace)
	}
	require.Len(t, b.cachedRefs, 1)
}
