package sampler

import (
	// "fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAdjustCoefficients(t *testing.T) {
	assert := assert.New(t)

	for _, a := range [][5]float64{
		// currentTPS, totalTPS, maxTPS, offset, cardinality
		[5]float64{10, 50, 15, 0.5, 200},
	} {
		currentTPS, totalTPS, maxTPS, offset, cardinality := a[0], a[1], a[2], a[3], a[4]
		newOffset, newSlope := adjustCoefficients(currentTPS, totalTPS, maxTPS, offset, cardinality)

		// Whatever the input is, we must always have respect basic bounds
		assert.True(newOffset >= minSignatureScoreOffset)
		assert.True(newSlope >= 1)
		assert.True(newSlope <= 10)

		// Check that we are adjusting in the "good" direction
		if currentTPS >= maxTPS {
			assert.True(newOffset <= offset)
		} else {
			assert.True(newOffset >= offset)
		}
	}
}
