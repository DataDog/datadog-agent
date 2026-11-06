// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && test

package amd

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readGPUMetricsFixture(t testing.TB, dir string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", dir, "device", "gpu_metrics"))
	require.NoError(t, err)
	return data
}

func counters(values ...uint64) []Counter {
	out := make([]Counter, len(values))
	for i, v := range values {
		out[i] = Counter{Value: v, Valid: true}
	}
	return out
}

func filled(v uint64) Counter { return Counter{Value: v, Valid: true} }

// The MI300X VF table (amdgpu-dkms 6.19.14) has 50 attributes; IDs 48, 50 and
// 51 are unfilled.
func TestParseGPUMetricsMI300XVF(t *testing.T) {
	gm, err := ParseGPUMetrics(readGPUMetricsFixture(t, "mi300x_vf"))
	require.NoError(t, err)
	assert.Equal(t, &GPUMetrics{
		Version:                GPUMetricsVersion{Format: 1, Content: 9},
		TemperatureHotspotC:    filled(39),
		SocketPowerW:           filled(144),
		PCIeLinkWidth:          filled(16),
		MemoryClockMHz:         filled(900),
		SystemClockNs:          filled(72203597569180),
		AccumulationCounter:    filled(2583688127),
		ProchotResidency:       filled(0),
		PPTResidency:           filled(350407088),
		SocketThermalResidency: filled(12105768),
		VRThermalResidency:     filled(0),
		HBMThermalResidency:    filled(0),
		PCIeReplayCount:        filled(0),
		GFXActivityAcc:         filled(1336391183), // U32: wrapped
		MemActivityAcc:         filled(2862819467),
		XGMIReadKB:             counters(0, 1, 2, 1, 1, 1, 1, 1),
		XGMIWriteKB:            counters(0, 1, 1, 1, 1, 1, 1, 1),
	}, gm)
}

// The MI350X VF table (kernel 7.0) has 47 attributes and no NUM_PARTITION.
func TestParseGPUMetricsMI350XVF(t *testing.T) {
	gm, err := ParseGPUMetrics(readGPUMetricsFixture(t, "mi350x_vf"))
	require.NoError(t, err)
	assert.Equal(t, &GPUMetrics{
		Version:                GPUMetricsVersion{Format: 1, Content: 9},
		TemperatureHotspotC:    filled(53),
		SocketPowerW:           filled(253),
		PCIeLinkWidth:          filled(16),
		MemoryClockMHz:         filled(2000),
		SystemClockNs:          filled(266856137538),
		AccumulationCounter:    filled(36392871),
		ProchotResidency:       filled(0),
		PPTResidency:           filled(23598934),
		SocketThermalResidency: filled(0),
		VRThermalResidency:     filled(0),
		HBMThermalResidency:    filled(0),
		PCIeReplayCount:        filled(0),
		GFXActivityAcc:         filled(2889439947),
		MemActivityAcc:         filled(715102902),
		XGMIReadKB:             counters(0, 53615, 17933, 71768, 0, 0, 35882, 53828),
		XGMIWriteKB:            counters(0, 17859, 17936, 17943, 17934, 17943, 17939, 17944),
	}, gm)
}

// mi300xVFHost lays out the MI300X capture in testdata/mi300x_vf as a PCI device
// with its hwmon directory.
func mi300xVFHost(t *testing.T) *FakeSysfs {
	t.Helper()
	fs := NewFakeSysfs(t)
	attrs := MI300XAttributes("")
	for name, content := range readFixtureDir(t, filepath.Join("testdata", "mi300x_vf", "device")) {
		attrs[name] = content
	}
	devDir := fs.AddPCIDevice("0000:83:00.0", "amdgpu", attrs)
	fs.AddHwmon(devDir, "hwmon0", readFixtureDir(t, filepath.Join("testdata", "mi300x_vf", "hwmon")))
	fs.AddCard("card1", devDir)
	return fs
}

func TestRealGPUMetricsCalibrate(t *testing.T) {
	for name, host := range map[string]func(*testing.T) *FakeSysfs{"mi300x_vf": mi300xVFHost, "mi350x_vf": mi350xVFHost} {
		t.Run(name, func(t *testing.T) {
			devices, err := Discover(host(t).Root)
			require.NoError(t, err)
			require.Len(t, devices, 1)
			m, err := devices[0].ReadMetrics()
			require.NoError(t, err)
			require.NoError(t, m.GPUMetricsErr)
			require.NotNil(t, m.GPUMetrics)
			checked, err := m.GPUMetrics.Calibrate(&m)
			require.NoError(t, err)
			assert.Equal(t, 4, checked, "power, hotspot, link width and memory clock")
		})
	}
}

func TestReadMetricsSkipsGPUMetrics(t *testing.T) {
	fs := mi300xVFHost(t)
	require.NoError(t, os.MkdirAll(filepath.Join(fs.Root, "bus", "pci", "devices", "0000:83:00.0", "xgmi_hive_info", "node1"), 0o755))
	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	m, err := devices[0].ReadMetricsWithOptions(ReadOptions{SkipGPUMetrics: true})
	require.NoError(t, err)
	assert.Nil(t, m.GPUMetrics)
	assert.NoError(t, m.GPUMetricsErr)
	assert.Zero(t, m.XGMIHiveSize)
	assert.True(t, m.PowerMilliwatts.Valid, "the other attributes are read")
}

func TestReadMetricsWithoutGPUMetrics(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:c1:00.0", "amdgpu", MI300XAttributes("")))
	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	m, err := devices[0].ReadMetrics()
	require.NoError(t, err)
	assert.Nil(t, m.GPUMetrics)
	assert.NoError(t, m.GPUMetricsErr)
}

func pptAttr(v uint64) testAttr {
	return testAttr{attrPPTResidencyAcc, typeU32, unitNone, []uint64{v}}
}

func TestParseGPUMetricsUnsupportedVersions(t *testing.T) {
	versions := []GPUMetricsVersion{
		{1, 0}, {1, 1}, {1, 3}, {1, 4}, {1, 5}, {1, 6}, {1, 7}, {1, 8}, // fixed layouts
		{2, 0}, {2, 4}, {3, 0}, // APUs
		{0, 9}, {4, 9}, {255, 255}, // unknown
	}
	for _, v := range versions {
		t.Run(v.String(), func(t *testing.T) {
			// A table with a valid dynamic layout must still not be decoded.
			data := buildGPUMetrics(v.Format, v.Content, pptAttr(42))
			gm, err := ParseGPUMetrics(data)
			require.ErrorIs(t, err, ErrGPUMetricsUnsupported)
			assert.Equal(t, &GPUMetrics{Version: v}, gm, "only the header is read")
		})
	}
}

func TestParseGPUMetricsFutureDynamicVersion(t *testing.T) {
	gm, err := ParseGPUMetrics(buildGPUMetrics(1, 10, pptAttr(42)))
	require.NoError(t, err)
	assert.Equal(t, filled(42), gm.PPTResidency)
}

func TestParseGPUMetricsMalformed(t *testing.T) {
	valid := buildGPUMetrics(1, 9, pptAttr(42), testAttr{attrXGMIReadDataAcc, typeU64, unitDataKB, make([]uint64, 8)})
	withSize := func(data []byte, size int) []byte {
		binary.LittleEndian.PutUint16(data[0:], uint16(size))
		return data
	}
	withCount := func(count int32) []byte {
		data := clone(valid)
		binary.LittleEndian.PutUint32(data[4:], uint32(count))
		return data
	}
	withEncoding := func(encoding uint64) []byte {
		data := clone(valid)
		binary.LittleEndian.PutUint64(data[gpuMetricsHeaderSize:], encoding)
		return data
	}
	ppt := encodeAttr(pptAttr(0))
	cases := map[string][]byte{
		"empty":                   {},
		"truncated header":        {8, 0, 1},
		"header only":             withSize([]byte{0, 0, 1, 9}, 4),
		"size above length":       withSize(clone(valid), len(valid)+1),
		"size below length":       withSize(clone(valid), len(valid)-1),
		"trailing byte":           withSize(append(clone(valid), 0), len(valid)+1),
		"truncated values":        withSize(clone(valid[:len(valid)-1]), len(valid)-1),
		"zero attributes":         withCount(0),
		"negative count":          withCount(-1),
		"count above size":        withCount(1000),
		"count above content":     withCount(3),
		"count below content":     withCount(1),
		"unknown type":            withEncoding(ppt&^(0xf<<20) | 8<<20),
		"zero instances":          withEncoding(ppt &^ 0x3ff),
		"instances overrun":       withEncoding(ppt | 0x3ff),
		"high encoding bits":      withEncoding(ppt | 1<<40),
		"wider type misaligns":    withEncoding(ppt&^(0xf<<20) | uint64(typeU64)<<20),
		"narrower type misaligns": withEncoding(ppt&^(0xf<<20) | uint64(typeU16)<<20),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			gm, err := ParseGPUMetrics(data)
			require.ErrorIs(t, err, ErrGPUMetricsMalformed)
			requireNoGPUMetricsValues(t, gm)
		})
	}
}

func clone(b []byte) []byte { return append([]byte(nil), b...) }

// requireNoGPUMetricsValues checks that a table carries no value.
func requireNoGPUMetricsValues(t *testing.T, gm *GPUMetrics) {
	t.Helper()
	if gm == nil {
		return
	}
	assert.Equal(t, &GPUMetrics{Version: gm.Version}, gm)
}

func TestParseGPUMetricsFieldSignatures(t *testing.T) {
	gm, err := ParseGPUMetrics(buildGPUMetrics(1, 9,
		testAttr{attrPPTResidencyAcc, typeU16, unitNone, []uint64{42}},                        // wrong type
		testAttr{attrSocketThmResidencyAcc, typeU32, unitPowerW, []uint64{42}},                // wrong unit
		testAttr{attrXGMIReadDataAcc, typeU64, unitDataKB, make([]uint64, 16)},                // wrong instance count
		testAttr{attrHBMThmResidencyAcc, typeU32, unitNone, []uint64{1}},                      // duplicated
		testAttr{attrHBMThmResidencyAcc, typeU32, unitNone, []uint64{2}},                      //
		testAttr{attrXGMIWriteDataAcc, typeU64, unitDataKB, []uint64{1, 2, 3, 4, 5, 6, 7, 8}}, // valid
		testAttr{attrAccumulationCounter, typeU64, unitNone, []uint64{math.MaxUint64}},        // unfilled
		testAttr{attrVRThmResidencyAcc, typeU32, unitNone, []uint64{math.MaxUint32}},          // unfilled
		testAttr{attrProchotResidencyAcc, typeU32, unitNone, []uint64{math.MaxUint32 - 1}},    // filled
		testAttr{999, typeS64, 200, []uint64{1, 2}},                                           // unknown attribute
	))
	require.NoError(t, err)
	assert.False(t, gm.PPTResidency.Valid)
	assert.False(t, gm.SocketThermalResidency.Valid)
	assert.Nil(t, gm.XGMIReadKB)
	assert.False(t, gm.HBMThermalResidency.Valid)
	assert.Equal(t, counters(1, 2, 3, 4, 5, 6, 7, 8), gm.XGMIWriteKB)
	assert.Equal(t, Counter{Value: math.MaxUint64}, gm.AccumulationCounter)
	assert.Equal(t, Counter{Value: math.MaxUint32}, gm.VRThermalResidency)
	assert.Equal(t, filled(math.MaxUint32-1), gm.ProchotResidency)
	assert.ElementsMatch(t, []string{"ppt_residency_acc", "socket_thm_residency_acc", "xgmi_read_data_acc", "hbm_thm_residency_acc"}, gm.InvalidFields)
}

// Linux 7.3 reports the residency and activity accumulators as U64, and the
// activity accumulators without a unit, under the same version; kernels before
// 7.0 report the accumulation counter as U32.
func TestParseGPUMetricsOtherKernelEncodings(t *testing.T) {
	gm, err := ParseGPUMetrics(buildGPUMetrics(1, 9,
		testAttr{attrAccumulationCounter, typeU32, unitNone, []uint64{7}},
		testAttr{attrPPTResidencyAcc, typeU64, unitNone, []uint64{1 << 40}},
		testAttr{attrHBMThmResidencyAcc, typeU64, unitNone, []uint64{math.MaxUint64}},
		testAttr{attrGFXActivityAcc, typeU64, unitNone, []uint64{1 << 41}},
		testAttr{attrMemActivityAcc, typeU64, unitNone, []uint64{3}},
	))
	require.NoError(t, err)
	assert.Equal(t, filled(7), gm.AccumulationCounter)
	assert.Equal(t, filled(1<<40), gm.PPTResidency)
	assert.Equal(t, Counter{Value: math.MaxUint64}, gm.HBMThermalResidency)
	assert.Equal(t, filled(1<<41), gm.GFXActivityAcc)
	assert.Equal(t, filled(3), gm.MemActivityAcc)
	assert.Empty(t, gm.InvalidFields)

	gm, err = ParseGPUMetrics(buildGPUMetrics(1, 9,
		testAttr{attrMemActivityAcc, typeU32, unitDataKB, []uint64{3}}, // not a known encoding
		testAttr{attrGFXActivityAcc, typeU64, unitPercent, make([]uint64, 4)},
	))
	require.NoError(t, err)
	assert.False(t, gm.MemActivityAcc.Valid)
	assert.False(t, gm.GFXActivityAcc.Valid)
	assert.ElementsMatch(t, []string{"mem_activity_acc", "gfx_activity_acc"}, gm.InvalidFields)
}

func TestReadGPUMetricsBounds(t *testing.T) {
	fs := NewFakeSysfs(t)
	attrs := MI300XAttributes("")
	attrs["gpu_metrics"] = string(make([]byte, maxGPUMetricsSize+1))
	fs.AddCard("card0", fs.AddPCIDevice("0000:c1:00.0", "amdgpu", attrs))
	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	m, err := devices[0].ReadMetrics()
	require.NoError(t, err)
	assert.ErrorIs(t, m.GPUMetricsErr, ErrGPUMetricsMalformed)
	assert.Nil(t, m.GPUMetrics)
}

func TestGPUMetricsCalibrate(t *testing.T) {
	gm := &GPUMetrics{
		Version:             GPUMetricsVersion{1, 9},
		TemperatureHotspotC: filled(60),
		SocketPowerW:        filled(700),
		PCIeLinkWidth:       filled(16),
		MemoryClockMHz:      filled(1300),
	}
	matching := Metrics{
		JunctionTemperatureC: valid(65),
		PowerMilliwatts:      valid(600000),
		PCIeLinkWidth:        valid(16),
		MemoryClockMHz:       valid(1300),
	}
	checked, err := gm.Calibrate(&matching)
	require.NoError(t, err)
	assert.Equal(t, 4, checked)

	checked, err = gm.Calibrate(&Metrics{PCIeLinkWidth: valid(16)})
	require.NoError(t, err, "comparisons without a sysfs value are skipped")
	assert.Equal(t, 1, checked)
	checked, err = gm.Calibrate(&Metrics{})
	require.ErrorIs(t, err, ErrGPUMetricsUncalibrated, "no comparison made")
	assert.Zero(t, checked)

	// The memory clock may move by one DPM level between the two reads.
	for name, tc := range map[string]struct {
		adjacent []float64
		ok       bool
	}{
		"adjacent level":     {adjacent: []float64{900, 1500}, ok: true},
		"non-adjacent level": {adjacent: []float64{900}},
		"single level":       {},
	} {
		t.Run(name, func(t *testing.T) {
			m := Metrics{MemoryClockMHz: valid(1100), MemoryClockAdjacentMHz: tc.adjacent}
			checked, err := (&GPUMetrics{MemoryClockMHz: filled(1500)}).Calibrate(&m)
			assert.Equal(t, 1, checked)
			assert.Equal(t, tc.ok, err == nil, err)
		})
	}

	for name, m := range map[string]Metrics{
		"power":       {PowerMilliwatts: valid(60000)},
		"temperature": {JunctionTemperatureC: valid(71)},
		"link width":  {PCIeLinkWidth: valid(8)},
		"clock":       {MemoryClockMHz: valid(900)},
	} {
		t.Run(name, func(t *testing.T) {
			checked, err := gm.Calibrate(&m)
			assert.Error(t, err)
			assert.Equal(t, 1, checked)
		})
	}

	// A table field the firmware did not fill is not compared.
	checked, err = (&GPUMetrics{SocketPowerW: Counter{Value: math.MaxUint16}, PCIeLinkWidth: filled(16)}).Calibrate(&matching)
	require.NoError(t, err)
	assert.Equal(t, 1, checked)
}

func TestXGMIHiveSize(t *testing.T) {
	fs := NewFakeSysfs(t)
	alone := fs.AddPCIDevice("0000:83:00.0", "amdgpu", MI300XAttributes("1111"))
	require.NoError(t, os.MkdirAll(filepath.Join(alone, "xgmi_hive_info"), 0o755))
	require.NoError(t, os.Symlink(alone, filepath.Join(alone, "xgmi_hive_info", "node1")))
	require.NoError(t, os.WriteFile(filepath.Join(alone, "xgmi_hive_info", "xgmi_hive_id"), []byte("1\n"), 0o644))
	none := fs.AddPCIDevice("0000:84:00.0", "amdgpu", MI300XAttributes("2222"))
	fs.AddCard("card0", alone)
	fs.AddCard("card1", none)

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	require.Len(t, devices, 2)
	assert.Equal(t, 1, devices[0].XGMIHiveSize())
	assert.Equal(t, 0, devices[1].XGMIHiveSize())
	m, err := devices[0].ReadMetrics()
	require.NoError(t, err)
	assert.Equal(t, 1, m.XGMIHiveSize)
}

// FuzzParseGPUMetrics checks that no table makes the decoder panic, and that
// a table it rejects yields no value.
func FuzzParseGPUMetrics(f *testing.F) {
	f.Add(readGPUMetricsFixture(f, "mi300x_vf"))
	f.Add(readGPUMetricsFixture(f, "mi350x_vf"))
	f.Add(buildGPUMetrics(1, 9, pptAttr(1)))
	f.Add(buildGPUMetrics(1, 8, pptAttr(1)))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		gm, err := ParseGPUMetrics(data)
		if err != nil {
			requireNoGPUMetricsValues(t, gm)
			return
		}
		require.NotNil(t, gm)
		assert.True(t, gm.Version.dynamic())
		assert.Equal(t, int(binary.LittleEndian.Uint16(data)), len(data))
	})
}
