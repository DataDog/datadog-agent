// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package metrics

// GaugeWithTimestamp tracks the value of a metric with a given timestamp
type GaugeWithTimestamp struct {
	points  []Point
	sampled bool
}

func (g *GaugeWithTimestamp) addSample(sample *MetricSample, timestamp float64) {
	g.points = append(g.points, Point{Ts: timestamp, Value: sample.Value})
	g.sampled = true
}

func (g *GaugeWithTimestamp) flush(_ float64) ([]*Serie, error) {
	points, sampled := g.points, g.sampled
	g.points, g.sampled = nil, false

	if !sampled {
		return []*Serie{}, NoSerieError{}
	}

	return []*Serie{
		{
			Points: points,
			MType:  APIGaugeType,
		},
	}, nil
}

func (g *GaugeWithTimestamp) isStateful() bool {
	return false
}
