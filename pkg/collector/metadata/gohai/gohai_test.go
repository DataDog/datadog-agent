package gohai

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetPayload(t *testing.T) {
	gohai := GetPayload()

	assert.NotNil(t, gohai.Gohai.CPU)
	assert.NotNil(t, gohai.Gohai.FileSystem)
	assert.NotNil(t, gohai.Gohai.Memory)
	assert.NotNil(t, gohai.Gohai.Network)
	assert.NotNil(t, gohai.Gohai.Platform)
}
