// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSortAndGroupSeries(t *testing.T) {
	series := Series{
		{
			Points: []Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
			},
			MType:    APIGaugeType,
			Name:     "test.metrics",
			Interval: 15,
			Tags:     []string{"tag1", "tag2:yes"},
		},
		{
			Points: []Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
			},
			MType:    APIRateType,
			Name:     "test.metrics",
			Interval: 15,
			Tags:     []string{"tag1", "tag2:no"},
		},
		{
			Points:   []Point{},
			MType:    APICountType,
			Name:     "test.metrics2",
			Interval: 15,
			Tags:     nil,
		},
		{
			Points:   []Point{},
			MType:    APICountType,
			Name:     "test.metrics3",
			Interval: 15,
			Tags:     nil,
		},
		{
			Points:   []Point{},
			MType:    APICountType,
			Name:     "test.metrics2",
			Interval: 15,
			Tags:     nil,
		},
	}

	expectedGroups := GroupedSeries{
		{series[0], series[1]},
		{series[2], series[4]},
		{series[3]},
	}

	groups := SortAndGroupSeries(series)
	assert.Equal(t, expectedGroups, groups)
}
