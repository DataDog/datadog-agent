package percentile

import (
	"math"
	"math/rand"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

type Generator interface {
	Generate() float64
}

type Dataset struct {
	Values   []float64
	ValCount int
	sorted   bool
}

func NewDataset() *Dataset { return &Dataset{} }
func (d *Dataset) Add(v float64) {
	d.Values = append(d.Values, v)
	d.ValCount++
	d.sorted = false
}

func (d *Dataset) Quantile(q float64) float64 {
	if q < 0 || q > 1 {
		panic("Quantile out of bounds")
	}
	d.Sort()
	if d.ValCount == 0 {
		return math.NaN()
	}

	rank := q * float64(d.ValCount-1)
	indexBelow := int(rank)
	indexAbove := indexBelow + 1
	if indexAbove > d.ValCount-1 {
		indexAbove = d.ValCount - 1
	}
	weightAbove := rank - float64(indexBelow)
	weightBelow := 1.0 - weightAbove

	if d.ValCount < int(1/EPSILON) {
		return weightBelow*d.Values[indexBelow] + weightAbove*d.Values[indexAbove]
	}
	return d.Values[indexBelow]
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
		s.Add(value)
		d.Add(value)
	}
	for _, q := range testQuantiles {
		assert.InDelta(t, d.Quantile(q), s.Quantile(q), EPSILON*(float64(n)))
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
			s.Add(value)
			d.Add(value)
		}
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

func TestMerge(t *testing.T) {
	for _, n := range testSizes {
		d := NewDataset()
		s1 := NewGKArray()
		generator1 := NewNormal(35, 1)
		for i := 0; i < n; i += 3 {
			value := generator1.Generate()
			s1.Add(value)
			d.Add(value)
		}
		s2 := NewGKArray()
		generator2 := NewNormal(50, 2)
		for i := 1; i < n; i += 3 {
			value := generator2.Generate()
			s2.Add(value)
			d.Add(value)
		}
		s1.Merge(s2)
		s3 := NewGKArray()
		generator3 := NewNormal(40, 0.5)
		for i := 2; i < n; i += 3 {
			value := generator3.Generate()
			s3.Add(value)
			d.Add(value)
		}
		s1.Merge(s3)

		for _, q := range testQuantiles {
			assert.InDelta(t, d.Quantile(q), s1.Quantile(q), 2*EPSILON*float64(n))
		}
	}
}

func TestInterpolatedQuantile(t *testing.T) {
	for _, n := range testSizes {
		if n < int(1/EPSILON) {
			s := NewGKArray()
			for i := 0; i < n; i++ {
				s.Add(float64(i))
			}
			for _, q := range testQuantiles {
				expected := q * (float64(n) - 1)
				assert.Equal(t, expected, s.Quantile(q))
			}
		}
	}
}
