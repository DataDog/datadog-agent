// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package percentile

import (
	"math"
	"math/rand"
	"sort"
	"testing"

	fuzz "github.com/google/gofuzz"
	"github.com/stretchr/testify/assert"
)

type Generator interface {
	Generate() float64
}

type Dataset struct {
	Values []float64
	Count  int64
	sorted bool
}

func NewDataset() *Dataset { return &Dataset{} }
func (d *Dataset) Add(v float64) {
	d.Values = append(d.Values, v)
	d.Count++
	d.sorted = false
}

func (d *Dataset) Quantile(q float64) float64 {
	if q < 0 || q > 1 {
		panic("Quantile out of bounds")
	}
	d.Sort()
	if d.Count == 0 {
		return math.NaN()
	}

	rank := q * float64(d.Count-1)
	indexBelow := int64(rank)
	indexAbove := indexBelow + 1
	if indexAbove > d.Count-1 {
		indexAbove = d.Count - 1
	}
	weightAbove := rank - float64(indexBelow)
	weightBelow := 1.0 - weightAbove

	if d.Count < int64(1/EPSILON) {
		return weightBelow*d.Values[indexBelow] + weightAbove*d.Values[indexAbove]
	}
	return d.Values[indexBelow]
}

func (d *Dataset) Min() float64 {
	d.Sort()
	return d.Values[0]
}

func (d *Dataset) Max() float64 {
	d.Sort()
	return d.Values[len(d.Values)-1]
}

func (d *Dataset) Sum() float64 {
	s := float64(0)
	for _, v := range d.Values {
		s += v
	}
	return s
}

func (d *Dataset) Avg() float64 {
	return d.Sum() / float64(d.Count)
}

func (d *Dataset) Sort() {
	if d.sorted {
		return
	}
	sort.Float64s(d.Values)
	d.sorted = true
}

var testQuantiles = []float64{0, 0.1, 0.25, 0.5, 0.75, 0.9, 0.95, 0.99, 0.999, 1}

var testSizes = []int{3, 5, 10, 100, 1000}

func EvaluateSketch(t *testing.T, n int, gen Generator) {
	g := NewGKArray()
	d := NewDataset()
	for i := 0; i < n; i++ {
		value := gen.Generate()
		g = g.Add(value)
		d.Add(value)
	}
	// Need to compress before querying for quantiles
	g = g.compressWithIncoming(nil)
	AssertSketchesAccurate(t, d, g, n)
}

func AssertSketchesAccurate(t *testing.T, d *Dataset, g GKArray, n int) {
	assert := assert.New(t)
	eps := float64(1.0e-6)
	for _, q := range testQuantiles {
		assert.InDelta(d.Quantile(q), g.Quantile(q), EPSILON*(float64(n)))
	}
	assert.Equal(d.Min(), g.Min)
	assert.Equal(d.Max(), g.Max)
	assert.InEpsilon(d.Avg(), g.Avg, eps)
	assert.InEpsilon(d.Sum(), g.Sum, eps)
	assert.Equal(d.Count, g.Count)
}

// Constant stream
type Constant struct{ constant float64 }

func NewConstant(constant float64) *Constant { return &Constant{constant: constant} }
func (s *Constant) Generate() float64        { return s.constant }

func TestConstant(t *testing.T) {
	for _, n := range testSizes {
		constantGenerator := NewConstant(42)
		g := NewGKArray()
		d := NewDataset()
		for i := 0; i < n; i++ {
			value := constantGenerator.Generate()
			g = g.Add(value)
			d.Add(value)
		}
		// Need to compress before querying for quantiles
		g = g.compressWithIncoming(nil)
		for _, q := range testQuantiles {
			assert.Equal(t, 42.0, g.Quantile(q))
		}
	}
}

// Uniform distribution
type Uniform struct{ currentVal float64 }

func NewUniform() *Uniform { return &Uniform{0} }
func (g *Uniform) Generate() float64 {
	value := g.currentVal
	g.currentVal++
	return value
}

func TestUniform(t *testing.T) {
	for _, n := range testSizes {
		uniformGenerator := NewUniform()
		EvaluateSketch(t, n, uniformGenerator)
	}
}

// Normal distribution
type Normal struct{ mean, stddev float64 }

func NewNormal(mean, stddev float64) *Normal { return &Normal{mean: mean, stddev: stddev} }
func (g *Normal) Generate() float64          { return rand.NormFloat64()*g.stddev + g.mean }

func TestNormal(t *testing.T) {
	for _, n := range testSizes {
		normalGenerator := NewNormal(35, 1)
		EvaluateSketch(t, n, normalGenerator)
	}
}

// Exponential distribution
type Exponential struct{ rate float64 }

func NewExponential(rate float64) *Exponential { return &Exponential{rate: rate} }
func (g *Exponential) Generate() float64       { return rand.ExpFloat64() / g.rate }

func TestExponential(t *testing.T) {
	for _, n := range testSizes {
		expGenerator := NewExponential(2)
		EvaluateSketch(t, n, expGenerator)
	}
}
func TestMergeNormal(t *testing.T) {
	for _, n := range testSizes {
		d := NewDataset()
		g1 := NewGKArray()
		generator1 := NewNormal(35, 1)
		for i := 0; i < n; i += 3 {
			value := generator1.Generate()
			g1 = g1.Add(value)
			d.Add(value)
		}
		g2 := NewGKArray()
		generator2 := NewNormal(50, 2)
		for i := 1; i < n; i += 3 {
			value := generator2.Generate()
			g2 = g2.Add(value)
			d.Add(value)
		}
		g1 = g1.Merge(g2)

		g3 := NewGKArray()
		generator3 := NewNormal(40, 0.5)
		for i := 2; i < n; i += 3 {
			value := generator3.Generate()
			g3 = g3.Add(value)
			d.Add(value)
		}
		g1 = g1.Merge(g3)
		AssertSketchesAccurate(t, d, g1, n)
	}
}

func TestMergeEmpty(t *testing.T) {
	for _, n := range testSizes {
		d := NewDataset()
		// Merge a non-empty sketch to an empty sketch
		g1 := NewGKArray()
		g2 := NewGKArray()
		generator := NewExponential(5)
		for i := 0; i < n; i++ {
			value := generator.Generate()
			g2 = g2.Add(value)
			d.Add(value)
		}
		g1 = g1.Merge(g2)
		AssertSketchesAccurate(t, d, g1, n)

		// Merge an empty sketch to a non-empty sketch
		g3 := NewGKArray()
		g2 = g2.Merge(g3)
		AssertSketchesAccurate(t, d, g2, n)
	}
}

func TestMergeMixed(t *testing.T) {
	for _, n := range testSizes {
		d := NewDataset()
		g1 := NewGKArray()
		generator1 := NewNormal(100, 1)
		for i := 0; i < n; i += 3 {
			value := generator1.Generate()
			g1 = g1.Add(value)
			d.Add(value)
		}
		g2 := NewGKArray()
		generator2 := NewExponential(5)
		for i := 1; i < n; i += 3 {
			value := generator2.Generate()
			g2 = g2.Add(value)
			d.Add(value)
		}
		g1 = g1.Merge(g2)

		g3 := NewGKArray()
		generator3 := NewExponential(0.1)
		for i := 2; i < n; i += 3 {
			value := generator3.Generate()
			g3 = g3.Add(value)
			d.Add(value)
		}
		g1 = g1.Merge(g3)

		AssertSketchesAccurate(t, d, g1, n)
	}
}

func TestInterpolatedQuantile(t *testing.T) {
	for _, n := range testSizes {
		if n < int(1/EPSILON) {
			g := NewGKArray()
			for i := 0; i < n; i++ {
				g = g.Add(float64(i))
			}
			g = g.compressWithIncoming(nil)
			for _, q := range testQuantiles {
				expected := q * (float64(n) - 1)
				assert.Equal(t, expected, g.Quantile(q))
			}
		}
	}
}

// Any random GKArray will not cause panic when Add() or Merge() is called
// as long as it passes the IsValid() method
func TestValidDoesNotPanic(t *testing.T) {
	var s1, s2 GKArray
	var q float64
	nTests := 100
	fuzzer := fuzz.New()
	for i := 0; i < nTests; i++ {
		fuzzer.Fuzz(&s1)
		fuzzer.Fuzz(&s2)
		fuzzer.Fuzz(&q)
		s1 = makeValid(s1)
		s2 = makeValid(s2)
		assert.True(t, s1.IsValid())
		assert.True(t, s2.IsValid())
		assert.NotPanics(t, func() { s1.Quantile(q); s1.Merge(s2) })
	}
}

func makeValid(s GKArray) GKArray {
	if len(s.Entries) == 0 {
		s.Count = int64(len(s.Entries))
	}

	gSum := int64(0)
	for _, e := range s.Entries {
		gSum += int64(e.G)
	}
	s.Count = gSum + int64(len(s.Incoming))

	return s
}

func TestQuantiles(t *testing.T) {
	var qVals []float64
	var vals []float64
	nTests := 100
	qFuzzer := fuzz.New().NilChance(0).NumElements(5, 10)
	vFuzzer := fuzz.New().NilChance(0).NumElements(10, 500)
	for i := 0; i < nTests; i++ {
		s := NewGKArray()
		qFuzzer.Fuzz(&qVals)
		sort.Float64s(qVals)
		vFuzzer.Fuzz(&vals)
		for _, v := range vals {
			s = s.Add(v)
		}
		s = s.compressWithIncoming(nil)
		quantiles := s.Quantiles(qVals)
		eps := 1.e-6
		for j, q := range qVals {
			if q < 0 || q > 1 {
				assert.True(t, math.IsNaN(quantiles[j]))
			} else {
				assert.InEpsilon(t, s.Quantile(q), quantiles[j], eps)
			}
		}
	}
}

func TestQuantilesInvalid(t *testing.T) {
	s := NewGKArray()
	gen := NewNormal(35, 1)
	qVals := []float64{-0.2, -0.1, 0.5, 0.75, 0.95, 1.2}
	n := 200
	for i := 0; i < n; i++ {
		s = s.Add(gen.Generate())
	}
	quantiles := s.Quantiles(qVals)
	assert.True(t, math.IsNaN(quantiles[0]))
	assert.True(t, math.IsNaN(quantiles[1]))
	assert.True(t, math.IsNaN(quantiles[5]))
	eps := 1.0e-6
	assert.InEpsilon(t, s.Quantile(0.5), quantiles[2], eps)
	assert.InEpsilon(t, s.Quantile(0.75), quantiles[3], eps)
	assert.InEpsilon(t, s.Quantile(0.95), quantiles[4], eps)
}

// Test that successive Quantile() calls do not modify the sketch
func TestConsistentQuantile(t *testing.T) {
	var vals []float64
	var q float64
	nTests := 200
	vfuzzer := fuzz.New().NilChance(0).NumElements(10, 500)
	fuzzer := fuzz.New()
	for i := 0; i < nTests; i++ {
		s := NewGKArray()
		vfuzzer.Fuzz(&vals)
		fuzzer.Fuzz(&q)
		for _, v := range vals {
			s = s.Add(v)
		}
		q1 := s.Quantile(q)
		q2 := s.Quantile(q)
		assert.Equal(t, q1, q2)
	}
}

// Test that Quantile() calls do not panic for number of values up to 1/epsilon
func TestNoPanic(t *testing.T) {
	s := NewGKArray()
	for i := 0; i < 2*int(1/EPSILON); i++ {
		s = s.Add(float64(i))
		assert.NotPanics(t, func() { s.Quantile(0.9) })
	}
}
