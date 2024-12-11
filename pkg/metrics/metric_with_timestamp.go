// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package metrics

// MetricWithTimestamp allows sending metric values with a given timestamp
type MetricWithTimestamp struct {
	apiType APIMetricType
	points  []Point
}

// NewMetricWithTimestamp returns a new MetricWithTimestamp with the given API type
func NewMetricWithTimestamp(apiType APIMetricType) *MetricWithTimestamp {
	return &MetricWithTimestamp{apiType: apiType}
}

func (mt *MetricWithTimestamp) addSample(sample *MetricSample, timestamp float64) {
	mt.points = append(mt.points, Point{Ts: timestamp, Value: sample.Value})
}

func (mt *MetricWithTimestamp) flush(_ float64) ([]*Serie, error) {
	points := mt.points
	mt.points = nil

	if len(points) == 0 {
		return []*Serie{}, NoSerieError{}
	}

	return []*Serie{
		{
			Points: points,
			MType:  mt.apiType,
		},
	}, nil
}

func (mt *MetricWithTimestamp) isStateful() bool {
	return false
}
