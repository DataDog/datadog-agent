// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package metrics

// CountWithTimestamp tracks the value of a metric with a given timestamp
type CountWithTimestamp struct {
	points []Point
}

func (g *CountWithTimestamp) addSample(sample *MetricSample, timestamp float64) {
	g.points = append(g.points, Point{Ts: timestamp, Value: sample.Value})
}

func (g *CountWithTimestamp) flush(_ float64) ([]*Serie, error) {
	points := g.points
	g.points = nil

	if len(points) == 0 {
		return []*Serie{}, NoSerieError{}
	}

	return []*Serie{
		{
			Points: points,
			MType:  APICountType,
		},
	}, nil
}

func (g *CountWithTimestamp) isStateful() bool {
	return false
}
