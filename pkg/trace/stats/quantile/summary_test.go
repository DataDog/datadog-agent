// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package quantile

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

/************************************************************************************
	DATA VALIDATION, with different strategies make sure of the correctness of
	our epsilon-approximate quantiles
************************************************************************************/

var testQuantiles = []float64{0, 0.1, 0.25, 0.5, 0.75, 0.90, 0.95, 0.99, 0.999, 0.9999, 1}

func GenSummarySlice(n int, gen func(i int) float64) []float64 {
	s := NewSliceSummary()

	for i := 0; i < n; i++ {
		s.Insert(gen(i), uint64(i))
	}

	vals := make([]float64, 0, len(testQuantiles))
	for _, q := range testQuantiles {
		val := s.Quantile(q)
		vals = append(vals, val)
	}

	return vals
}

/* CONSTANT STREAMS
   The most simple checker
*/
func ConstantGenerator(i int) float64 {
	return 42
}
func SummarySliceConstantN(t *testing.T, n int) {
	assert := assert.New(t)
	vals := GenSummarySlice(n, ConstantGenerator)
	for _, v := range vals {
		assert.Equal(42.0, v)
	}
}
func TestSummarySliceConstant10(t *testing.T) {
	SummarySliceConstantN(t, 10)
}
func TestSummarySliceConstant100(t *testing.T) {
	SummarySliceConstantN(t, 100)
}
func TestSummarySliceConstant1000(t *testing.T) {
	SummarySliceConstantN(t, 1000)
}
func TestSummarySliceConstant10000(t *testing.T) {
	SummarySliceConstantN(t, 10000)
}
func TestSummarySliceConstant100000(t *testing.T) {
	SummarySliceConstantN(t, 100000)
}

/* uniform distribution
   expected quantiles are easily to compute as the value == its rank
   1 to i
*/
func UniformGenerator(i int) float64 {
	return float64(i)
}
func SummarySliceUniformN(t *testing.T, n int) {
	assert := assert.New(t)
	vals := GenSummarySlice(n, UniformGenerator)

	for i, v := range vals {
		var exp float64
		if testQuantiles[i] == 0 {
			exp = 0
		} else if testQuantiles[i] == 1 {
			exp = float64(n) - 1
		} else {
			rank := math.Ceil(testQuantiles[i] * float64(n))
			exp = rank - 1
		}
		assert.InDelta(exp, v, EPSILON*float64(n), "quantile %f failed, exp: %f, val: %f", testQuantiles[i], exp, v)
	}
}
func TestSummarySliceUniform10(t *testing.T) {
	SummarySliceUniformN(t, 10)
}
func TestSummarySliceUniform100(t *testing.T) {
	SummarySliceUniformN(t, 100)
}
func TestSummarySliceUniform1000(t *testing.T) {
	SummarySliceUniformN(t, 1000)
}
func TestSummarySliceUniform10000(t *testing.T) {
	SummarySliceUniformN(t, 10000)
}
func TestSummarySliceUniform100000(t *testing.T) {
	SummarySliceUniformN(t, 100000)
}

func TestSummarySliceMerge(t *testing.T) {
	assert := assert.New(t)
	s1 := NewSliceSummary()
	for i := 0; i < 101; i++ {
		s1.Insert(float64(i), uint64(i))
	}

	s2 := NewSliceSummary()
	for i := 0; i < 50; i++ {
		s2.Insert(float64(i), uint64(i))
	}

	s1.Merge(s2)

	expected := map[float64]float64{
		0.0: 0,
		0.2: 15,
		0.4: 30,
		0.6: 45,
		0.8: 70,
		1.0: 100,
	}

	for q, e := range expected {
		v := s1.Quantile(q)
		assert.Equal(e, v)
	}
}

func TestSliceSummaryRemergeReal10000(t *testing.T) {
	s := NewSliceSummary()
	for n := 0; n < 10000; n++ {
		s1 := NewSliceSummary()
		for i := 0; i < 100; i++ {
			s1.Insert(float64(i), uint64(i))
		}
		s.Merge(s1)

	}

	fmt.Println(s)
	slices := s.BySlices()
	fmt.Println(slices)
	total := 0
	for _, s := range slices {
		total += s.Weight
	}
	fmt.Println(total)
}

func TestSliceSummaryRemerge10000(t *testing.T) {
	s1 := NewSliceSummary()
	for n := 0; n < 1000; n++ {
		for i := 0; i < 100; i++ {
			s1.Insert(float64(i), uint64(i))
		}

		//      fmt.Println(s1)
	}

	fmt.Println(s1)
	slices := s1.BySlices()
	fmt.Println(slices)
	total := 0
	for _, s := range slices {
		total += s.Weight
	}
	fmt.Println(total)
}

func TestSummaryBySlices(t *testing.T) {
	assert := assert.New(t)

	s := NewSliceSummary()
	for i := 1; i < 11; i++ {
		s.Insert(float64(i), uint64(i))
	}
	s.Insert(float64(5), uint64(42))
	s.Insert(float64(5), uint64(53))

	slices := s.BySlices()
	fmt.Println(slices)
	assert.Equal(10, len(slices))
	for i, sl := range slices {
		assert.Equal(float64(i+1), sl.Start)
		assert.Equal(float64(i+1), sl.End)
		if i == 4 {
			assert.Equal(3, sl.Weight)
		} else {
			assert.Equal(1, sl.Weight)
		}
	}
}
