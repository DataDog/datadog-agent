// +build !windows

package backoff

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultRandomSeed(t *testing.T) {
	assert := assert.New(t)

	delayProvider1 := DefaultExponentialDelayProvider()
	delayProvider2 := DefaultExponentialDelayProvider()

	// Ensure different timers are not synchronized in their backoffing (use different seeds)
	assert.NotEqual(delayProvider1(0, nil), delayProvider2(0, nil))
}
