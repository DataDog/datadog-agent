package gohai

import (
	"github.com/DataDog/gohai/cpu"
	"github.com/DataDog/gohai/filesystem"
	"github.com/DataDog/gohai/memory"
	"github.com/DataDog/gohai/network"
	"github.com/DataDog/gohai/platform"

	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetPayload(t *testing.T) {

	assert.NotNil(t, gohai.Gohai.CPU)
	assert.NotNil(t, gohai.Gohai.FileSystem)
	assert.NotNil(t, gohai.Gohai.Memory)
	assert.NotNil(t, gohai.Gohai.Network)
	assert.NotNil(t, gohai.Gohai.Platform)
}
