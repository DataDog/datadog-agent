package quantile

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/mapping"
	"github.com/DataDog/sketches-go/ddsketch/store"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"math"
	"testing"
)

const relativeValueError = 0.01

func genConvertedSummarySlice(t *testing.T, n int, gen func(i int) float64, testQuantiles []float64) []float64 {
	assert := assert.New(t)
	m, err := mapping.NewLogarithmicMapping(relativeValueError)
	assert.Nil(err)
	s := ddsketch.NewDDSketch(m, store.NewDenseStore(), store.NewDenseStore())

	for i := 0; i < n; i++ {
		x := gen(i)
		assert.Nil(s.Accept(x))
	}

	data, err := proto.Marshal(s.ToProto())
	assert.Nil(err)
	var ske pb.DDSketch
	err = proto.Unmarshal(data, &ske)
	assert.Nil(err)
	_, gkSketch, err := DDSketchesToGK(data, data)
	assert.Nil(err)

	vals := make([]float64, 0, len(testQuantiles))
	for _, q := range testQuantiles {
		val := gkSketch.Quantile(q)
		vals = append(vals, val)
	}
	return vals
}

func testDDSketchToGKConstant(t *testing.T, n int) {
	assert := assert.New(t)
	vals := genConvertedSummarySlice(t, n, ConstantGenerator, testQuantiles)
	for _, v := range vals {
		assert.InEpsilon(42.0, v, relativeValueError)
	}
}

func TestDDSketchToGKConstant10(t *testing.T) {
	testDDSketchToGKConstant(t, 10)
}

func TestDDSketchToGKConstant1000(t *testing.T) {
	testDDSketchToGKConstant(t, 1000)
}

/* uniform distribution
   expected quantiles are easily to compute as the value == its rank
   1 to i
*/

func testDDSketchToGKUniform(t *testing.T, n int) {
	assert := assert.New(t)
	vals := genConvertedSummarySlice(t, n, UniformGenerator, testQuantiles)

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
		// the errors stack
		assert.InDelta(exp, v,EPSILON*float64(n)+ relativeValueError*exp, "quantile %f failed, exp: %f, val: %f", testQuantiles[i], exp, v)
	}
}

func TestDDSketchToGKUniform10(t *testing.T) {
	testDDSketchToGKUniform(t, 10)
}

func TestDDSketchToGKUniform1e3(t *testing.T) {
	testDDSketchToGKUniform(t, 1000)
}
