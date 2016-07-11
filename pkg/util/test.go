package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// AssertAlmostEqual is self explanatory
func AssertAlmostEqual(t *testing.T, expected, actual interface{}) {
	var delta = 0.1
	assert.InDelta(t, expected, actual, delta)
}
