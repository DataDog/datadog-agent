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

func (s *Set) flush(timestamp float64, out []*Serie) ([]*Serie, error) {
	if len(s.values) == 0 {
		return out, NoSerieError{}
	}

	serie := GetSerie()
	serie.Points = append(serie.Points[:0], Point{Ts: timestamp, Value: float64(len(s.values))})
	serie.MType = APIGaugeType

	s.values = make(map[string]bool)
	return append(out, serie), nil
}

func (s *Set) isStateful() bool {
	return false
}
