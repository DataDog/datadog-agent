// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && otlp

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	noopimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-noop"
)

func TestPayloadsBuilderV3_WriteSketchSkipsNativeHistograms(t *testing.T) {
	dp := pmetric.NewHistogramDataPoint()
	dp.ExplicitBounds().FromRaw([]float64{1, 5, 10})
	dp.BucketCounts().FromRaw([]uint64{1, 3, 5, 2})
	dp.SetCount(11)
	dp.SetSum(42.0)

	edp := pmetric.NewExponentialHistogramDataPoint()
	edp.SetScale(4)
	edp.SetZeroCount(5)
	edp.Positive().BucketCounts().FromRaw([]uint64{10, 20})
	edp.SetCount(35)

	pipelineConfig := PipelineConfig{
		Filter: AllowAllFilter{},
		V3:     true,
	}
	pipelineContext := &PipelineContext{}

	pb, err := newPayloadsBuilderV3(1000, 10000, 1000_0000, noopimpl.New(), pipelineConfig, pipelineContext)
	require.NoError(t, err)

	explicitSketch := &metrics.SketchSeries{
		Name: "native.explicit",
		Points: []metrics.SketchPoint{{
			Ts:     1000,
			Sketch: &metrics.ExplicitBoundHistogramPoint{Point: dp},
		}},
	}
	require.NoError(t, pb.writeSketch(explicitSketch))
	assert.Equal(t, 0, pb.pointsThisPayload, "explicit-bound histogram should be skipped")

	exponentialSketch := &metrics.SketchSeries{
		Name: "native.exponential",
		Points: []metrics.SketchPoint{{
			Ts:     1000,
			Sketch: &metrics.ExponentialHistogramPoint{Point: edp},
		}},
	}
	require.NoError(t, pb.writeSketch(exponentialSketch))
	assert.Equal(t, 0, pb.pointsThisPayload, "exponential histogram should be skipped")

	ddSketch := &metrics.SketchSeries{
		Name:   "dd.sketch",
		Points: pointsOf(1000, 1.0, 2.0),
	}
	require.NoError(t, pb.writeSketch(ddSketch))
	assert.Equal(t, 1, pb.pointsThisPayload, "DDSketch should be written normally")
}
