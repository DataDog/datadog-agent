// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package metrics

import (
	"math"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics/percentile"
	"github.com/stretchr/testify/assert"
)

func TestContextSketchSampling(t *testing.T) {
	ctxSketch := MakeContextSketch()
	contextKey, _ := ckey.Parse("aaffffffffffffffffffffffffffffff")

	ctxSketch.AddSample(contextKey, &MetricSample{Value: 1, Mtype: DistributionType}, 1, 10)
	ctxSketch.AddSample(contextKey, &MetricSample{Value: 5, Mtype: DistributionType}, 3, 10)
	resultSeries := ctxSketch.Flush(12345.0)

	expectedSketch := percentile.NewGKArray()
	expectedSketch = expectedSketch.Add(1)
	expectedSketch = expectedSketch.Add(5)
	expectedSeries := &percentile.SketchSeries{
		ContextKey: contextKey,
		Sketches:   []percentile.Sketch{{Timestamp: int64(12345), Sketch: expectedSketch}},
	}

	assert.Equal(t, 1, len(resultSeries))
	AssertSketchSeriesEqual(t, expectedSeries, resultSeries[0])

	// No sketches should be flushed when there's no new sample since
	// last flush
	resultSeries = ctxSketch.Flush(12355.0)
	assert.Equal(t, 0, len(resultSeries))
}

// The sketches ignore sample values of +Inf/-Inf/NaN
func TestContextSketchSamplingInfinity(t *testing.T) {
	ctxSketch := MakeContextSketch()
	contextKey, _ := ckey.Parse("ffffffffffffffffffffffffffffffff")

	ctxSketch.AddSample(contextKey, &MetricSample{Value: math.Inf(1), Mtype: DistributionType}, 1, 10)
	ctxSketch.AddSample(contextKey, &MetricSample{Value: math.Inf(-1), Mtype: DistributionType}, 2, 10)
	ctxSketch.AddSample(contextKey, &MetricSample{Value: math.NaN(), Mtype: DistributionType}, 3, 10)
	resultSeries := ctxSketch.Flush(12345.0)

	assert.Equal(t, 0, len(resultSeries))
}
