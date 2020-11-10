// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package quantile

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBySlicesWeightedHalf(t *testing.T) {
	s := NewSliceSummary()
	for i := 0; i < 100000; i++ {
		s.Insert(float64(i % 10000))
	}

	s2 := NewSliceSummary()
	for i := 0; i < 100000; i++ {
		s2.Insert(float64(i % 10000))
	}

	sw1 := WeightedSliceSummary{1.0, s}
	sw2 := WeightedSliceSummary{0.5, s2}

	ss := BySlicesWeighted(sw1, sw2)

	// deviation = (num of sum merged = 2) deviation * GK-dev (eps * N)
	deviation := 2 * EPSILON * (100000 + 50000)
	total := 0
	for _, sl := range ss {
		total += sl.Weight
		// corner case - tolerate
		if sl.Weight == 1 {
			continue
		}

		expected := int(float64(sl.End-sl.Start) * 15)
		require.InDelta(t, expected, sl.Weight, deviation,
			"slice [%.0f;%.0f] = %d failed assertion for slices %v",
			sl.Start, sl.End, sl.Weight, ss,
		)

	}
	require.InDelta(t, 150000, total, deviation, "summaries totals do not match %v", ss)
}

func TestBySlicesWeightedSingle(t *testing.T) {
	s := NewSliceSummary()
	for i := 0; i < 1000000; i++ {
		s.Insert(float64(i))
	}

	sw := WeightedSliceSummary{0.1, s}
	ss := BySlicesWeighted(sw)

	// deviation = deviation * GK-dev (eps * N)
	deviation := EPSILON * 1000000

	total := 0
	for _, sl := range ss {
		total += sl.Weight
		// if the entry is alone this is ok
		if sl.Weight == 1 {
			continue
		}

		expected := int(float64(sl.End-sl.Start) * 0.1)
		require.InDelta(t, expected, sl.Weight, deviation,
			"slice [%.0f;%.0f] = %d failed assertion for slices %v",
			sl.Start, sl.End, sl.Weight, ss,
		)

	}
	require.InDelta(t, 100000, total, deviation, "summaries totals do not match %v", ss)
}

func TestBySlicesWeightedSmall(t *testing.T) {
	s := NewSliceSummary()
	for i := 0; i < 10; i++ {
		s.Insert(float64(i))
	}

	sw := WeightedSliceSummary{0.5, s}
	ss := BySlicesWeighted(sw)

	// should have ~5 elements probabilistically chosen
	fmt.Println(ss)
}

func TestBySlicesWeightedEmpty(t *testing.T) {
	ss := BySlicesWeighted()
	assert.Equal(t, 0, len(ss))
}
