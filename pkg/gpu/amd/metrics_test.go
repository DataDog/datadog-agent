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
	"syscall"
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

// A Phoenix APU (kernel 6.12) pads power1_cap with NULs after the newline.
func TestReadMetricsNULPaddedAttribute(t *testing.T) {
	fs := NewFakeSysfs(t)
	devDir := fs.AddPCIDevice("0000:09:00.0", "amdgpu", MI300XAttributes(""))
	fs.AddCard("card0", devDir)
	fs.AddHwmon(devDir, "hwmon4", map[string]string{
		"power1_input": "5097000\n",
		"power1_cap":   "15000000\n" + strings.Repeat("\x00", 1011),
	})

	m, err := discoverSingle(t, fs).ReadMetrics()
	require.NoError(t, err)
	assert.Equal(t, valid(15000), m.PowerCapMilliwatts)
	assert.Equal(t, valid(5097), m.PowerMilliwatts)
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

// hwmon reports the SMU's current clocks. Since Linux 7.0 and amdgpu-dkms
// 6.19.14, pp_dpm_* of AMD Instinct GPUs can mark no level while the clock is
// between two (an MI300X VF under load showed UCLK at 1270 MHz between its 1100
// and 1300 MHz levels).
func TestReadMetricsClocks(t *testing.T) {
	const unmarkedSclk = "0: 500Mhz \n1: 2200Mhz \n"
	clocksHwmon := map[string]string{
		"freq1_label": "sclk\n",
		"freq1_input": "1933000000\n",
		"freq2_label": "mclk\n",
		"freq2_input": "1270000000\n",
	}
	for name, tc := range map[string]struct {
		sclk          string // pp_dpm_sclk content, absent when empty
		unreadable    bool   // pp_dpm_sclk is a directory
		hwmon         map[string]string
		graphics, mem Reading
		wantErr       string
	}{
		"hwmon preferred":           {sclk: "0: 500Mhz \n1: 1420Mhz *\n2: 2200Mhz \n", hwmon: clocksHwmon, graphics: valid(1933), mem: valid(1270)},
		"no active level":           {sclk: unmarkedSclk, hwmon: clocksHwmon, graphics: valid(1933), mem: valid(1270)},
		"no hwmon clocks":           {sclk: "0: 500Mhz \n1: 1420Mhz *\n", hwmon: map[string]string{}, graphics: valid(1420), mem: valid(1300)},
		"no hwmon clocks, unmarked": {sclk: unmarkedSclk, hwmon: map[string]string{}, mem: valid(1300)},
		"sclk input not exposed":    {sclk: "S: 49Mhz *\n1: 500Mhz \n", hwmon: map[string]string{"freq1_label": "sclk\n"}, graphics: valid(49), mem: valid(1300)},
		"unparsable sclk input":     {sclk: "1: 1420Mhz *\n", hwmon: map[string]string{"freq1_label": "sclk\n", "freq1_input": "fast\n"}, graphics: valid(1420), mem: valid(1300), wantErr: "freq1_input"},
		"RAS interrupt":             {sclk: "unavailable\n", hwmon: clocksHwmon, mem: valid(1270)},                    // hwmon then returns a value the driver did not set
		"unreadable pp_dpm_sclk":    {unreadable: true, hwmon: clocksHwmon, mem: valid(1270), wantErr: "pp_dpm_sclk"}, // EBUSY in a RAS interrupt or reset
		"sclk label on second freq": {hwmon: map[string]string{"freq1_label": "mclk\n", "freq1_input": "2000000000\n", "freq2_label": "sclk\n", "freq2_input": "85000000\n"}, graphics: valid(85), mem: valid(2000)},
	} {
		t.Run(name, func(t *testing.T) {
			fs := NewFakeSysfs(t)
			attrs := MI300XAttributes("")
			delete(attrs, "pp_dpm_sclk")
			if tc.sclk != "" {
				attrs["pp_dpm_sclk"] = tc.sclk
			}
			devDir := fs.AddPCIDevice("0000:c1:00.0", "amdgpu", attrs)
			fs.AddCard("card0", devDir)
			fs.AddHwmon(devDir, "hwmon0", tc.hwmon)
			if tc.unreadable {
				require.NoError(t, os.Mkdir(filepath.Join(devDir, "pp_dpm_sclk"), 0o755))
			}

			m, err := discoverSingle(t, fs).ReadMetrics()
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.graphics, m.GraphicsClockMHz)
			assert.Equal(t, tc.mem, m.MemoryClockMHz)
		})
	}
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

func TestReadMetricsSuspendedDeviceIsIdle(t *testing.T) {
	for name, tc := range map[string]struct {
		err           error
		runtimeStatus string
		want          Reading
		wantErr       bool
	}{
		"runtime-suspended":  {err: syscall.EPERM, runtimeStatus: "suspended\n", want: valid(0)},
		"suspending":         {err: syscall.EPERM, runtimeStatus: "suspending\n", want: valid(0)},
		"resuming":           {err: syscall.EPERM, runtimeStatus: "resuming\n", want: valid(0)},
		"EPERM while active": {err: syscall.EPERM, runtimeStatus: "active\n", wantErr: true}, // e.g. a GPU reset
		"EPERM, no status":   {err: syscall.EPERM, wantErr: true},
		"permission denied":  {err: syscall.EACCES, runtimeStatus: "suspended\n", wantErr: true},
		"unsupported":        {err: syscall.EOPNOTSUPP},
	} {
		t.Run(name, func(t *testing.T) {
			fs := NewFakeSysfs(t)
			devDir := fs.AddPCIDevice("0000:c1:00.0", "amdgpu", MI300XAttributes(""))
			fs.AddCard("card0", devDir)
			if tc.runtimeStatus != "" {
				fs.WriteFiles(filepath.Join(devDir, "power"), map[string]string{"runtime_status": tc.runtimeStatus})
			}
			readActivityFile = func(path string) ([]byte, error) {
				return nil, &os.PathError{Op: "read", Path: path, Err: tc.err}
			}
			t.Cleanup(func() { readActivityFile = os.ReadFile })
			m, err := discoverSingle(t, fs).ReadMetrics()
			if tc.wantErr {
				assert.ErrorIs(t, err, tc.err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.want, m.GPUBusyPercent)
			assert.Equal(t, tc.want, m.MemoryBusyPercent)
		})
	}
}

// amdgpu returns EPERM for gpu_metrics while the device is runtime-suspended,
// which is no error, but also during a GPU reset, which is one.
func TestReadMetricsSuspendedDeviceGPUMetrics(t *testing.T) {
	for name, tc := range map[string]struct {
		runtimeStatus string
		wantErr       bool
	}{
		"runtime-suspended":  {runtimeStatus: "suspended\n"},
		"resuming":           {runtimeStatus: "resuming\n"},
		"EPERM while active": {runtimeStatus: "active\n", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			fs := NewFakeSysfs(t)
			devDir := fs.AddPCIDevice("0000:c1:00.0", "amdgpu", MI300XAttributes(""))
			fs.AddCard("card0", devDir)
			fs.WriteFiles(filepath.Join(devDir, "power"), map[string]string{"runtime_status": tc.runtimeStatus})
			fs.WriteFiles(devDir, map[string]string{"gpu_metrics": ""})
			readGPUMetricsFile = func(path string, _ int64) ([]byte, error) {
				return nil, &os.PathError{Op: "read", Path: path, Err: syscall.EPERM}
			}
			t.Cleanup(func() { readGPUMetricsFile = readFileLimited })
			m, err := discoverSingle(t, fs).ReadMetrics()
			if tc.wantErr {
				assert.ErrorIs(t, err, syscall.EPERM)
			} else {
				assert.NoError(t, err)
			}
			assert.Nil(t, m.GPUMetrics)
			assert.NoError(t, m.GPUMetricsErr)
		})
	}
}

func TestReadMetricsZeroLinkWidthIsUnavailable(t *testing.T) {
	fs := NewFakeSysfs(t)
	attrs := MI300XAttributes("")
	attrs["current_link_width"] = "0\n"
	attrs["max_link_width"] = "0\n"
	fs.AddCard("card0", fs.AddPCIDevice("0000:c1:00.0", "amdgpu", attrs))
	m, err := discoverSingle(t, fs).ReadMetrics()
	require.NoError(t, err)
	assert.False(t, m.PCIeLinkWidth.Valid)
	assert.False(t, m.PCIeMaxLinkWidth.Valid)
}

// The contents follow amdgpu_ras_sysfs_read, amdgpu_ras_aca_sysfs_read and
// amdgpu_ras_sysfs_features_read in amdgpu_ras.c.
func TestReadMetricsECC(t *testing.T) {
	const features = "feature mask: 0x20065bff\n" // MI300X VF: every block amd-smi counts
	for name, tc := range map[string]struct {
		ras                              map[string]string // no ras directory when nil
		uncorrected, corrected, deferred Reading
		wantErr                          string
	}{
		"no ras directory":   {},
		"no count attribute": {ras: map[string]string{"features": features}},
		"MI300X VF without errors": {
			ras: map[string]string{
				"features":           features,
				"umc_err_count":      "ue: 0\nce: 0\nde: 0\n",
				"gfx_err_count":      "ue: 0\nce: 0\n",
				"sdma_err_count":     "ue: 0\nce: 0\n",
				"mmhub_err_count":    "ue: 0\nce: 0\n",
				"pcie_bif_err_count": "ue: 0\nce: 0\n",
			},
			uncorrected: valid(0), corrected: valid(0), deferred: valid(0),
		},
		"errors in several blocks": {
			ras: map[string]string{
				"features":            features,
				"umc_err_count":       "ue: 2\nce: 7\nde: 1\n",
				"gfx_err_count":       "ue: 1\nce: 0\n",
				"sdma_err_count":      "ue: 0\nce: 3\n",
				"xgmi_wafl_err_count": "ue: 0\nce: 4\n",
				"vcn_err_count":       "ue: 5\nce: 5\n", // not counted by amd-smi
			},
			uncorrected: valid(3), corrected: valid(14), deferred: valid(1),
		},
		"ACA attributes only": { // fallback when <block>_err_count is absent
			ras: map[string]string{
				"features":  features,
				"aca_umc":   "ue: 1\nce: 6\nde: 2\n",
				"aca_gfx":   "ue: 0\nce: 1\nde: 1\n",
				"aca_mmhub": "ue: 0\nce: 0\nde: 0\n",
			},
			uncorrected: valid(1), corrected: valid(7), deferred: valid(3),
		},
		"block disabled in the feature mask": {
			ras: map[string]string{
				"features":      "feature mask: 0x1\n", // UMC only
				"umc_err_count": "ue: 1\nce: 2\nde: 0\n",
				"gfx_err_count": "ue: 5\nce: 5\n",
			},
			uncorrected: valid(1), corrected: valid(2), deferred: valid(0),
		},
		"no feature mask": {
			ras: map[string]string{"umc_err_count": "ue: 1\nce: 2\nde: 0\n"},
		},
		"unparsable feature mask": {
			ras: map[string]string{
				"features":      "garbage\n",
				"umc_err_count": "ue: 1\nce: 2\nde: 0\n",
			},
			wantErr: "features",
		},
		"no deferred count": {
			ras:         map[string]string{"features": features, "gfx_err_count": "ue: 1\nce: 2\n"},
			uncorrected: valid(1), corrected: valid(2),
		},
		"reset starting during the scan": {
			ras: map[string]string{
				"features":      features,
				"umc_err_count": "ue: 1\nce: 2\nde: 0\n",
				"gfx_err_count": "Query currently inaccessible\n",
			},
		},
		"unparsable block": {
			ras: map[string]string{
				"features":      features,
				"umc_err_count": "ue: 2\nce: many\nde: 0\n",
				"gfx_err_count": "ue: 1\nce: 2\n",
			},
			wantErr: "umc_err_count",
		},
		"unparsable later block": {
			ras: map[string]string{
				"features":      features,
				"umc_err_count": "ue: 2\nce: 0\nde: 0\n",
				"gfx_err_count": "ue: 1\n",
			},
			wantErr: "gfx_err_count",
		},
	} {
		t.Run(name, func(t *testing.T) {
			fs := NewFakeSysfs(t)
			devDir := fs.AddPCIDevice("0000:c1:00.0", "amdgpu", MI300XAttributes(""))
			fs.AddCard("card0", devDir)
			if tc.ras != nil {
				fs.WriteFiles(filepath.Join(devDir, "ras"), tc.ras)
			}

			m, err := discoverSingle(t, fs).ReadMetrics()
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.uncorrected, m.ECCUncorrected)
			assert.Equal(t, tc.corrected, m.ECCCorrected)
			assert.Equal(t, tc.deferred, m.ECCDeferred)
			assert.Equal(t, valid(37), m.GPUBusyPercent, "other metrics are unaffected")
		})
	}
}

// amdgpu_ras_sysfs_read fails with EINVAL when the error query fails, such as
// during a GPU reset: no partial sum is reported.
func TestReadMetricsECCReadFailure(t *testing.T) {
	fs := NewFakeSysfs(t)
	devDir := fs.AddPCIDevice("0000:c1:00.0", "amdgpu", MI300XAttributes(""))
	fs.AddCard("card0", devDir)
	fs.WriteFiles(filepath.Join(devDir, "ras"), map[string]string{
		"features":      "feature mask: 0x20065bff\n",
		"umc_err_count": "ue: 1\nce: 2\nde: 0\n",
		"gfx_err_count": "ue: 0\nce: 0\n",
	})
	readRASFile = func(path string) ([]byte, error) {
		if filepath.Base(path) == "gfx_err_count" {
			return nil, &os.PathError{Op: "read", Path: path, Err: syscall.EINVAL}
		}
		return os.ReadFile(path)
	}
	t.Cleanup(func() { readRASFile = os.ReadFile })

	m, err := discoverSingle(t, fs).ReadMetrics()
	require.ErrorIs(t, err, syscall.EINVAL)
	assert.False(t, m.ECCUncorrected.Valid)
	assert.False(t, m.ECCCorrected.Valid)
	assert.False(t, m.ECCDeferred.Valid)
	assert.Equal(t, valid(37), m.GPUBusyPercent, "other metrics are unaffected")
}
