package enrichment

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemapDirection(t *testing.T) {
	assert.Equal(t, "ingress", RemapDirection(uint32(0)))
	assert.Equal(t, "egress", RemapDirection(uint32(1)))
	assert.Equal(t, "ingress", RemapDirection(uint32(99))) // invalid direction will default to ingress
}
