package portrollup

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPortToString(t *testing.T) {
	assert.Equal(t, "65535", PortToString(math.MaxUint16))
	assert.Equal(t, "10", PortToString(10))
	assert.Equal(t, "0", PortToString(0))
	assert.Equal(t, "*", PortToString(-1))
	assert.Equal(t, "invalid", PortToString(-10))
}
