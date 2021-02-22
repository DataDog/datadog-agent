// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quantile

import (
	"math"
	"testing"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/pb/sketchpb"
	"github.com/DataDog/sketches-go/ddsketch/store"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

const relativeValueError = 0.01

// fillBinsMap copies half of the bins to the map in order to ensure
// that we test both the map and the array storage of bins.
func fillBinsMap(s *sketchpb.DDSketch) {
	s.PositiveValues.BinCounts = make(map[int32]float64)
	n := len(s.PositiveValues.ContiguousBinCounts)
	m := n / 2
	for i, c := range s.PositiveValues.ContiguousBinCounts[m:] {
		s.PositiveValues.BinCounts[int32(i+m)+s.PositiveValues.ContiguousBinIndexOffset] = c
	}
	s.PositiveValues.ContiguousBinCounts = s.PositiveValues.ContiguousBinCounts[:m]
}

// getConvertedSketchQuantiles follows this steps:
// 1. Generates two DDSketches: OK and Errors:
//    - Errors uses the generator function from 0 to n-1
//    - OK uses the generator function from n to 2*n-1)
// 2. Converts OK and errors DDSketches to hits and errors GK sketches (hits = ok + errors)
//    - That way, hits is the distribution from 0 to 2*n-1
// 3. Computes quantiles on the GK sketches hits and errors and returns them
func getConvertedSketchQuantiles(t *testing.T, n int, gen func(i int) float64, testQuantiles []float64) (hits []float64, errors []float64) {
	assert := assert.New(t)
	m, err := mapping.NewLogarithmicMapping(relativeValueError)
	assert.Nil(err)
	errS := ddsketch.NewDDSketch(m, store.NewDenseStore(), store.NewDenseStore())
	okS := ddsketch.NewDDSketch(m, store.NewDenseStore(), store.NewDenseStore())

	for i := 0; i < n; i++ {
		x := gen(i)
		assert.Nil(errS.Add(x))
	}
	for i := n; i < n*2; i++ {
		x := gen(i)
		assert.Nil(okS.Add(x))
	}
	okProto := okS.ToProto()
	errProto := errS.ToProto()

	fillBinsMap(okProto)
	fillBinsMap(errProto)

	okData, err := proto.Marshal(okProto)
	assert.Nil(err)
	errData, err := proto.Marshal(errProto)
	assert.Nil(err)
	gkHitsSketch, gkErrSketch, err := DDToGKSketches(okData, errData)
	assert.Nil(err)

	for _, q := range testQuantiles {
		val := gkHitsSketch.Quantile(q)
		hits = append(hits, val)
	}
	for _, q := range testQuantiles {
		val := gkErrSketch.Quantile(q)
		errors = append(errors, val)
	}
	return hits, errors
}

func testDDSketchToGKConstant(n int) func(t *testing.T) {
	return func(t *testing.T) {
		assert := assert.New(t)
		hits, errors := getConvertedSketchQuantiles(t, n, ConstantGenerator, testQuantiles)
		for _, v := range append(hits, errors...) {
			assert.InEpsilon(42.0, v, relativeValueError)
		}
	}
}

// testDDSketchToGKUniform tests the conversion from dd to gk sketches on uniform distributions.
// For uniform distributions, quantiles are easy to compute as the value == its rank.
func testDDSketchToGKUniform(n int) func(t *testing.T) {
	return func(t *testing.T) {
		assert := assert.New(t)
		hits, errors := getConvertedSketchQuantiles(t, n, UniformGenerator, testQuantiles)

		for i, v := range errors {
			var exp float64
			if testQuantiles[i] == 0 {
				exp = 0
			} else if testQuantiles[i] == 1 {
				exp = float64(n) - 1
			} else {
				rank := math.Ceil(testQuantiles[i] * float64(n))
				exp = rank - 1
			}
			assert.InDelta(exp, v, EPSILON*float64(n)+relativeValueError*exp, "quantile %f failed, exp: %f, val: %f", testQuantiles[i], exp, v)
		}
		for i, v := range hits {
			var exp float64
			if testQuantiles[i] == 0 {
				exp = 0
			} else if testQuantiles[i] == 1 {
				exp = float64(2*n) - 1
			} else {
				rank := math.Ceil(testQuantiles[i] * float64(2*n))
				exp = rank - 1
			}
			assert.InDelta(exp, v, EPSILON*float64(2*n)+relativeValueError*exp, "quantile %f failed, exp: %f, val: %f", testQuantiles[i], exp, v)
		}
	}
}

func TestDDToGKSketch(t *testing.T) {
	t.Run("uniform10", testDDSketchToGKUniform(10))
	t.Run("uniform1e3", testDDSketchToGKUniform(1000))
	t.Run("constant10", testDDSketchToGKConstant(10))
	t.Run("constant1e3", testDDSketchToGKConstant(1000))
}
