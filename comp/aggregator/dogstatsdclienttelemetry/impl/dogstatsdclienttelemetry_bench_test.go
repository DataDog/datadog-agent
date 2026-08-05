// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package dogstatsdclienttelemetryimpl

import (
	"testing"

	"github.com/stretchr/testify/require"

	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const finalDogStatsDClientTelemetrySeriesPerOperation = 1_000_000

// BenchmarkComponentFinalDogStatsDSerieObserver_1MFinalSeries measures the
// real client telemetry observer for final normal-aggregation series.
func BenchmarkComponentFinalDogStatsDSerieObserver_1MFinalSeries(b *testing.B) {
	for _, benchmark := range []struct {
		name      string
		serieName string
		expected  float64
	}{
		{
			name:      "matching-bytes-sent",
			serieName: dogStatsDClientBytesSentMetric,
			expected:  finalDogStatsDClientTelemetrySeriesPerOperation,
		},
		{
			name:      "unmatched-metric-name",
			serieName: "datadog.dogstatsd.client.unmatched",
		},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			telemetry := telemetrymock.New(b)
			provides := NewComponent(Requires{Telemetry: telemetry})
			series := make([]*metrics.Serie, finalDogStatsDClientTelemetrySeriesPerOperation)
			for i := range series {
				series[i] = &metrics.Serie{
					Name:     benchmark.serieName,
					MType:    metrics.APIRateType,
					Interval: 1,
					Points:   []metrics.Point{{Value: 1}},
				}
			}

			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				for _, serie := range series {
					provides.Observer.ObserveFinalDogStatsDSerie(serie)
				}
			}
			b.StopTimer()

			countMetrics, err := telemetry.GetCountMetric("dogstatsd_client", "bytes_sent")
			require.NoError(b, err)
			require.Len(b, countMetrics, 1)
			require.Equal(b, benchmark.expected*float64(b.N), countMetrics[0].Value())
			b.ReportMetric(float64(finalDogStatsDClientTelemetrySeriesPerOperation), "final-series/op")
		})
	}
}
