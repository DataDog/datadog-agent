// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

// Gauge tracks the value of a metric
type Gauge struct {
	gauge   float64
	sampled bool
}

//nolint:revive // TODO(AML) Fix revive linter
func (g *Gauge) addSample(sample *MetricSample, _ float64) {
	g.gauge = sample.Value
	g.sampled = true
}

func (g *Gauge) flush(timestamp float64) ([]*Serie, error) {
	value, sampled := g.gauge, g.sampled
	g.gauge, g.sampled = 0, false

	if !sampled {
		return []*Serie{}, NoSerieError{}
	}

	return []*Serie{
		{
			// we use the timestamp passed to the flush
			Points: []Point{{Ts: timestamp, Value: value}},
			MType:  APIGaugeType,
		},
	}, nil
}

func (g *Gauge) isStateful() bool {
	return false
}
