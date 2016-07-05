package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func AssertAlmostEqual(t *testing.T, expected, actual interface{}) {
	var delta float64 = 0.1
	assert.InDelta(t, expected, actual, delta)
}
