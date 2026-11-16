// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && zlib

package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/impl"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

type testExplicitHistogram struct {
	metrics.DistributionMetadata
	points testExplicitPoints
}

func (h *testExplicitHistogram) GetName() string { return h.Name }

func (h *testExplicitHistogram) WriteTo(w metrics.DistributionWriter) error {
	return w.WriteOtelExplicitHistogram(h.DistributionMetadata, len(h.points), h.points)
}

type testExponentialHistogram struct {
	metrics.DistributionMetadata
	points testExponentialPoints
}

func (h *testExponentialHistogram) GetName() string { return h.Name }

func (h *testExponentialHistogram) WriteTo(w metrics.DistributionWriter) error {
	return w.WriteOtelExponentialHistogram(h.DistributionMetadata, len(h.points), h.points)
}

type distributionsSource struct {
	values []metrics.Distribution
	index  int
}

func (s *distributionsSource) MoveNext() bool {
	s.index++
	return s.index < len(s.values)
}
func (s *distributionsSource) Current() metrics.Distribution { return s.values[s.index] }
func (s *distributionsSource) Count() uint64                 { return uint64(len(s.values)) }
func (s *distributionsSource) WaitForValue() bool            { return true }

func otelHistogramsSource() *distributionsSource {
	return &distributionsSource{index: -1, values: []metrics.Distribution{
		&testExplicitHistogram{
			DistributionMetadata: metrics.DistributionMetadata{Name: "otel.explicit"},
			points:               testExplicitPoints{{ts: 10, cnt: 3, bounds: []float64{1}, counts: []uint64{1, 2}}},
		},
		Makeseries(0),
		&testExponentialHistogram{
			DistributionMetadata: metrics.DistributionMetadata{Name: "otel.exponential"},
			points:               testExponentialPoints{{ts: 10, cnt: 1, posCnt: []uint64{1}}},
		},
	}}
}

func TestSketchSeriesMarshalSplitCompressOtelHistograms(t *testing.T) {
	mockConfig := mock.New(t)
	mockConfig.SetInTest("serializer_compressor_kind", compression.ZlibKind)
	compressor := metricscompression.NewComponent(metricscompression.Requires{Cfg: mockConfig}).Comp

	t.Run("v3 writes all flavors", func(t *testing.T) {
		v3 := PipelineConfig{Filter: AllowAllFilter{}, V3: true}
		pipelines := PipelineSet{v3: {}}
		err := SketchSeriesList{SketchesSource: otelHistogramsSource()}.MarshalSplitCompressPipelines(mockConfig, compressor, pipelines, logmock.New(t))
		require.NoError(t, err)
		payloads := pipelines[v3].payloads
		require.Len(t, payloads, 1)
		require.Equal(t, len(Makeseries(0).Points)+2, payloads[0].GetPointCount())
	})

	t.Run("v2 rejects OTel histograms", func(t *testing.T) {
		pipelines := testPipelines()
		err := SketchSeriesList{SketchesSource: otelHistogramsSource()}.MarshalSplitCompressPipelines(mockConfig, compressor, pipelines, logmock.New(t))
		require.ErrorIs(t, err, errOtelUnsupported)
	})
}
