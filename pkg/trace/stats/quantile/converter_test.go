package quantile

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"math"
	"testing"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/pb/sketchpb"
	"github.com/DataDog/sketches-go/ddsketch/store"
	"github.com/stretchr/testify/assert"
)

const relativeValueError = 0.01

func fillNonContiguousBins(s *sketchpb.DDSketch) {
	// copy half of the bins to the map
	s.PositiveValues.BinCounts = make(map[int32]float64)
	n := len(s.PositiveValues.ContiguousBinCounts)
	x := n / 2
	for i, c := range s.PositiveValues.ContiguousBinCounts[x:] {
		s.PositiveValues.BinCounts[int32(i+x)+s.PositiveValues.ContiguousBinIndexOffset] = c
	}
	fmt.Println("adding")
	s.PositiveValues.ContiguousBinCounts = s.PositiveValues.ContiguousBinCounts[:x]
}

// getConvertedSketchQuantiles generates a DDSketch using the generator function, then converts it
// to GK Sketch and gets the quantiles.
func getConvertedSketchQuantiles(t *testing.T, n int, gen func(i int) float64, testQuantiles []float64) (hits []float64, errors []float64) {
	assert := assert.New(t)
	m, err := mapping.NewLogarithmicMapping(relativeValueError)
	assert.Nil(err)
	errS := ddsketch.NewDDSketch(m, store.NewDenseStore(), store.NewDenseStore())
	okS := ddsketch.NewDDSketch(m, store.NewDenseStore(), store.NewDenseStore())

	// err is the distribution until n, hits (ok + err) is the distribution until 2*n
	for i := 0; i < n; i++ {
		x := gen(i)
		assert.Nil(errS.Accept(x))
	}
	for i := n; i < n*2; i++ {
		x := gen(i)
		assert.Nil(okS.Accept(x))
	}
	okProto := okS.ToProto()
	errProto := errS.ToProto()

	fillNonContiguousBins(okProto)
	fillNonContiguousBins(errProto)

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

func testDDSketchToGKConstant(t *testing.T, n int) {
	assert := assert.New(t)
	hits, errors := getConvertedSketchQuantiles(t, n, ConstantGenerator, testQuantiles)
	for _, v := range append(hits, errors...) {
		assert.InEpsilon(42.0, v, relativeValueError)
	}
}

/* uniform distribution
   expected quantiles are easy to compute as the value == its rank
*/

func testDDSketchToGKUniform(t *testing.T, n int) {
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
	// hits = ok + err. because ok is the distribution from n to 2n,
	// and errors is the distribution from 1 to n, hits is the distribution from 1 to 2n
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

func TestDDToGKSketch(t *testing.T) {
	t.Run("uniform10", func(t *testing.T) {
		testDDSketchToGKUniform(t, 10)
	})
	t.Run("uniform1e3", func(t *testing.T) {
		testDDSketchToGKUniform(t, 1000)
	})
	t.Run("constant10", func(t *testing.T) {
		testDDSketchToGKConstant(t, 10)
	})
	t.Run("constant1e3", func(t *testing.T) {
		testDDSketchToGKConstant(t, 1000)
	})
}
