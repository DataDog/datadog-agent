// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ringbuffer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

func TestSketchBufferRetainsContextPerPoint(t *testing.T) {
	buffer := NewSketchBuffer(Options{Capacity: 2, ShardCount: 1})
	source := Source{Kind: SourceDogStatsDBucketed}

	require.NoError(t, buffer.AppendSketchSeries(context.Background(), source, &metrics.SketchSeries{
		DistributionMetadata: metrics.DistributionMetadata{Name: "dist.a"},
		Points: []metrics.SketchPoint{
			{Ts: 10, Sketch: testRetainedSketchData(1)},
			{Ts: 11, Sketch: testRetainedSketchData(2)},
		},
	}))
	require.NoError(t, buffer.AppendSketchSeries(context.Background(), source, &metrics.SketchSeries{
		DistributionMetadata: metrics.DistributionMetadata{Name: "dist.b"},
		Points:               []metrics.SketchPoint{{Ts: 12, Sketch: testRetainedSketchData(3)}},
	}))

	series := buffer.SketchSeriesBetween(time.Time{}, time.Time{})
	require.Len(t, series, 2)
	require.Equal(t, "dist.a", series[0].Name)
	require.Equal(t, int64(11), series[0].Points[0].Ts)
	require.Equal(t, "dist.b", series[1].Name)
	require.Equal(t, int64(12), series[1].Points[0].Ts)
}

func testRetainedSketchData(value float64) *quantile.Sketch {
	agent := quantile.Agent{}
	agent.Insert(value, 1)
	return agent.Finish()
}
