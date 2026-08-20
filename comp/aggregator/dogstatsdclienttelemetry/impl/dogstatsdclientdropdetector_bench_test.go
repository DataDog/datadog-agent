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

	dogstatsdclientdropdetector "github.com/DataDog/datadog-agent/comp/aggregator/dogstatsdclientdropdetector/def"
	dogstatsdclientdropdetectorimpl "github.com/DataDog/datadog-agent/comp/aggregator/dogstatsdclientdropdetector/impl"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

type benchmarkFinalSerieObserver interface {
	ObserveFinalDogStatsDSerie(serie *metrics.Serie)
}

type benchmarkLifecycle struct{}

func (*benchmarkLifecycle) Append(compdef.Hook) {}

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

func newBenchmarkDetector(b *testing.B) dogstatsdclientdropdetector.Component {
	hostname, _ := hostnamemock.NewMock(hostnamemock.MockHostname("benchmark-host"))
	return dogstatsdclientdropdetectorimpl.NewComponent(dogstatsdclientdropdetectorimpl.Requires{
		Lifecycle:      &benchmarkLifecycle{},
		Config:         config.NewMock(b),
		Log:            logmock.New(b),
		Hostname:       hostname,
		HealthPlatform: healthplatformmock.New(b),
	}).Comp
}

func benchmarkFinalDogStatsDSeries(name string) ([]*metrics.Serie, string, float64) {
	series := make([]*metrics.Serie, finalDogStatsDClientTelemetrySeriesPerOperation)
	metricName := "bytes_sent"
	expected := float64(finalDogStatsDClientTelemetrySeriesPerOperation)

	for i := range series {
		serieName := name
		tags := tagset.CompositeTagsFromSlice([]string{"client_transport:uds", dogStatsDClientLibraryTagPrefix + "go"})
		if name == "unmatched-distinct-metric-names" {
			serieName = fmt.Sprintf("customer.metric.%d", i)
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

// BenchmarkComponentFinalDogStatsDSerieObserverWithDropDetector_1MFinalSeries
// compares the COAT-only observer with the combined COAT and drop-detection path.
func BenchmarkComponentFinalDogStatsDSerieObserverWithDropDetector_1MFinalSeries(b *testing.B) {
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
					detector := newBenchmarkDetector(b)
					provides := NewComponent(Requires{Telemetry: telemetry, DropDetector: detector})
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
						if mode.detectDrops {
							detector.CompleteFinalDogStatsDSerieFlush()
						}
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
