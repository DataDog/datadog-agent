// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && test

package amd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readFixtureDir returns the files of a testdata directory as sysfs attributes.
func readFixtureDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	attrs := make(map[string]string, len(entries))
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		require.NoError(t, err)
		attrs[entry.Name()] = string(content)
	}
	return attrs
}

// mi350xVFHost lays out the real capture in testdata/mi350x_vf (an MI350X
// virtual function on a DigitalOcean AMD Developer Cloud droplet) the way the
// host exposes it: the PCI device behind card1, the amdgpu_xcp_* platform
// devices behind the other cards, and KFD node 1.
// Hardware serial and hive identifiers in the fixture are anonymized.
func mi350xVFHost(t *testing.T) *FakeSysfs {
	t.Helper()
	const root = "testdata/mi350x_vf"
	fs := NewFakeSysfs(t)

	devDir := fs.AddPCIDevice("0000:83:00.0", "amdgpu", readFixtureDir(t, filepath.Join(root, "device")))
	fs.AddHwmon(devDir, "hwmon0", readFixtureDir(t, filepath.Join(root, "hwmon")))
	fs.AddCard("card1", devDir)

	xcp, err := os.ReadFile(filepath.Join(root, "xcp_platform_devices"))
	require.NoError(t, err)
	for i, name := range strings.Fields(string(xcp)) {
		fs.AddCard("card"+string(rune('2'+i)), fs.AddPlatformDevice(name))
	}

	fs.AddKFDNode(0, 0, 0, 0, 0)
	kfd := readFixtureDir(t, filepath.Join(root, "kfd"))
	fs.WriteFiles(filepath.Join(fs.Root, "class", "kfd", "kfd", "topology", "nodes", "1"), map[string]string{
		"gpu_id":     kfd["gpu_id_node1"],
		"properties": kfd["properties_node1"],
	})
	fs.WriteFiles(filepath.Join(fs.Root, "class", "kfd", "kfd", "topology", "nodes", "1", "mem_banks", "0"), map[string]string{
		"properties": kfd["mem_bank0_node1_properties"],
	})
	return fs
}

func TestRealMI350XVirtualFunction(t *testing.T) {
	fs := mi350xVFHost(t)

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	require.Len(t, devices, 1, "the 7 amdgpu_xcp_* platform devices are not GPUs")
	dev := devices[0]

	assert.Equal(t, "amd-00c0ffee00c0ffee", dev.UUID)
	assert.Equal(t, "0000:83:00.0", dev.PCIBusID)
	assert.Equal(t, uint16(0x75b0), dev.DeviceID)
	assert.Equal(t, "AMD Instinct MI350X VF", dev.Name, "no product_name attribute on this host; the name comes from amdgpu.ids")
	assert.Equal(t, "gfx950", dev.Architecture)
	assert.Equal(t, uint64(308902100992), dev.MemoryTotal)
	assert.Equal(t, []uint64{29921}, dev.kfdGPUIDs)
	assert.Equal(t, []int{129}, dev.renderMinors)
	assert.Equal(t, uint32(2200), dev.MaxEngineClockMHz, "max_engine_clk_fcompute")
	assert.Equal(t, uint32(2000), dev.MaxMemoryClockMHz, "mem_clk_max of the VRAM bank")
	assert.Equal(t, uint32(8192), dev.MemoryBusWidthBits, "width of the VRAM bank (HBM3E)")
	assert.Equal(t, uint32(256), dev.ComputeUnits, "simd_count 1024 / simd_per_cu 4")

	metrics, err := dev.ReadMetrics()
	require.NoError(t, err)
	assert.Equal(t, Metrics{
		GPUBusyPercent:       valid(0),
		MemoryBusyPercent:    valid(0),
		VRAMTotalBytes:       valid(308902100992),
		VRAMUsedBytes:        valid(299544576),
		JunctionTemperatureC: valid(53), // no edge sensor on this device
		MemoryTemperatureC:   valid(41),
		PowerMilliwatts:      valid(254000), // power1_input only, no power1_average
		PowerCapMilliwatts:   valid(1000000),
		GraphicsClockMHz:     valid(49), // deep-sleep level "S: 49Mhz *"
		MemoryClockMHz:       valid(2000),
		PCIeLinkWidth:        valid(16),
		PCIeMaxLinkWidth:     valid(16),
		PCIeLinkSpeedGTs:     valid(32),
		PCIeMaxLinkSpeedGTs:  valid(32),
	}, metrics)
}
