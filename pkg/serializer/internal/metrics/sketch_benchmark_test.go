// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

//go:build test

package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/impl"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

func benchmarkSplitPayloadsSketchesNew(b *testing.B, numPoints int) {
	testSketchSeries := metrics.NewSketchesSourceTest()
	for i := 0; i < numPoints; i++ {
		testSketchSeries.Append(Makeseries(200))
	}
	serializer := SketchSeriesList{SketchesSource: testSketchSeries}
	b.ReportAllocs()
	b.ResetTimer()
	mockConfig := mock.New(b)
	compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
	logger := logmock.New(b)

	for n := 0; n < b.N; n++ {
		payloads, err := serializer.MarshalSplitCompress(marshaler.NewBufferContext(), mockConfig, compressor, logger)
		require.NoError(b, err)
		var pb int
		for _, p := range payloads {
			pb += p.Len()
		}
		b.ReportMetric(float64(pb), "payload-bytes")
		b.ReportMetric(float64(len(payloads)), "payloads")
	}
}

func BenchmarkMarshalSplitCompress1(b *testing.B)     { benchmarkSplitPayloadsSketchesNew(b, 1) }
func BenchmarkMarshalSplitCompress10(b *testing.B)    { benchmarkSplitPayloadsSketchesNew(b, 10) }
func BenchmarkMarshalSplitCompress100(b *testing.B)   { benchmarkSplitPayloadsSketchesNew(b, 100) }
func BenchmarkMarshalSplitCompress1000(b *testing.B)  { benchmarkSplitPayloadsSketchesNew(b, 1000) }
func BenchmarkMarshalSplitCompress10000(b *testing.B) { benchmarkSplitPayloadsSketchesNew(b, 10000) }
