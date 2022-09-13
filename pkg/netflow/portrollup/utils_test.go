package portrollup

import (
	"github.com/stretchr/testify/assert"
	"math"
	"testing"
)

func TestPortToString(t *testing.T) {
	assert.Equal(t, "65535", PortToString(math.MaxUint16))
	assert.Equal(t, "10", PortToString(10))
	assert.Equal(t, "0", PortToString(0))
	assert.Equal(t, "*", PortToString(-1))
	assert.Equal(t, "invalid", PortToString(-10)) // -10 is invalid, right now we convert it to `*`
}
