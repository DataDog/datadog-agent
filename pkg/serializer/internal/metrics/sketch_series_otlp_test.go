// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && otlp && zlib && zstd

package metrics

import (
	"testing"

	"github.com/DataDog/agent-payload/v5/gogen"
	"go.opentelemetry.io/collector/pdata/pmetric"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/impl"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/compression"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSketchSeriesMarshalSplitCompressSkipsNativeHistograms(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}
	logger := logmock.New(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := mock.New(t)
			mockConfig.SetInTest("serializer_compressor_kind", tc.kind)

			dp := pmetric.NewHistogramDataPoint()
			dp.ExplicitBounds().FromRaw([]float64{1, 5, 10})
			dp.BucketCounts().FromRaw([]uint64{1, 3, 5, 2})
			dp.SetCount(11)

			sl := metrics.NewSketchesSourceTest()
			sl.Append(&metrics.SketchSeries{
				Name: "native.histogram",
				Tags: tagset.CompositeTagsFromSlice([]string{"env:test"}),
				Host: "testhost",
				Points: []metrics.SketchPoint{{
					Ts:     1000,
					Sketch: &metrics.ExplicitBoundHistogramPoint{Point: dp},
				}},
			})
			sl.Append(Makeseries(0))

			pipelines := testPipelines()
			serializer := SketchSeriesList{SketchesSource: sl}
			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
			err := serializer.MarshalSplitCompressPipelines(mockConfig, compressor, pipelines, logger)
			require.NoError(t, err)
			payloads := pipelines.GetPayloads()

			firstPayload := payloads[0]
			decompressed, _ := compressor.Decompress(firstPayload.GetContent())

			pl := new(gogen.SketchPayload)
			require.NoError(t, pl.Unmarshal(decompressed))

			require.Len(t, pl.Sketches, 1, "native histogram should be skipped, only DDSketch remains")
			assert.Equal(t, "name.0", pl.Sketches[0].Metric)
		})
	}
}
