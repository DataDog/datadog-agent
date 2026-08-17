// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package dogstatsdclienttelemetryimpl

import (
	"fmt"
	"math"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

const finalDogStatsDClientTelemetrySeriesPerOperation = 1_000_000

type benchmarkFinalSerieObserver interface {
	ObserveFinalDogStatsDSerie(serie *metrics.Serie)
	CompleteFinalDogStatsDSerieFlush()
}

// coatOnlyBenchmarkObserver preserves the observer behavior from before drop
// detection so the benchmark can compare both paths without a production-only
// nullable detector state.
type coatOnlyBenchmarkObserver struct {
	bytesSent          telemetry.Counter
	bytesDropped       telemetry.Counter
	bytesDroppedQueue  telemetry.Counter
	bytesDroppedWriter telemetry.Counter
}

func (o *coatOnlyBenchmarkObserver) ObserveFinalDogStatsDSerie(serie *metrics.Serie) {
	if serie.MType != metrics.APIRateType || serie.Interval <= 0 {
		return
	}

	var counter telemetry.Counter
	switch serie.Name {
	case dogStatsDClientBytesSentMetric:
		counter = o.bytesSent
	case dogStatsDClientBytesDroppedMetric:
		counter = o.bytesDropped
	case dogStatsDClientBytesDroppedQueueMetric:
		counter = o.bytesDroppedQueue
	case dogStatsDClientBytesDroppedWriterMetric:
		counter = o.bytesDroppedWriter
	default:
		return
	}

	client, transport := clientTelemetryTags(serie)
	for _, point := range serie.Points {
		bytes := point.Value * float64(serie.Interval)
		if !(bytes >= 0 && bytes < math.MaxUint64) {
			continue
		}
		counter.Add(bytes, client, transport)
	}
}

func (*coatOnlyBenchmarkObserver) CompleteFinalDogStatsDSerieFlush() {}

func benchmarkFinalDogStatsDSeries(name string) ([]*metrics.Serie, string, float64) {
	series := make([]*metrics.Serie, finalDogStatsDClientTelemetrySeriesPerOperation)
	metricName := "bytes_sent"
	expected := float64(finalDogStatsDClientTelemetrySeriesPerOperation)

	for i := range series {
		serieName := name
		tags := tagset.CompositeTagsFromSlice([]string{dogStatsDClientUDSTransportTag, dogStatsDClientLibraryTagPrefix + "go"})
		if name == "unmatched-distinct-metric-names" {
			serieName = fmt.Sprintf("customer.metric.%d", i)
			metricName = "bytes_sent"
			expected = 0
			tags = tagset.CompositeTags{}
		}
		series[i] = &metrics.Serie{
			Name:     serieName,
			Host:     "host-a",
			Tags:     tags,
			MType:    metrics.APIRateType,
			Interval: 1,
			Points:   []metrics.Point{{Value: 1}},
		}
	}

	if name == dogStatsDClientBytesDroppedWriterMetric {
		metricName = "bytes_dropped_writer"
	}
	return series, metricName, expected
}

// BenchmarkComponentFinalDogStatsDSerieObserver_1MFinalSeries compares the
// COAT-only observer with the combined COAT and drop-detection path. It
// uses one million distinct ordinary counters for the normal path and one
// million matching client-telemetry counters for the worst case.
func BenchmarkComponentFinalDogStatsDSerieObserver_1MFinalSeries(b *testing.B) {
	for _, input := range []struct {
		name      string
		serieName string
	}{
		{name: "unmatched-distinct-metric-names", serieName: "unmatched-distinct-metric-names"},
		{name: "matching-bytes-sent", serieName: dogStatsDClientBytesSentMetric},
		{name: "matching-bytes-dropped-writer", serieName: dogStatsDClientBytesDroppedWriterMetric},
	} {
		b.Run(input.name, func(b *testing.B) {
			series, metricName, expected := benchmarkFinalDogStatsDSeries(input.serieName)
			for _, mode := range []struct {
				name        string
				detectDrops bool
			}{
				{name: "coat-only"},
				{name: "coat-and-drop-detection", detectDrops: true},
			} {
				b.Run(mode.name, func(b *testing.B) {
					telemetry := telemetrymock.New(b)
					provides, _ := newTestComponent(b, telemetry)
					component := provides.Observer.(*component)
					var observer benchmarkFinalSerieObserver = component
					if !mode.detectDrops {
						observer = &coatOnlyBenchmarkObserver{
							bytesSent:          component.bytesSent,
							bytesDropped:       component.bytesDropped,
							bytesDroppedQueue:  component.bytesDroppedQueue,
							bytesDroppedWriter: component.bytesDroppedWriter,
						}
					}

					b.ReportAllocs()
					b.ReportMetric(finalDogStatsDClientTelemetrySeriesPerOperation, "final-series/op")
					b.ResetTimer()
					for n := 0; n < b.N; n++ {
						for _, serie := range series {
							observer.ObserveFinalDogStatsDSerie(serie)
						}
						observer.CompleteFinalDogStatsDSerieFlush()
					}
					b.StopTimer()
					b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*finalDogStatsDClientTelemetrySeriesPerOperation), "ns/final-series")
					countMetrics, err := telemetry.GetCountMetric("dogstatsd_client", metricName)
					if expected == 0 {
						require.Error(b, err)
						return
					}
					require.NoError(b, err)
					require.Len(b, countMetrics, 1)
					require.Equal(b, expected*float64(b.N), countMetrics[0].Value())
					runtime.KeepAlive(series)
					runtime.KeepAlive(component)
				})
			}
		})
	}
}
