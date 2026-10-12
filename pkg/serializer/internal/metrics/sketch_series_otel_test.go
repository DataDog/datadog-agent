// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && zlib

package metrics

import (
	"testing"

	"github.com/DataDog/agent-payload/v5/gogen"
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

func TestSketchSeriesMarshalSplitCompressOtelHistograms(t *testing.T) {
	mockConfig := mock.New(t)
	mockConfig.SetInTest("serializer_compressor_kind", compression.ZlibKind)
	compressor := metricscompression.NewComponent(metricscompression.Requires{Cfg: mockConfig}).Comp

	ddsketch := Makeseries(0)
	source := &distributionsSource{index: -1, values: []metrics.Distribution{
		&testExplicitHistogram{
			DistributionMetadata: metrics.DistributionMetadata{Name: "otel.explicit"},
			points:               testExplicitPoints{{ts: 10, cnt: 3, bounds: []float64{1}, counts: []uint64{1, 2}}},
		},
		ddsketch,
		&testExponentialHistogram{
			DistributionMetadata: metrics.DistributionMetadata{Name: "otel.exponential"},
			points:               testExponentialPoints{{ts: 10, cnt: 1, posCnt: []uint64{1}}},
		},
	}}

	v2 := PipelineConfig{Filter: AllowAllFilter{}, V3: false}
	v3 := PipelineConfig{Filter: AllowAllFilter{}, V3: true}
	pipelines := PipelineSet{v2: {}, v3: {}}

	err := SketchSeriesList{SketchesSource: source}.MarshalSplitCompressPipelines(mockConfig, compressor, pipelines, logmock.New(t))
	require.NoError(t, err)

	// V2 skips the OTel histograms and keeps the DDSketch series.
	v2Payloads := pipelines[v2].payloads
	require.Len(t, v2Payloads, 1)
	require.Equal(t, len(ddsketch.Points), v2Payloads[0].GetPointCount())
	decompressed, err := compressor.Decompress(v2Payloads[0].GetContent())
	require.NoError(t, err)
	pl := new(gogen.SketchPayload)
	require.NoError(t, pl.Unmarshal(decompressed))
	require.Len(t, pl.Sketches, 1)
	require.Equal(t, ddsketch.Name, pl.Sketches[0].Metric)

	// V3 writes all three series.
	v3Payloads := pipelines[v3].payloads
	require.Len(t, v3Payloads, 1)
	require.Equal(t, len(ddsketch.Points)+2, v3Payloads[0].GetPointCount())
}
