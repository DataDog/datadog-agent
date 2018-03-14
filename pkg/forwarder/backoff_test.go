package forwarder

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMinBackoffFactorValid(t *testing.T) {
	assert.True(t, minBackoffFactor >= 2)
}
