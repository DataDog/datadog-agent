// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"slices"
	"testing"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logsfilter"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// diverseLogContent returns distinct line shapes (JSON, kv, syslog, plain) for series s
// so neighboring series exercise different tokenizer / pattern signatures.
func diverseLogContent(s int) string {
	switch s % 20 {
	case 0:
		return fmt.Sprintf(`{"msg":"log from series %d","level":"info"}`, s)
	case 1:
		return fmt.Sprintf(`{"@timestamp":"2024-01-01T00:00:00Z","message":"evt-%d","severity":"WARN","svc":"api"}`, s)
	case 2:
		return fmt.Sprintf(`{"trace_id":"%08x","span_id":"%08x","msg":"child span","ok":true}`, s, s+1)
	case 3:
		return fmt.Sprintf(`{"nested":{"user":%d,"shard":3},"event":"login","ip":"10.0.0.%d"}`, s, s%255)
	case 4:
		return fmt.Sprintf(`level=INFO ts=1704067200 series=%d msg="request done" duration_ms=12`, s)
	case 5:
		return fmt.Sprintf(`level=ERROR logger=com.example req=%d stack=java.lang.Exception`, s)
	case 6:
		return fmt.Sprintf(`[2024-01-15 14:30:00] INFO  worker-%d  task=flush completed=true`, s)
	case 7:
		return fmt.Sprintf(`<134>1 2024-01-15T14:30:00Z host app-%d - - - msg="syslog style"`, s)
	case 8:
		return fmt.Sprintf(`10.1.2.3 - - [15/Jan/2024:14:30:00 +0000] "GET /api/v%d/items HTTP/1.1" 200 4321`, s%50)
	case 9:
		return fmt.Sprintf(`time="2024-01-15T14:30:00Z" level=debug msg="slow query" series=%d ms=450`, s)
	case 10:
		return fmt.Sprintf(`{"arr":[%d,2,3],"obj":{"k":"v"},"flag":false}`, s)
	case 11:
		return fmt.Sprintf(`ERROR: connection reset by peer series=%d errno=104`, s)
	case 12:
		return fmt.Sprintf(`{"msg":"unicode 测试 %d café","meta":{"region":"eu-west-1"}}`, s)
	case 13:
		return fmt.Sprintf(`kafka: topic=logs partition=%d offset=999 key=null`, s)
	case 14:
		return fmt.Sprintf(`{"a":1,"b":%d,"c":{"d":[1,2]}}`, s)
	case 15:
		return fmt.Sprintf(`[pid=12345] series=%d action=gc pause_ms=3`, s)
	case 16:
		return fmt.Sprintf(`{"http":{"method":"POST","path":"/hook/%d","status":201}}`, s)
	case 17:
		return fmt.Sprintf(`plain text line series=%d no json here`, s)
	case 18:
		return fmt.Sprintf(`{"double":%d.%d,"scientific":1.23e-4}`, s/10, s%10)
	case 19:
		return fmt.Sprintf(`merge key=value series=%d another=42`, s)
	default:
		return fmt.Sprintf(`{"msg":"log from series %d","level":"info"}`, s)
	}
}

// BenchmarkLogExtraction_SeriesCount measures raw log ingestion cost with
// the default catalog extractors across increasing series count.
func BenchmarkLogExtraction_SeriesCount(b *testing.B) {
	for _, numSeries := range []int{50, 200, 500, 2000} {
		numSeries := numSeries
		b.Run(fmt.Sprintf("series=%d", numSeries), func(b *testing.B) {
			logs := make([]*logObs, numSeries)
			for s := 0; s < numSeries; s++ {
				logs[s] = &logObs{
					content:     fmt.Sprintf(`{"msg":"log from series %d","level":"info"}`, s),
					status:      "error",
					tags:        []string{fmt.Sprintf("series:%d", s)},
					timestampMs: 0,
				}
			}

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				_, _, _, extractors, _ := defaultCatalog().Instantiate(benchmarkSettings)
				storage := newTimeSeriesStorage()
				e := newEngine(engineConfig{
					storage:    storage,
					extractors: extractors,
				})
				b.StartTimer()

				tsMs := int64(i) * 1000
				for _, l := range logs {
					l.timestampMs = tsMs
					e.IngestLog("ns", l)
				}
			}
		})
	}
}

// BenchmarkLogExtraction_DiversePatterns is like SeriesCount but assigns each series one
// of twenty distinct line shapes (JSON, structured kv, syslog, access log, plain text)
// so ingestion reflects higher pattern cardinality.
func BenchmarkLogExtraction_DiversePatterns(b *testing.B) {
	for _, numSeries := range []int{50, 200, 500, 2000} {
		numSeries := numSeries
		b.Run(fmt.Sprintf("series=%d", numSeries), func(b *testing.B) {
			logs := make([]*logObs, numSeries)
			for s := 0; s < numSeries; s++ {
				logs[s] = &logObs{
					content:     diverseLogContent(s),
					status:      "error",
					tags:        []string{fmt.Sprintf("series:%d", s)},
					timestampMs: 0,
				}
			}

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				_, _, _, extractors, _ := defaultCatalog().Instantiate(benchmarkSettings)
				storage := newTimeSeriesStorage()
				e := newEngine(engineConfig{
					storage:    storage,
					extractors: extractors,
				})
				b.StartTimer()

				tsMs := int64(i) * 1000
				for _, l := range logs {
					l.timestampMs = tsMs
					e.IngestLog("ns", l)
				}
			}
		})
	}
}

// BenchmarkLogContextSteadyState measures repeated writes to an existing
// log-derived series, so series creation does not hide per-log context costs.
func BenchmarkLogContextSteadyState(b *testing.B) {
	extractor := NewLogPatternExtractor(DefaultLogPatternExtractorConfig())
	extractor.config.MinClusterSizeBeforeEmit = 1
	e := newEngine(engineConfig{
		storage:    newTimeSeriesStorage(),
		extractors: []observerdef.LogMetricsExtractor{extractor},
	})
	log := &logObs{
		content:     "GET /users/123 returned 500",
		tags:        []string{"service:api"},
		timestampMs: 1_000,
	}
	e.IngestLog("logs", log)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log.timestampMs = int64(i+2) * 1_000
		e.IngestLog("logs", log)
	}
}

func highLoadLogTags() []string {
	return []string{
		"pod_name:api-123", "container_id:abc", "env:staging", "service:api",
		"kube_namespace:default", "kube_deployment:api", "image_name:api", "image_tag:v1",
		"cluster_name:stormeagle", "region:us-east-1", "team:agent", "version:7.84.0",
		"orchestrator:ecs", "container_name:api", "kube_replica_set:api-123", "short_image:api",
	}
}

func newBenchLogRules(b *testing.B, raw []logsfilter.ProcessingRule) *logsfilter.Rules {
	b.Helper()
	rules, err := logsfilter.NewRules(raw)
	if err != nil {
		b.Fatal(err)
	}
	return rules
}

// BenchmarkLogProducer_Handoff measures the producer-side cost of filtering and
// enqueueing a log observation across rule configurations and channel states.
func BenchmarkLogProducer_Handoff(b *testing.B) {
	noRules := newBenchLogRules(b, nil)
	sourceOnlyRejectRules := newBenchLogRules(b, []logsfilter.ProcessingRule{
		{Name: "drop_containerd", Type: "exclude_at_match", Source: "containerd"},
	})
	tagRules := newBenchLogRules(b, []logsfilter.ProcessingRule{
		{Name: "keep_prod_api", Type: "include_at_match", Source: "containerd", Tags: []string{"env:prod", "service:api"}},
		{Name: "keep_staging_web", Type: "include_at_match", Source: "containerd", Tags: []string{"env:staging", "service:web"}},
		{Name: "keep_staging_api", Type: "include_at_match", Source: "containerd", Tags: []string{"env:staging", "service:api", "team:agent"}},
		{Name: "drop_all", Type: "exclude_at_match"},
	})

	acceptedTags := highLoadLogTags()
	rejectedTags := slices.Clone(acceptedTags)
	rejectedTags[2] = "env:dev"

	for _, tc := range []struct {
		name        string
		rules       *logsfilter.Rules
		source      string
		tags        []string
		channelFull bool
	}{
		{name: "no_rules_16_tags", rules: noRules, source: "containerd", tags: acceptedTags},
		{name: "source_only_rejected_16_tags", rules: sourceOnlyRejectRules, source: "containerd", tags: acceptedTags},
		{name: "tag_accepted_16_tags", rules: tagRules, source: "containerd", tags: acceptedTags},
		{name: "tag_rejected_16_tags", rules: tagRules, source: "containerd", tags: rejectedTags},
		{name: "channel_full_16_tags", rules: noRules, source: "containerd", tags: acceptedTags, channelFull: true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			telemetryComp := telemetryimpl.NewMock(b)
			ch := make(chan observation, 1000)
			stopDrain := make(chan struct{})
			if tc.channelFull {
				for i := 0; i < cap(ch); i++ {
					ch <- observation{}
				}
			} else {
				go func() {
					for {
						select {
						case <-ch:
						case <-stopDrain:
							return
						}
					}
				}()
				b.Cleanup(func() { close(stopDrain) })
			}

			logFilter := &logAdmissionFilter{
				rules:      tc.rules,
				containers: newLogSourceRateState("info", -1, -1, -1),
			}
			h := &handle{
				ch:        ch,
				source:    "logs",
				telemetry: newObserverTelemetry(telemetryComp),
				logFilter: logFilter,
			}
			sample := &logObs{
				source:      tc.source,
				content:     `{"msg":"request completed","status":200}`,
				status:      "warn",
				tags:        tc.tags,
				hostname:    "host-1",
				timestampMs: 1_700_000_000_000,
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				h.ObserveLog(sample)
			}
		})
	}
}

// BenchmarkMixedMetricLogConsumer measures mixed metric and log ingestion
// through the shared observer channel and reports any queue drops.
func BenchmarkMixedMetricLogConsumer(b *testing.B) {
	cfg := configmock.NewFromYAML(b, `
anomaly_detection:
  reporting:
    events:
      enabled: true
  logs:
    internal:
      enabled: false
    processing_rules:
      - type: include_at_match
        name: keep_staging_api
        source: containerd
        tags: ["env:staging", "service:api"]
      - type: exclude_at_match
        name: drop_all
`)
	provides, err := NewComponent(Requires{
		Lifecycle: &testLifecycle{},
		Config:    cfg,
		Telemetry: telemetryimpl.NewMock(b),
	})
	if err != nil {
		b.Fatal(err)
	}
	obs := provides.Comp.(*observerImpl)
	b.Cleanup(func() { close(obs.obsCh) })

	metricHandle := obs.GetHandle("check")
	logHandle := obs.GetHandle("logs")

	metricSample := highLoadMetric("system.cpu.user")
	contextKey := testContextKeyFor(metricSample)
	logSample := &logObs{
		source:      "containerd",
		content:     `{"msg":"request failed","status":500}`,
		status:      "error",
		tags:        highLoadLogTags(),
		hostname:    "host-1",
		timestampMs: 1_700_000_000_000,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tsSec := int64(1_700_000_000 + i/64)
		metricSample.timestamp = tsSec
		metricHandle.ObserveMetric(metricSample, contextKey)

		logSample.timestampMs = tsSec * 1000
		logHandle.ObserveLog(logSample)
	}
	obs.Flush()
	totalDrops := metricHandle.(*handle).dropCount.Load() + logHandle.(*handle).dropCount.Load()
	b.ReportMetric(float64(totalDrops)/float64(b.N), "drops/op")
}
