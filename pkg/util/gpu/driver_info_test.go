// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// l4Information is the information file of an L4 GPU, as written by driver 580.65.06.
const l4Information = `Model: 		 NVIDIA L4
IRQ:   		 11
GPU UUID: 	 GPU-e80c8f33-55f5-55d2-18b7-64563f748393
Video BIOS: 	 95.04.29.00.07
Bus Type: 	 PCI
DMA Size: 	 47 bits
DMA Mask: 	 0x7fffffffffff
Bus Location: 	 0000:00:03.0
Device Minor: 	 0
GPU Firmware: 	 580.65.06
GPU Excluded:	 No
`

func writeDriverGPU(t *testing.T, procRoot, busLocation, information string) {
	t.Helper()
	dir := filepath.Join(procRoot, driverGPUsDir, busLocation)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "information"), []byte(information), 0o644))
}

func TestReadDriverGPUs(t *testing.T) {
	procRoot := t.TempDir()
	writeDriverGPU(t, procRoot, "0000:00:05.0", "Model: Tesla T4\nGPU UUID: GPU-1072fdf3-18d9-8720-8b19-141e6afeb8f8\nBus Location: 0000:00:05.0\nDevice Minor: 1\n")
	writeDriverGPU(t, procRoot, "0000:00:03.0", l4Information)

	gpus, err := ReadDriverGPUs(procRoot)
	require.NoError(t, err)
	assert.Equal(t, []DriverGPU{
		{Minor: 0, UUID: "GPU-e80c8f33-55f5-55d2-18b7-64563f748393", Model: "NVIDIA L4", BusLocation: "0000:00:03.0"},
		{Minor: 1, UUID: "GPU-1072fdf3-18d9-8720-8b19-141e6afeb8f8", Model: "Tesla T4", BusLocation: "0000:00:05.0"},
	}, gpus)
}

func TestReadDriverGPUsWithoutDriver(t *testing.T) {
	gpus, err := ReadDriverGPUs(t.TempDir())
	assert.NoError(t, err)
	assert.Empty(t, gpus)
}

func TestReadDriverGPUsSkipsInvalidEntries(t *testing.T) {
	procRoot := t.TempDir()
	writeDriverGPU(t, procRoot, "0000:00:03.0", l4Information)
	writeDriverGPU(t, procRoot, "0000:00:04.0", "Model: NVIDIA L4\nDevice Minor: 1\n")
	writeDriverGPU(t, procRoot, "0000:00:05.0", "GPU UUID: GPU-abc\nDevice Minor: x\n")

	gpus, err := ReadDriverGPUs(procRoot)
	assert.Error(t, err)
	require.Len(t, gpus, 1)
	assert.Equal(t, "GPU-e80c8f33-55f5-55d2-18b7-64563f748393", gpus[0].UUID)
}
