// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

// Set tracks the number of unique elements in a set. They are only use by
// DogStatsD
type Set struct {
	values map[string]bool
}

// NewSet return a new initialized Set
func NewSet() *Set {
	return &Set{values: make(map[string]bool)}
}

//nolint:revive // TODO(AML) Fix revive linter
func (s *Set) addSample(sample *MetricSample, _ float64) {
	s.values[sample.RawValue] = true
}

func (s *Set) flush(timestamp float64) ([]*Serie, error) {
	if len(s.values) == 0 {
		return []*Serie{}, NoSerieError{}
	}

	res := []*Serie{
		{
			// we use the timestamp passed to the flush
			Points: []Point{{Ts: timestamp, Value: float64(len(s.values))}},
			MType:  APIGaugeType,
		},
	}

	s.values = make(map[string]bool)
	return res, nil
}

func (s *Set) isStateful() bool {
	return false
}
