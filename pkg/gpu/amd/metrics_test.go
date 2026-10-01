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

func discoverSingle(t *testing.T, fs *FakeSysfs) *Device {
	t.Helper()
	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	return devices[0]
}

func TestReadMetricsFullTelemetry(t *testing.T) {
	fs := NewFakeSysfs(t)
	devDir := fs.AddPCIDevice("0000:c1:00.0", "amdgpu", MI300XAttributes(""))
	fs.AddCard("card0", devDir)
	fs.AddHwmon(devDir, "hwmon3", map[string]string{
		"temp1_label":    "edge\n",
		"temp1_input":    "45000\n",
		"temp2_label":    "junction\n",
		"temp2_input":    "52500\n",
		"temp3_label":    "mem\n",
		"temp3_input":    "-1000\n",
		"power1_average": "300500000\n",
		"power1_input":   "1\n", // ignored when the average is exposed
		"power1_cap":     "750000000\n",
	})

	m, err := discoverSingle(t, fs).ReadMetrics()
	require.NoError(t, err)

	assert.Equal(t, Metrics{
		GPUBusyPercent:       valid(37),
		MemoryBusyPercent:    valid(12),
		VRAMTotalBytes:       valid(206141652992),
		VRAMUsedBytes:        valid(294965248),
		EdgeTemperatureC:     valid(45),
		JunctionTemperatureC: valid(52.5),
		MemoryTemperatureC:   valid(-1),
		PowerMilliwatts:      valid(300500),
		PowerCapMilliwatts:   valid(750000),
		GraphicsClockMHz:     valid(1420),
		MemoryClockMHz:       valid(1300),
		PCIeLinkWidth:        valid(16),
		PCIeMaxLinkWidth:     valid(16),
		PCIeLinkSpeedGTs:     valid(32),
		PCIeMaxLinkSpeedGTs:  valid(32),
	}, m)
}

func TestReadMetricsMissingAttributesAreInvalidNotErrors(t *testing.T) {
	fs := NewFakeSysfs(t)
	attrs := MI300XAttributes("")
	delete(attrs, "gpu_busy_percent")
	delete(attrs, "mem_busy_percent")
	attrs["pp_dpm_mclk"] = "0: 900Mhz\n1: 1300Mhz\n" // no active level
	attrs["current_link_speed"] = "Unknown\n"
	devDir := fs.AddPCIDevice("0000:c1:00.0", "amdgpu", attrs)
	fs.AddCard("card0", devDir)
	fs.AddHwmon(devDir, "hwmon0", JunctionOnlyHwmon())

	m, err := discoverSingle(t, fs).ReadMetrics()
	require.NoError(t, err)

	assert.False(t, m.GPUBusyPercent.Valid)
	assert.False(t, m.MemoryBusyPercent.Valid)
	assert.False(t, m.MemoryClockMHz.Valid)
	assert.False(t, m.PCIeLinkSpeedGTs.Valid)
	assert.False(t, m.EdgeTemperatureC.Valid)
	assert.Equal(t, valid(41), m.JunctionTemperatureC)
	assert.Equal(t, valid(35), m.MemoryTemperatureC)
	assert.Equal(t, valid(142000), m.PowerMilliwatts, "falls back to power1_input")
	assert.Equal(t, valid(1420), m.GraphicsClockMHz)
}

func TestReadMetricsWithoutHwmon(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:c1:00.0", "amdgpu", MI300XAttributes("")))

	m, err := discoverSingle(t, fs).ReadMetrics()
	require.NoError(t, err)
	assert.False(t, m.JunctionTemperatureC.Valid)
	assert.False(t, m.PowerMilliwatts.Valid)
	assert.False(t, m.PowerCapMilliwatts.Valid)
	assert.Equal(t, valid(37), m.GPUBusyPercent)
}

func TestReadMetricsParseErrorsKeepOtherValues(t *testing.T) {
	fs := NewFakeSysfs(t)
	attrs := MI300XAttributes("")
	attrs["gpu_busy_percent"] = "busy\n"
	attrs["pp_dpm_sclk"] = "0: fastMhz *\n"
	attrs["max_link_speed"] = "fast GT/s PCIe\n"
	fs.AddCard("card0", fs.AddPCIDevice("0000:c1:00.0", "amdgpu", attrs))

	m, err := discoverSingle(t, fs).ReadMetrics()
	require.Error(t, err)
	assert.ErrorContains(t, err, "gpu_busy_percent")
	assert.ErrorContains(t, err, "pp_dpm_sclk")
	assert.ErrorContains(t, err, "max_link_speed")

	assert.False(t, m.GPUBusyPercent.Valid)
	assert.False(t, m.GraphicsClockMHz.Valid)
	assert.False(t, m.PCIeMaxLinkSpeedGTs.Valid)
	assert.Equal(t, valid(12), m.MemoryBusyPercent)
	assert.Equal(t, valid(206141652992), m.VRAMTotalBytes)
}

// The accepted formats follow what ROCm SMI parses from pp_dpm_* files
// (rocm_smi/src/rocm_smi.cc and tests/amd_smi_test/unit/gpu/clock_range_test.cc
// in ROCm/rocm-systems projects/amdsmi).
func TestParseDPMFrequency(t *testing.T) {
	for _, tc := range []struct {
		line    string
		mhz     float64
		wantErr bool
	}{
		{line: "1: 1700Mhz *", mhz: 1700},
		{line: "S: 19Mhz *", mhz: 19},
		{line: "1:       1837Mhz *\n", mhz: 1837},
		{line: "2: 1300 Mhz *", mhz: 1300},
		{line: "0: 1.5Ghz *", mhz: 1500},
		{line: "0: 800000Khz *", mhz: 800},
		{line: "0: 900MHz*", mhz: 900},
		{line: "1700Mhz *", wantErr: true},
		{line: "0: Mhz *", wantErr: true},
		{line: "0: 900 *", wantErr: true},
	} {
		t.Run(tc.line, func(t *testing.T) {
			mhz, err := parseDPMFrequency(tc.line)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.mhz, mhz)
		})
	}
}

func TestReadMetricsLinkSpeeds(t *testing.T) {
	for _, tc := range []struct {
		content string
		want    Reading
		wantErr bool
	}{
		{content: "2.5 GT/s PCIe\n", want: valid(2.5)},
		{content: "8.0 GT/s PCIe\n", want: valid(8)},
		{content: "16.0 GT/s\n", want: valid(16)},
		{content: "Unknown\n"},
		{content: "Unknown speed\n"},
		{content: "garbage\n", wantErr: true},
		{content: "8.0 GT/s nonsense\n", wantErr: true},
		{content: "NaN GT/s PCIe\n", wantErr: true},
		{content: "+Inf GT/s PCIe\n", wantErr: true},
		{content: "-8.0 GT/s PCIe\n", wantErr: true},
		{content: "0 GT/s PCIe\n", wantErr: true},
	} {
		t.Run(strings.TrimSpace(tc.content), func(t *testing.T) {
			fs := NewFakeSysfs(t)
			attrs := MI300XAttributes("")
			attrs["current_link_speed"] = tc.content
			fs.AddCard("card0", fs.AddPCIDevice("0000:c1:00.0", "amdgpu", attrs))
			m, err := discoverSingle(t, fs).ReadMetrics()
			if tc.wantErr {
				require.ErrorContains(t, err, "current_link_speed")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.want, m.PCIeLinkSpeedGTs)
			assert.Equal(t, valid(37), m.GPUBusyPercent)
		})
	}
}

func TestReadMetricsReportsHwmonEnumerationErrors(t *testing.T) {
	fs := NewFakeSysfs(t)
	devDir := fs.AddPCIDevice("0000:c1:00.0", "amdgpu", MI300XAttributes(""))
	fs.AddCard("card0", devDir)
	// A non-directory causes ReadDir to fail even when tests run as root.
	fs.WriteFiles(devDir, map[string]string{"hwmon": "not a directory"})

	m, err := discoverSingle(t, fs).ReadMetrics()
	require.ErrorContains(t, err, "hwmon")
	assert.False(t, m.PowerMilliwatts.Valid)
	assert.Equal(t, valid(37), m.GPUBusyPercent)
}

func TestReadMetricsReportsPermissionErrors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read files without read permission")
	}
	fs := NewFakeSysfs(t)
	devDir := fs.AddPCIDevice("0000:c1:00.0", "amdgpu", MI300XAttributes(""))
	fs.AddCard("card0", devDir)
	require.NoError(t, os.Chmod(filepath.Join(devDir, "gpu_busy_percent"), 0))

	m, err := discoverSingle(t, fs).ReadMetrics()
	require.ErrorIs(t, err, os.ErrPermission)
	assert.False(t, m.GPUBusyPercent.Valid)
	assert.Equal(t, valid(12), m.MemoryBusyPercent)
}
