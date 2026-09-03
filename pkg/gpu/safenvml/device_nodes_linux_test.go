// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleGPUInformation = `Model: 		 NVIDIA A10G
IRQ:   		 42
GPU UUID: 	 GPU-82523fd8-3897-f599-d088-ccd04ae45ec1
Video BIOS: 	 94.04.42.00.01
Bus Type: 	 PCIe
DMA Size: 	 47 bits
DMA Mask: 	 0x7fffffffffff
Bus Location: 	 0000:00:1e.0
Device Minor: 	 3
GPU Excluded: 	 No
`

const sampleProcDevices = `Character devices:
  1 mem
  4 /dev/vc/0
 10 misc
195 nvidia-frontend
234 nvidia-uvm
506 nvidia-caps

Block devices:
259 blkext
  7 loop
`

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestReadGPUMinor(t *testing.T) {
	dir := t.TempDir()
	infoPath := filepath.Join(dir, "information")
	writeFile(t, infoPath, sampleGPUInformation)

	minor, err := readGPUMinor(infoPath)
	require.NoError(t, err)
	assert.Equal(t, 3, minor)
}

func TestReadGPUMinorMissingField(t *testing.T) {
	dir := t.TempDir()
	infoPath := filepath.Join(dir, "information")
	writeFile(t, infoPath, "Model: 		 NVIDIA A10G\n")

	_, err := readGPUMinor(infoPath)
	assert.Error(t, err)
}

func TestReadCharDeviceMajor(t *testing.T) {
	dir := t.TempDir()
	devicesPath := filepath.Join(dir, "devices")
	writeFile(t, devicesPath, sampleProcDevices)

	major, ok := readCharDeviceMajor(devicesPath, "nvidia-uvm")
	require.True(t, ok)
	assert.Equal(t, uint32(234), major)
}

func TestReadCharDeviceMajorNotFound(t *testing.T) {
	dir := t.TempDir()
	devicesPath := filepath.Join(dir, "devices")
	// "blkext" is in the Block devices section and must not be matched.
	writeFile(t, devicesPath, sampleProcDevices)

	_, ok := readCharDeviceMajor(devicesPath, "blkext")
	assert.False(t, ok)

	_, ok = readCharDeviceMajor(devicesPath, "nvidia-uvm-tools")
	assert.False(t, ok)
}

func TestExpectedDeviceNodes(t *testing.T) {
	procRoot := t.TempDir()
	writeFile(t, filepath.Join(procRoot, "devices"), sampleProcDevices)

	gpusDir := filepath.Join(procRoot, "driver", "nvidia", "gpus")
	writeFile(t, filepath.Join(gpusDir, "0000:00:1e.0", "information"), sampleGPUInformation)

	nodes := expectedDeviceNodes(procRoot, gpusDir)

	assert.Contains(t, nodes, devNode{name: "nvidiactl", major: nvidiaCharMajor, minor: nvidiaCtlMinor})
	assert.Contains(t, nodes, devNode{name: "nvidia3", major: nvidiaCharMajor, minor: 3})
	assert.Contains(t, nodes, devNode{name: "nvidia-uvm", major: 234, minor: 0})
	assert.Contains(t, nodes, devNode{name: "nvidia-uvm-tools", major: 234, minor: 1})
}

func TestExpectedDeviceNodesNoUVM(t *testing.T) {
	procRoot := t.TempDir()
	// /proc/devices without an nvidia-uvm entry: no UVM nodes should be listed.
	writeFile(t, filepath.Join(procRoot, "devices"), "Character devices:\n195 nvidia-frontend\n")

	gpusDir := filepath.Join(procRoot, "driver", "nvidia", "gpus")
	writeFile(t, filepath.Join(gpusDir, "0000:00:1e.0", "information"), sampleGPUInformation)

	nodes := expectedDeviceNodes(procRoot, gpusDir)
	for _, n := range nodes {
		assert.NotContains(t, n.name, "uvm")
	}
}

func TestIsNvmlInitError(t *testing.T) {
	assert.True(t, isNvmlInitError(NewNvmlAPIErrorOrNil("Init", nvml.ERROR_UNKNOWN)))
	assert.False(t, isNvmlInitError(NewNvmlAPIErrorOrNil("DeviceGetCount", nvml.ERROR_UNKNOWN)))
	assert.False(t, isNvmlInitError(errors.New("library not found")))
	assert.False(t, isNvmlInitError(nil))
}
