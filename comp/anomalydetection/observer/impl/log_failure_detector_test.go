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
		{`{"Level":"Error","msg":"write failed"}`, "info", "error", true},
		{`{"lvl":"warn"}`, "error", "warn", true},
		{`{"level":50,"msg":"pino error"}`, "info", "error", true},
		{`{"level":30,"msg":"pino info"}`, "error", "info", true},
		{`{"level":3,"msg":"unknown numeric scale"}`, "error", "", false},
		{`2026-11-16 10:00:00.123 ERROR [main] c.e.OrderService - write failed`, "info", "error", true},
		{`2026-11-16T10:00:00.000Z  INFO 1 --- [main] o.s.b.Application : Started`, "error", "info", true},
		{`time=2026-11-16T10:00:00Z level=error msg="write failed"`, "info", "error", true},
		{`ts=1 lvl=INFO msg=ok`, "error", "info", true},
		{`level=unknown msg="no recognized level"`, "error", "", false},
		{`[ERROR] 2026-11-16 write failed`, "info", "error", true},
		{`2026/11/16 10:00:00 [error] 12#12: upstream timed out`, "info", "error", true},
		{`E1116 10:00:00.123456       1 controller.go:42] sync failed`, "info", "error", true},
		{`I1116 10:00:00.123456       1 controller.go:42] synced`, "error", "info", true},
		{`retrying after error in upstream call`, "error", "", false},
		{`INFO: received ERROR response from upstream`, "error", "info", true},
		{`ERROR: retrying at INFO level`, "info", "error", true},
		{`{"severity":50,"msg":"write failed"}`, "info", "error", true},
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
	require.Equal(t, defaultLogFailureWeight, a[0].FailureWeight, "evidence carries the detector's scorer weight")
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
	require.NotContains(t, d.lastRead, refs[0])
	d.Reset()
	require.False(t, d.Ready())
	require.Zero(t, d.checkedUpTo)
}

// Analysis advances only when input arrives, so a burst followed by silence is
// first visible to the detector well after its last bucket completed.
func TestLogFailureReportsBurstFirstSeenAfterQuietGap(t *testing.T) {
	for _, tc := range []struct {
		name      string
		advanceTo int64
	}{
		{"next input shortly after", 115},
		{"next input after the window", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newErrorCountTestEngine(NewLogFailureDetector())
			require.Empty(t, e.Advance(99).anomalies, "the detector has run before the burst, as in live analysis")
			for _, sec := range []int64{100, 100, 105} {
				e.IngestLog("logs", &logObs{timestampMs: sec * 1000, content: `{"level":"error"}`, tags: []string{"service:api"}})
			}
			a := e.Advance(tc.advanceTo).anomalies
			require.Len(t, a, 1)
			require.Equal(t, int64(109), a[0].Timestamp, "evidence keeps the time of the qualifying bucket")
			require.Equal(t, 3.0, a[0].DebugInfo.CurrentValue)
			require.Empty(t, e.Advance(tc.advanceTo+10).anomalies, "a bucket is reported once")
		})
	}
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
				{`{"enabled":true,"failure_weight":0}`, LogFailureConfig{FailureWeight: 0}},
				{`{"enabled":true,"failure_weight":6}`, LogFailureConfig{FailureWeight: 6}},
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
	for _, raw := range []string{
		`{"failure_weight":-1}`, `{"failure_weight":6.5}`, `{"failure_weight":"3"}`, `{"failure_weight":null}`, `null`,
		`{"min_errors":3}`, `{"window_seconds":30}`,
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := ParseSettingsFromJSON(map[string]json.RawMessage{LogFailureDetectorName: json.RawMessage(raw)})
			require.Error(t, err, "invalid weights and the fixed thresholds are rejected")
		})
	}
	catalog := defaultCatalog()
	for _, tc := range []struct {
		yaml string
		want float64
	}{
		{"enabled: true", defaultLogFailureWeight}, {"failure_weight: 6", 6}, {"failure_weight: 0", 0},
		{"failure_weight: -1", defaultLogFailureWeight}, {"failure_weight: 6.5", defaultLogFailureWeight},
	} {
		cfg := configmock.NewFromYAML(t, "anomaly_detection:\n  detectors:\n    log_failure:\n      "+tc.yaml+"\n")
		_, _, _, _, components, err := catalog.Instantiate(settingsFromAgentConfig(catalog, cfg))
		require.NoError(t, err, "an invalid Agent weight falls back instead of blocking startup")
		require.Equal(t, tc.want, components[LogFailureDetectorName].instance.(*LogFailureDetector).config.FailureWeight, tc.yaml)
	}
	_, _, _, _, _, err := catalog.Instantiate(ComponentSettings{configs: map[string]any{LogFailureDetectorName: LogFailureConfig{FailureWeight: 7}}})
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
	_, _, _, extractors, _, err := catalog.Instantiate(settings)
	require.NoError(t, err)
	require.NotNil(t, errorCountBucketizer(extractors), "an explicit extractor disable is ignored")
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
		d.checkedUpTo = 0
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
