package percentile

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

var testQuantiles = []float64{0, 0.1, 0.25, 0.5, 0.75, 0.9, 0.95, 0.99, 0.999, 1}
var testSizes = []int{10, 100, 1000, 10000, 100000}

func GenSketch(n int, gen func(i int) float64) []float64 {
	s := NewGKArray()

	for i := 0; i < n; i++ {
		s.Add(gen(i))
	}

	vals := make([]float64, 0, len(testQuantiles))
	for _, q := range testQuantiles {
		val := s.Quantile(q)
		vals = append(vals, val)
	}

	return vals
}

// Constant stream
func ConstantGenerator(i int) float64 {
	return 42
}

func TestConstant(t *testing.T) {
	for _, n := range testSizes {
		vals := GenSketch(n, ConstantGenerator)
		for _, v := range vals {
			assert.Equal(t, 42.0, v)
		}
	}
}

// Uniform distribution
func UniformGenerator(i int) float64 {
	return float64(i)
}

func TestUniform(t *testing.T) {
	for _, n := range testSizes {
		vals := GenSketch(n, UniformGenerator)
		for i, v := range vals {
			var expected float64
			if testQuantiles[i] == 0 {
				expected = 0
			} else if testQuantiles[i] == 1 {
				expected = float64(n) - 1
			} else {
				expected = math.Floor(testQuantiles[i] * (float64(n) - 1))
			}
			assert.InDelta(t, expected, v, EPSILON*float64(n))
		}
	}
}

func TestMerge(t *testing.T) {
	for _, n := range testSizes {
		s1 := NewGKArray()
		for i := 0; i < n; i += 2 {
			s1.Add(float64(i))
		}
		s2 := NewGKArray()
		for i := 1; i < n; i += 2 {
			s2.Add(float64(i))
		}
		s1.Merge(s2)

		for i, q := range testQuantiles {
			val := s1.Quantile(q)

			var expected float64
			if testQuantiles[i] == 0 {
				expected = 0
			} else if testQuantiles[i] == 1 {
				expected = float64(n) - 1
			} else {
				expected = math.Floor(q * (float64(n) - 1))
			}
			assert.InDelta(t, expected, val, 2*EPSILON*float64(n))
		}
	}
}
