// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math/rand"
	"testing"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

// buildSyntheticStorage creates a storage pre-populated with numSeries series,
// each with numSeconds data points. The last third of the data has a step-change
// to give detectors a realistic signal shape during warm-up.
func buildSyntheticStorage(numSeries, numSeconds int) *timeSeriesStorage {
	rng := rand.New(rand.NewSource(42))
	storage := newTimeSeriesStorage()
	for sec := int64(0); sec < int64(numSeconds); sec++ {
		for s := 0; s < numSeries; s++ {
			name := fmt.Sprintf("metric_%d", s)
			value := 100.0 + rng.Float64()*10
			if sec > int64(numSeconds*2/3) {
				value = 200.0 + rng.Float64()*10
			}
			storage.Add("ns", name, value, sec, nil)
		}
	}
	return storage
}

// BenchmarkIngestion_SeriesCount ramps the number of series, measuring raw write cost.
func BenchmarkIngestion_SeriesCount(b *testing.B) {
	for _, numSeries := range []int{50, 200, 500, 2000} {
		numSeries := numSeries
		b.Run(fmt.Sprintf("series=%d", numSeries), func(b *testing.B) {
			rng := rand.New(rand.NewSource(42))
			obs := make([]*metricObs, numSeries)
			for s := 0; s < numSeries; s++ {
				obs[s] = &metricObs{
					name:      fmt.Sprintf("metric_%d", s),
					value:     100.0 + rng.Float64()*10,
					timestamp: 0,
				}
			}

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				storage := newTimeSeriesStorage()
				e := newEngine(engineConfig{storage: storage})
				b.StartTimer()

				for _, o := range obs {
					o.timestamp = int64(i)
					e.IngestMetric("ns", o)
				}
			}
		})
	}
}

// BenchmarkStorage_BucketCounts measures the allocation cost of retaining a
// 30-second window when buckets contain either one sample (the common metric
// path) or repeated same-second samples that require explicit counts.
func BenchmarkStorage_BucketCounts(b *testing.B) {
	const (
		numSeries  = 200
		numSeconds = 30
	)
	names := make([]string, numSeries)
	for i := range names {
		names[i] = fmt.Sprintf("metric_%d", i)
	}

	for _, samplesPerBucket := range []int{1, 4} {
		b.Run(fmt.Sprintf("samples=%d", samplesPerBucket), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				storage := newTimeSeriesStorageWith(StorageConfig{PointRetentionSecs: numSeconds})
				for sec := int64(0); sec < numSeconds; sec++ {
					for series, name := range names {
						for sample := 0; sample < samplesPerBucket; sample++ {
							storage.Add("ns", name, float64(series+sample), sec, nil)
						}
					}
				}
				if got := storage.TotalSeriesCount(); got != numSeries {
					b.Fatalf("series count = %d, want %d", got, numSeries)
				}
			}
		})
	}
}

func BenchmarkMetricFilterV1Rules(b *testing.B) {
	filter := newV1MetricFilter(b)
	for _, tc := range []struct {
		name         string
		metricName   string
		wantRejected bool
	}{
		{name: "rejected_unmatched_16_tags", metricName: "kubernetes.pod.count", wantRejected: true},
		{name: "accepted_first_rule_16_tags", metricName: "system.cpu.user"},
		{name: "accepted_last_rule_16_tags", metricName: "container.net.rcvd"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			sample := highLoadMetric(tc.metricName)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				decision := prepareMetricIngest("check", sample, filter)
				if gotRejected := decision.metric == nil; gotRejected != tc.wantRejected {
					b.Fatalf("rejected=%t, want %t", gotRejected, tc.wantRejected)
				}
			}
		})
	}
}

func BenchmarkHandleObserveMetricV1RulesParallelRejectedMetric(b *testing.B) {
	telemetryComp := telemetryimpl.GetCompatComponent()
	telemetryComp.Reset()
	b.Cleanup(telemetryComp.Reset)

	h := &handle{
		source:    "check",
		filter:    newV1MetricFilter(b),
		telemetry: newObserverTelemetry(telemetryComp),
	}
	sample := highLoadMetric("kubernetes.pod.count")
	if h.ObserveMetricAndReportDrop(sample) {
		b.Fatal("expected metric to be rejected by processing rules")
	}

	b.ReportAllocs()
	b.SetParallelism(4)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if h.ObserveMetricAndReportDrop(sample) {
				panic("expected metric to be rejected by processing rules")
			}
		}
	})
}

func newV1MetricFilter(b *testing.B) *metricsFilterRules {
	b.Helper()

	rules := make([]metricsProcessingRule, 0, 16)
	for _, name := range []string{
		"system.cpu.user",
		"system.cpu.system",
		"system.cpu.iowait",
		"system.load.norm.1",
		"system.mem.pct_usable",
		"system.io.r_await",
		"system.io.w_await",
		"system.io.util",
		"container.cpu.usage",
		"container.cpu.throttled",
		"container.memory.working_set",
		"container.memory.oom_events",
		"container.io.partial_stall",
		"container.net.sent",
		"container.net.rcvd",
	} {
		rules = append(rules, metricsProcessingRule{
			Type:        includeAtMatch,
			Name:        "keep_" + name,
			NamePattern: name,
		})
	}
	rules = append(rules, metricsProcessingRule{
		Type:        excludeAtMatch,
		Name:        "drop_everything_else",
		NamePattern: "*",
	})

	filter, err := newMetricsFilterRules(rules)
	if err != nil {
		b.Fatal(err)
	}
	return filter
}

func highLoadMetric(name string) *metricObs {
	return &metricObs{
		name:      name,
		timestamp: 1000,
		tags: []string{
			"pod_name:api-123", "container_id:abc", "env:staging", "service:api",
			"kube_namespace:default", "kube_deployment:api", "image_name:api", "image_tag:v1",
			"cluster_name:stormeagle", "region:us-east-1", "team:agent", "version:7.84.0",
			"orchestrator:ecs", "container_name:api", "kube_replica_set:api-123", "short_image:api",
		},
	}
}
