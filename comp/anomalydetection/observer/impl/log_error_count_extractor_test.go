// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"context"
	"fmt"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/require"
)

func TestLogErrorCountSeverity(t *testing.T) {
	for status, want := range map[string]float64{
		"error": 1, " ERROR ": 1, "critical": 1, "fatal": 1, "alert": 1, "emergency": 1,
		"warn": 0, "info": 0, "debug": 0, "trace": 0,
	} {
		t.Run(status, func(t *testing.T) {
			out := (&LogErrorCountExtractor{}).ProcessLog(&logObs{source: "datadog-agent",
				status: status, content: "error in the text does not determine severity",
				tags: []string{"service:api", "env:prod", "container_id:123", "source:java", "level:" + status},
			})
			require.Len(t, out.Metrics, 1)
			require.Equal(t, want, out.Metrics[0].Value)
			require.True(t, out.Metrics[0].Tags.Find(func(tag string) bool { return tag == "service:api" }))
			require.True(t, out.Metrics[0].Tags.Find(func(tag string) bool { return tag == "env:prod" }))
			require.Equal(t, 2, out.Metrics[0].Tags.Len())
		})
	}
}

func newErrorCountTestEngine(detectors ...observerdef.Detector) *engine {
	registrations := make([]detectorRegistration, len(detectors))
	for i, detector := range detectors {
		registrations[i] = detectorRegistration{detector: detector}
	}
	return mustNewEngine(engineConfig{
		storage:    newTimeSeriesStorageWith(StorageConfig{}),
		extractors: []observerdef.LogMetricsExtractor{&LogErrorCountExtractor{}},
		detectors:  registrations,
	})
}

func errorCountTestSeries(e *engine, service string) *observerdef.Series {
	return e.storage.GetSeries(LogErrorCountExtractorName, logErrorCountMetric,
		[]string{"observer_source:logs", "service:" + service}, observerdef.AggregateAverage)
}

func TestLogErrorCountPoolsPatternsBeforeFirstError(t *testing.T) {
	e := newErrorCountTestEngine()
	// Pattern bucketization remains disabled. Container and pattern changes
	// must not create fresh error-count series or reset their histories.
	for _, input := range []struct {
		sec                        int64
		status, content, container string
	}{
		{100, "info", "started", "a"}, {103, "info", "ready", "a"},
		{105, "info", "served request", "b"}, {108, "warn", "retrying", "b"},
		{110, "error", "first-ever failed write", "c"}, {111, "fatal", "new fatal pattern", "d"},
	} {
		e.IngestLog("logs", &logObs{source: "datadog-agent", timestampMs: input.sec * 1000, status: input.status,
			content: input.content, tags: []string{"service:api", "container_id:" + input.container}})
		e.Advance(input.sec - 1)
	}
	e.Advance(114)
	series := errorCountTestSeries(e, "api")
	require.NotNil(t, series)
	require.Equal(t, []observerdef.Point{{Timestamp: 104, Value: 0}, {Timestamp: 109, Value: 0}, {Timestamp: 114, Value: 2}}, series.Points)
	meta := e.storage.ListSeries(observerdef.WorkloadSeriesFilter())
	require.Len(t, meta, 1)
	require.True(t, e.storage.SupportsAggregate(meta[0].Ref, observerdef.AggregateAverage))
	require.False(t, e.storage.SupportsAggregate(meta[0].Ref, observerdef.AggregateCount))
}

func TestLogErrorCountMissingIntervalsAndServiceIsolation(t *testing.T) {
	e := newErrorCountTestEngine()
	e.IngestLog("logs", &logObs{source: "datadog-agent", timestampMs: 101_000, status: "info", tags: []string{"service:api"}})
	e.IngestLog("logs", &logObs{source: "datadog-agent", timestampMs: 102_000, status: "error", tags: []string{"service:worker"}})
	e.Advance(104)
	// Other services or metric activity cannot assert this service is healthy.
	e.Advance(199)
	e.IngestLog("logs", &logObs{source: "datadog-agent", timestampMs: 201_000, status: "error", tags: []string{"service:api"}})
	e.Advance(204)
	require.Equal(t, []observerdef.Point{{Timestamp: 104, Value: 0}, {Timestamp: 204, Value: 1}}, errorCountTestSeries(e, "api").Points)
	require.Equal(t, []observerdef.Point{{Timestamp: 104, Value: 1}}, errorCountTestSeries(e, "worker").Points)
	// Completed history must never be rewritten by late errors.
	e.IngestLog("logs", &logObs{source: "datadog-agent", timestampMs: 102_000, status: "error", tags: []string{"service:api"}})
	require.Equal(t, int64(1), e.latePoints.Load())
	require.Empty(t, e.errorCounts.series)
}

func TestLogErrorCountDoesNotInheritPatternIdleZeros(t *testing.T) {
	e := mustNewEngine(engineConfig{
		storage:         newTimeSeriesStorageWith(StorageConfig{}),
		extractors:      []observerdef.LogMetricsExtractor{&LogErrorCountExtractor{}, &fixedLogCountExtractor{}},
		logCountBuckets: LogCountBucketConfig{Enabled: true, BucketSeconds: 10, IdleTTLSeconds: 60},
	})
	e.IngestLog("logs", &logObs{source: "datadog-agent", timestampMs: 101_000, status: "info", tags: []string{"service:api"}})
	e.Advance(170)
	require.Equal(t, []observerdef.Point{{Timestamp: 104, Value: 0}}, errorCountTestSeries(e, "api").Points)
	patterns := e.storage.GetSeries("fixed_log_count", "log.fixed.count",
		[]string{"observer_source:logs", "service:api"}, observerdef.AggregateAverage)
	require.Greater(t, len(patterns.Points), 1, "existing pattern idle-zero behavior is preserved")
}

func TestLogErrorCountPendingBoundsAndReset(t *testing.T) {
	e := newErrorCountTestEngine()
	telComp := telemetryimpl.NewMock(t)
	tel := newObserverTelemetry(telComp)
	e.onErrorCountCapacityHit = tel.recordErrorCountCapacityHit
	for i := 0; i < 300; i++ {
		e.IngestLog("logs", &logObs{source: "datadog-agent", status: "info", timestampMs: 100_000, tags: []string{fmt.Sprintf("service:s%d", i)}})
	}
	require.Len(t, e.errorCounts.series, 256)
	require.Equal(t, 44.0, observerMetric(t, telComp, telemetryErrorCountCapacityDropped, nil).GetCounter().GetValue())
	for sec := int64(105); sec < 200; sec += 5 {
		e.IngestLog("logs", &logObs{source: "datadog-agent", status: "info", timestampMs: sec * 1000, tags: []string{"service:s0"}})
	}
	for _, state := range e.errorCounts.series {
		require.LessOrEqual(t, len(state.values), 2)
	}
	e.Reset()
	require.Empty(t, e.errorCounts.series)
	require.NoError(t, e.SetExtractors(nil))
	require.Nil(t, e.errorCounts)
	require.NoError(t, e.SetExtractors([]observerdef.LogMetricsExtractor{&LogErrorCountExtractor{}}))
	require.NotNil(t, e.errorCounts)
}

func TestLogErrorCountFollowsLogFailure(t *testing.T) {
	_, _, _, extractors, _, err := defaultCatalog().Instantiate(ComponentSettings{})
	require.NoError(t, err)
	require.Nil(t, errorCountBucketizer(extractors))
	catalog := defaultCatalog()
	for _, tc := range []struct {
		yaml string
		want bool
	}{
		{"log_error_count_extractor:\n      enabled: true", false},
		{"log_failure:\n      enabled: true\n    log_error_count_extractor:\n      enabled: false", true},
	} {
		cfg := configmock.NewFromYAML(t, "anomaly_detection:\n  detectors:\n    "+tc.yaml+"\n")
		settings, err := settingsFromAgentConfig(catalog, cfg)
		require.NoError(t, err)
		_, _, _, extractors, _, err := catalog.Instantiate(settings)
		require.NoError(t, err, "an explicit extractor toggle never blocks startup")
		require.Equal(t, tc.want, errorCountBucketizer(extractors) != nil, "the extractor runs exactly when log_failure does")
	}
}

func TestLogFailureDoesNotActivateWithoutConsumer(t *testing.T) {
	cfg := configmock.NewFromYAML(t, `
anomaly_detection:
  detectors:
    log_failure:
      enabled: true
`)
	telComp := telemetryimpl.NewMock(t)
	provides, err := NewComponent(Requires{
		Lifecycle: &testLifecycle{}, Config: cfg, Telemetry: telComp,
	})
	require.NoError(t, err)
	require.IsType(t, &noopObserveHandle{}, provides.Comp.GetHandle("logs"))
	requireNoObserverMetricFamilies(t, telComp)
}

func TestLogFailureRespectsHandleAdmissionAndApplicationLevel(t *testing.T) {
	for _, tc := range []struct {
		name, minimum, status, level string
		want                         bool
	}{
		{"stdout_error_admitted", "info", "info", "ERROR", true},
		// Container stdout arrives as info; the declared level admits it at the default minimum.
		{"stdout_error_admitted_at_default_minimum", "warn", "info", "ERROR", true},
		{"minimum_above_error_still_filters", "critical", "info", "ERROR", false},
		{"stderr_info_is_not_failure", "warn", "error", "INFO", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.NewFromYAML(t, `
anomaly_detection:
  reporting:
    events:
      enabled: true
  logs:
    internal:
      enabled: false
    containers:
      min_severity: `+tc.minimum+`
      max_rate_low_priority: -1
      max_rate_high_priority: -1
  detectors:
    bocpd:
      enabled: false
    log_pattern_extractor:
      enabled: false
    log_metrics_extractor:
      enabled: false
    log_failure:
      enabled: true
`)
			lc := &testLifecycle{}
			provides, err := NewComponent(Requires{Lifecycle: lc, Config: cfg, Telemetry: telemetryimpl.NewMock(t)})
			require.NoError(t, err)
			obs := provides.Comp.(*observerImpl)
			h := obs.GetHandle("logs")
			for _, sec := range []int64{100, 100, 105, 110} {
				h.ObserveLog(&logObs{source: "docker", status: tc.status, content: `{"level":"` + tc.level + `"}`,
					timestampMs: sec * 1000, tags: []string{"service:api"}})
			}
			require.Len(t, lc.hooks, 1)
			require.NoError(t, lc.hooks[0].OnStop(context.Background()))
			found := false
			for _, registration := range obs.engine.detectors {
				if registration.detector.Name() == LogFailureDetectorName {
					found = true
					require.Equal(t, tc.want, registration.detector.Ready())
				}
			}
			require.True(t, found)
		})
	}
}

func TestDeclaredErrorAdmissionRequiresErrorCountExtraction(t *testing.T) {
	logsCh := make(chan observation, 4)
	filter := &logAdmissionFilter{
		kubelet:    newLogSourceRateState("warn", -1, -1, -1),
		containers: newLogSourceRateState("warn", -1, -1, -1),
	}
	h := &handle{logsCh: logsCh, source: "logs", logFilter: filter}
	observe := func(content string) {
		h.ObserveLog(&logObs{source: "docker", status: "info", content: content, tags: []string{"service:api"}})
	}

	observe(`{"level":"error"}`)
	require.Empty(t, logsCh, "without error-count extraction the transport minimum applies unchanged")

	filter.admitDeclaredErrors = true
	observe(`{"level":"info","msg":"error budget healthy"}`)
	observe(`2026-11-16 10:00:00 INFO request served`)
	require.Empty(t, logsCh, "declared non-error output stays below the minimum")
	observe(`2026-11-16 10:00:00.123 ERROR [main] write failed`)
	require.Len(t, logsCh, 1)
	admitted := (<-logsCh).log
	require.Equal(t, "error", admitted.status, "admission and rate limiting use the declared level")
	observe(`{"severity":50,"msg":"write failed"}`)
	require.Len(t, logsCh, 1, "numeric levels under any supported key pass the pre-check")
	require.Equal(t, "error", (<-logsCh).log.status)
}

func TestLogErrorCountHostEnvironmentAndIngressIsolation(t *testing.T) {
	e := newErrorCountTestEngine()
	for _, input := range []struct{ host, env, source string }{
		{"host-a", "prod", "logs"}, {"host-b", "prod", "logs"},
		{"host-a", "staging", "logs"}, {"host-a", "prod", "agent_logs"},
	} {
		e.IngestLog(input.source, &logObs{source: "datadog-agent", timestampMs: 100_000, hostname: input.host,
			status: "error", tags: []string{"service:api", "env:" + input.env}})
	}
	e.Advance(104)
	series := e.storage.AllSeries(LogErrorCountExtractorName, observerdef.AggregateAverage)
	require.Len(t, series, 4)
	for _, s := range series {
		require.Equal(t, []observerdef.Point{{Timestamp: 104, Value: 1}}, s.Points)
	}
}

func TestLogErrorCountEvictionAndReplayReset(t *testing.T) {
	e := newErrorCountTestEngine()
	e.storage.cfg.InactiveSeriesTTLSeconds = 10
	e.storage.cfg.InactiveSeriesCheckIntervalSeconds = 1
	e.IngestLog("logs", &logObs{source: "datadog-agent", timestampMs: 100_000, status: "error", tags: []string{"service:api"}})
	e.Advance(104)
	require.NotNil(t, errorCountTestSeries(e, "api"))
	e.Advance(120)
	require.Nil(t, errorCountTestSeries(e, "api"))
	require.Empty(t, e.errorCounts.series)
	// Replaying earlier timestamps must not inherit a previous run's watermark.
	require.NoError(t, e.ResetForReplay(nil, nil, nil, []observerdef.LogMetricsExtractor{&LogErrorCountExtractor{}}, StorageConfig{}, BaselineConfig{}))
	e.IngestLog("logs", &logObs{source: "datadog-agent", timestampMs: 100_000, status: "info", tags: []string{"service:api"}})
	e.Advance(104)
	require.Equal(t, []observerdef.Point{{Timestamp: 104, Value: 0}}, errorCountTestSeries(e, "api").Points)
}

func BenchmarkLogErrorCountIngest(b *testing.B) {
	e := newErrorCountTestEngine()
	l := &logObs{source: "datadog-agent", status: "error", tags: []string{"service:api", "container_id:123"}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sec := int64(100 + i/100)
		l.timestampMs = sec * 1000
		e.IngestLog("logs", l)
		if i%100 == 0 {
			e.Advance(sec - 1)
		}
	}
}
