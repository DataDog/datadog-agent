// +build linux_bpf

package http

import (
	"testing"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/stretchr/testify/assert"
)

func TestAddRequest(t *testing.T) {
	var stats RequestStats
	stats.AddRequest(400, 10.0)
	stats.AddRequest(404, 15.0)
	stats.AddRequest(405, 20.0)

	for i := 0; i < 5; i++ {
		if i == 3 {
			assert.Equal(t, 3, stats[i].Count)
			assert.Equal(t, 3.0, stats[i].Latencies.GetCount())

			verifyQuantile(t, stats[i].Latencies, 0.0, 10.0)  // min item
			verifyQuantile(t, stats[i].Latencies, 0.99, 15.0) // median
			verifyQuantile(t, stats[i].Latencies, 1.0, 20.0)  // max item
		} else {
			assert.Equal(t, 0, stats[i].Count)
			assert.Nil(t, stats[i].Latencies)
		}
	}
}

func TestCombineWith(t *testing.T) {
	var stats RequestStats
	for i := 0; i < 5; i++ {
		assert.Equal(t, 0, stats[i].Count)
		assert.True(t, stats[i].Latencies == nil)
	}

	var stats2, stats3, stats4 RequestStats
	stats2.AddRequest(400, 10.0)
	stats3.AddRequest(404, 15.0)
	stats4.AddRequest(405, 20.0)

	stats.CombineWith(stats2)
	stats.CombineWith(stats3)
	stats.CombineWith(stats4)

	for i := 0; i < 5; i++ {
		if i == 3 {
			assert.Equal(t, 3, stats[i].Count)
			verifyQuantile(t, stats[i].Latencies, 0.0, 10.0)
			verifyQuantile(t, stats[i].Latencies, 0.5, 15.0)
			verifyQuantile(t, stats[i].Latencies, 1.0, 20.0)
		} else {
			assert.Equal(t, 0, stats[i].Count, "eita deu ruim %v", i)
			assert.True(t, stats[i].Latencies == nil)
		}
	}
}

func verifyQuantile(t *testing.T, sketch *ddsketch.DDSketch, q float64, expectedValue float64) {
	val, err := sketch.GetValueAtQuantile(q)
	assert.Nil(t, err)

	acceptableError := expectedValue * RelativeAccuracy
	assert.True(t, val >= expectedValue-acceptableError)
	assert.True(t, val <= expectedValue+acceptableError)
}
