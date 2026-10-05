// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package amd

import (
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sysfsSeeds are attribute contents seen on real devices, plus malformed ones.
var sysfsSeeds = []string{
	"37\n",
	"",
	"\n",
	"\x00",
	"15000000\n" + strings.Repeat("\x00", 1011), // NUL-padded hwmon attribute (Phoenix APU)
	"-1\n",
	"18446744073709551616\n", // overflows uint64
	"0x1002\n",
	"Unknown\n",
	"not a number",
}

// requireFiniteReadings checks that every valid reading is a finite number:
// a NaN or infinity must be reported as unavailable, never sent as a metric.
func requireFiniteReadings(t *testing.T, m Metrics) {
	v := reflect.ValueOf(m)
	for i := range v.NumField() {
		r, ok := v.Field(i).Interface().(Reading)
		if !ok || !r.Valid {
			continue
		}
		assert.False(t, math.IsNaN(r.Value) || math.IsInf(r.Value, 0), "%s = %v", v.Type().Field(i).Name, r.Value)
	}
}

// FuzzReadMetrics checks that no attribute content makes ReadMetrics panic or
// report a non-finite value.
func FuzzReadMetrics(f *testing.F) {
	for _, seed := range sysfsSeeds {
		f.Add(seed, "1: 1420Mhz *\n", "32.0 GT/s PCIe\n", "edge\n")
	}
	f.Add("37\n", "S: 19Mhz *\n0: 872Mhz\n", "Unknown speed\n", "junction\n")
	f.Add("37\n", "1: 1.5GHz *\n", "16.0 GT/s\n", "mem\n")
	f.Add("37\n", "1: "+strings.Repeat("9", 308)+"Ghz *\n", "1e309 GT/s PCIe\n", "edge\n") // overflow to +Inf
	f.Add("37\n", "*\n:\n1: *\n1: Mhz *\n", "NaN GT/s PCIe\n", "")

	f.Fuzz(func(t *testing.T, number, dpmTable, linkSpeed, label string) {
		fs := NewFakeSysfs(t)
		attrs := MI300XAttributes("")
		for _, name := range []string{"gpu_busy_percent", "mem_busy_percent", "mem_info_vram_total", "mem_info_vram_used", "current_link_width", "max_link_width"} {
			attrs[name] = number
		}
		attrs["pp_dpm_sclk"] = dpmTable
		attrs["pp_dpm_mclk"] = dpmTable
		attrs["current_link_speed"] = linkSpeed
		attrs["max_link_speed"] = linkSpeed
		devDir := fs.AddPCIDevice("0000:c1:00.0", "amdgpu", attrs)
		fs.AddCard("card0", devDir)
		fs.AddHwmon(devDir, "hwmon0", map[string]string{
			"temp1_label":    label,
			"temp1_input":    number,
			"power1_average": number,
			"power1_cap":     number,
		})

		devices, err := Discover(fs.Root)
		require.NoError(t, err)
		require.Len(t, devices, 1)
		m, _ := devices[0].ReadMetrics()
		requireFiniteReadings(t, m)
	})
}

// FuzzDiscover checks that no PCI, KFD topology or KFD process content makes
// discovery or process attribution panic.
func FuzzDiscover(f *testing.F) {
	properties := "cpu_cores_count 0\ngfx_target_version 90402\nlocation_id 49408\ndomain 0\nsimd_count 1216\nsimd_per_cu 4\n"
	for _, seed := range sysfsSeeds {
		f.Add("0x1002\n", "0x74a1\n", seed, seed, properties, "4101\n", seed)
	}
	f.Add("0x1002\n", "0x74a1\n", "00c0ffee00c0ffee\n", "AMD Instinct MI300X\n", properties, "4101\n", "42\n")
	f.Add("0x10de\n", "0x2330\n", "", "", "", "", "")
	f.Add("0x1002\n", "0xzzzz\n", "0\n", "\x00", "location_id 99999999\ndomain 99999999999\nsimd_per_cu 0\n", "0\n", "x")

	f.Fuzz(func(t *testing.T, vendor, device, uniqueID, productName, kfdProperties, gpuID, vram string) {
		fs := NewFakeSysfs(t)
		attrs := MI300XAttributes("")
		attrs["vendor"] = vendor
		attrs["device"] = device
		attrs["unique_id"] = uniqueID
		attrs["product_name"] = productName
		fs.AddCard("card0", fs.AddPCIDevice("0000:c1:00.0", "amdgpu", attrs))
		fs.AddKFDNode(1, 4101, 0, 0xc100, 90402)
		fs.WriteFiles(filepath.Join(fs.Root, "class", "kfd", "kfd", "topology", "nodes", "1"), map[string]string{
			"properties": kfdProperties,
			"gpu_id":     gpuID,
		})
		fs.AddKFDProcess(100, 4101, 0)
		fs.WriteFiles(filepath.Join(fs.Root, "class", "kfd", "kfd", "proc", "100"), map[string]string{"vram_4101": vram})

		devices, _ := Discover(fs.Root)
		for _, dev := range devices {
			assert.True(t, strings.HasPrefix(dev.UUID, Vendor+"-"), "UUID %q", dev.UUID)
		}
		_ = KFDAccessDeniedWarning(devices)
		_, _, _ = ReadProcessMemory(fs.Root, devices)
	})
}

// FuzzParseDPMFrequency checks that a parsed pp_dpm_* level is a finite,
// non-negative frequency.
func FuzzParseDPMFrequency(f *testing.F) {
	for _, seed := range []string{"1: 1700Mhz *", "S: 19Mhz *", "0:        872Mhz", "1: 1.5GHz", "2: 900 MHz", "1: 800khz", "", ":", "1:", "1: Mhz", "1: 1.2.3Mhz", "1: " + strings.Repeat("9", 308) + "Ghz"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, line string) {
		mhz, err := parseDPMFrequency(line)
		if err != nil {
			return
		}
		assert.False(t, math.IsNaN(mhz) || math.IsInf(mhz, 0) || mhz < 0, "%q parsed as %v", line, mhz)
	})
}
