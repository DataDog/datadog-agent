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
	cpuPayload, _ := new(cpu.Cpu).Collect()
	fileSystemPayload, _ := new(filesystem.FileSystem).Collect()
	memoryPayload, _ := new(memory.Memory).Collect()
	networkPayload, _ := new(network.Network).Collect()
	platformPayload, _ := new(platform.Platform).Collect()

	gohai := GetPayload()

	assert.Equal(t, cpuPayload, gohai.Gohai.CPU)
	assert.Equal(t, fileSystemPayload, gohai.Gohai.FileSystem)
	assert.Equal(t, memoryPayload, gohai.Gohai.Memory)
	assert.Equal(t, networkPayload, gohai.Gohai.Network)
	assert.Equal(t, platformPayload, gohai.Gohai.Platform)
}
