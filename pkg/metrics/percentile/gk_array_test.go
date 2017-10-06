// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package percentile

import (
	"math"
	"math/rand"
	"sort"
	"testing"

	"github.com/google/gofuzz"
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

var testSizes = []int{3, 5, 10, 100, 1000, 10000, 100000}

func EvaluateSketch(t *testing.T, n int, gen Generator) {
	s := NewGKArray()
	d := NewDataset()
	for i := 0; i < n; i++ {
		value := gen.Generate()
		s = s.Add(value)
		d.Add(value)
	}
	// Need to compress before querying for quantiles
	s = s.compressWithIncoming(nil)
	eps := float64(1.0e-6)
	for _, q := range testQuantiles {
		assert.InDelta(t, d.Quantile(q), s.Quantile(q), EPSILON*(float64(n)))
		assert.Equal(t, d.Min(), s.Min)
		assert.Equal(t, d.Max(), s.Max)
		assert.InEpsilon(t, d.Avg(), s.Avg, eps)
		assert.InEpsilon(t, d.Sum(), s.Sum, eps)
		assert.Equal(t, d.Count, s.Count)
	}
}

// Constant stream
type Constant struct{ constant float64 }

func NewConstant(constant float64) *Constant { return &Constant{constant: constant} }
func (s *Constant) Generate() float64        { return s.constant }

func TestConstant(t *testing.T) {
	for _, n := range testSizes {
		constantGenerator := NewConstant(42)
		s := NewGKArray()
		d := NewDataset()
		for i := 0; i < n; i++ {
			value := constantGenerator.Generate()
			s = s.Add(value)
			d.Add(value)
		}
		// Need to compress before querying for quantiles
		s = s.compressWithIncoming(nil)
		for _, q := range testQuantiles {
			assert.Equal(t, 42.0, s.Quantile(q))
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
		s1 := NewGKArray()
		generator1 := NewNormal(35, 1)
		for i := 0; i < n; i += 3 {
			value := generator1.Generate()
			s1 = s1.Add(value)
			d.Add(value)
		}
		s2 := NewGKArray()
		generator2 := NewNormal(50, 2)
		for i := 1; i < n; i += 3 {
			value := generator2.Generate()
			s2 = s2.Add(value)
			d.Add(value)
		}
		s1 = s1.Merge(s2)
		s3 := NewGKArray()
		generator3 := NewNormal(40, 0.5)
		for i := 2; i < n; i += 3 {
			value := generator3.Generate()
			s3 = s3.Add(value)
			d.Add(value)
		}
		s1 = s1.Merge(s3)
		eps := float64(1e-6)
		for _, q := range testQuantiles {
			assert.InDelta(t, d.Quantile(q), s1.Quantile(q), 2*EPSILON*float64(n))
			assert.InEpsilon(t, d.Min(), s1.Min, eps)
			assert.InEpsilon(t, d.Max(), s1.Max, eps)
			assert.InEpsilon(t, d.Avg(), s1.Avg, eps)
			assert.InEpsilon(t, d.Sum(), s1.Sum, eps)
			assert.InEpsilon(t, d.Count, s1.Count, eps)
			assert.Equal(t, 0, len(s1.Incoming))
		}
	}
}

func TestMergeEmpty(t *testing.T) {
	for _, n := range testSizes {
		d := NewDataset()
		// Merge a non-empty sketch to an empty sketch
		s1 := NewGKArray()
		s2 := NewGKArray()
		generator := NewExponential(5)
		for i := 0; i < n; i++ {
			value := generator.Generate()
			s2 = s2.Add(value)
			d.Add(value)
		}
		s1 = s1.Merge(s2)
		eps := float64(1e-6)
		for _, q := range testQuantiles {
			assert.InDelta(t, d.Quantile(q), s1.Quantile(q), 2*EPSILON*float64(n))
			assert.InEpsilon(t, d.Min(), s1.Min, eps)
			assert.InEpsilon(t, d.Max(), s1.Max, eps)
			assert.InEpsilon(t, d.Avg(), s1.Avg, eps)
			assert.InEpsilon(t, d.Sum(), s1.Sum, eps)
			assert.InEpsilon(t, d.Count, s1.Count, eps)
			assert.Equal(t, 0, len(s1.Incoming))
		}

		// Merge an empty sketch to a non-empty sketch
		s3 := NewGKArray()
		s2 = s2.Merge(s3)
		for _, q := range testQuantiles {
			assert.InDelta(t, d.Quantile(q), s2.Quantile(q), 2*EPSILON*float64(n))
			assert.InEpsilon(t, d.Min(), s2.Min, eps)
			assert.InEpsilon(t, d.Max(), s2.Max, eps)
			assert.InEpsilon(t, d.Avg(), s2.Avg, eps)
			assert.InEpsilon(t, d.Sum(), s2.Sum, eps)
			assert.InEpsilon(t, d.Count, s2.Count, eps)
			assert.Equal(t, 0, len(s2.Incoming))
		}
	}
}

func TestMergeMixed(t *testing.T) {
	for _, n := range testSizes {
		d := NewDataset()
		s1 := NewGKArray()
		generator1 := NewNormal(100, 1)
		for i := 0; i < n; i += 3 {
			value := generator1.Generate()
			s1 = s1.Add(value)
			d.Add(value)
		}
		s2 := NewGKArray()
		generator2 := NewExponential(5)
		for i := 1; i < n; i += 3 {
			value := generator2.Generate()
			s2 = s2.Add(value)
			d.Add(value)
		}
		s1 = s1.Merge(s2)
		s3 := NewGKArray()
		generator3 := NewExponential(0.1)
		for i := 2; i < n; i += 3 {
			value := generator3.Generate()
			s3 = s3.Add(value)
			d.Add(value)
		}
		s1 = s1.Merge(s3)

		eps := float64(1e-6)
		for _, q := range testQuantiles {
			assert.InDelta(t, d.Quantile(q), s1.Quantile(q), 2*EPSILON*float64(n))
			assert.InEpsilon(t, d.Min(), s1.Min, eps)
			assert.InEpsilon(t, d.Max(), s1.Max, eps)
			assert.InEpsilon(t, d.Avg(), s1.Avg, eps)
			assert.InEpsilon(t, d.Sum(), s1.Sum, eps)
			assert.InEpsilon(t, d.Count, s1.Count, eps)
			assert.Equal(t, 0, len(s1.Incoming))
		}
	}
}

func TestInterpolatedQuantile(t *testing.T) {
	for _, n := range testSizes {
		if n < int(1/EPSILON) {
			s := NewGKArray()
			for i := 0; i < n; i++ {
				s = s.Add(float64(i))
			}
			s = s.compressWithIncoming(nil)
			for _, q := range testQuantiles {
				expected := q * (float64(n) - 1)
				assert.Equal(t, expected, s.Quantile(q))
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
