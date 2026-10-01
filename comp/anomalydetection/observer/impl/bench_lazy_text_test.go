// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

var (
	benchmarkLazyTextAnomalySink observer.Anomaly
	benchmarkLazyTextTitleSink   string
	benchmarkLazyTextDetailSink  string
)

// BenchmarkLazyAnomalyText measures the complete ScanMW detection call with
// reusable input. The control fixture must not emit; the step fixture must emit.
// Selected output renders one anomaly in sixteen, matching a scorer that only
// displays a small subset of detector output.
func BenchmarkLazyAnomalyText(b *testing.B) {
	for _, tc := range []struct {
		name        string
		step        bool
		renderEvery int
	}{
		{name: "no-anomalies", step: false},
		{name: "output-disabled", step: true},
		{name: "selected-output", step: true, renderEvery: 16},
		{name: "all-output", step: true, renderEvery: 1},
	} {
		b.Run(tc.name, func(b *testing.B) {
			points := make([]observer.Point, 40)
			for i := range points {
				value := 50.0
				if tc.step && i >= 20 {
					value = 200
				}
				points[i] = observer.Point{Timestamp: int64(i + 1), Value: value}
			}
			series := observer.Series{Namespace: "ns", Name: "metric", Points: points}
			detector := NewScanMWDetector()
			_, _, found := detector.scanMW(points, &series, observer.AggregateAverage)
			if found != tc.step {
				b.Fatalf("fixture emitted=%v, want %v", found, tc.step)
			}

			b.ReportAllocs()
			b.ResetTimer()
			emitted := 0
			for i := 0; i < b.N; i++ {
				anomaly, _, found := detector.scanMW(points, &series, observer.AggregateAverage)
				benchmarkLazyTextAnomalySink = anomaly
				if found {
					emitted++
					if tc.renderEvery > 0 && i%tc.renderEvery == 0 {
						benchmarkLazyTextTitleSink, benchmarkLazyTextDetailSink = observer.FormatAnomaly(anomaly)
					}
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(emitted)/float64(b.N), "anomalies/op")
		})
	}
}
