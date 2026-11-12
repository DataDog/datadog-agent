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

// gfxBusyOffset returns GFX_ACTIVITY_ACC minus the mean of GFX_BUSY_ACC,
// modulo the width of the former, and clears the fields used to compute it.
func gfxBusyOffset(t *testing.T, gm *GPUMetrics) int64 {
	t.Helper()
	require.True(t, gm.gfxBusyMean.Valid)
	mask := gm.gfxActivityMask
	diff := (gm.GFXActivityAcc.Value - gm.gfxBusyMean.Value) & mask
	gm.gfxBusyMean, gm.gfxActivityMask = Counter{}, 0
	if diff > mask/2 {
		return -int64((mask - diff + 1) & mask)
	}
	return int64(diff)
}

// The MI300X VF table (amdgpu-dkms 6.19.14) has 50 attributes; IDs 48, 50 and
// 51 are unfilled.
func TestParseGPUMetricsMI300XVF(t *testing.T) {
	gm, err := ParseGPUMetrics(readGPUMetricsFixture(t, "mi300x_vf"))
	require.NoError(t, err)
	assert.Equal(t, int64(1), gfxBusyOffset(t, gm), "U32: wrapped, 1 off the mean of GFX_BUSY_ACC mod 2^32")
	assert.Equal(t, &GPUMetrics{
		Version:                GPUMetricsVersion{Format: 1, Content: 9},
		SystemClockNs:          filled(72203597569180),
		AccumulationCounter:    filled(2583688127),
		ProchotResidency:       filled(0),
		PPTResidency:           filled(350407088),
		SocketThermalResidency: filled(12105768),
		PCIeReplayCount:        filled(0),
		GFXActivityAcc:         filled(1336391183),
		MemActivityAcc:         filled(2862819467),
		XGMIReadKB:             counters(0, 1, 2, 1, 1, 1, 1, 1),
		XGMIWriteKB:            counters(0, 1, 1, 1, 1, 1, 1, 1),
		XGMILinkSpeedGTs:       filled(32),
		XGMILinkWidth:          filled(16),
	}, gm)
}

// The MI350X VF table (kernel 7.0) has 47 attributes and no NUM_PARTITION.
func TestParseGPUMetricsMI350XVF(t *testing.T) {
	gm, err := ParseGPUMetrics(readGPUMetricsFixture(t, "mi350x_vf"))
	require.NoError(t, err)
	assert.Equal(t, int64(-131), gfxBusyOffset(t, gm), "131 off the mean of GFX_BUSY_ACC")
	assert.Equal(t, &GPUMetrics{
		Version:                GPUMetricsVersion{Format: 1, Content: 9},
		SystemClockNs:          filled(266856137538),
		AccumulationCounter:    filled(36392871),
		ProchotResidency:       filled(0),
		PPTResidency:           filled(23598934),
		SocketThermalResidency: filled(0),
		PCIeReplayCount:        filled(0),
		GFXActivityAcc:         filled(2889439947),
		MemActivityAcc:         filled(715102902),
		XGMIReadKB:             counters(0, 53615, 17933, 71768, 0, 0, 35882, 53828),
		XGMIWriteKB:            counters(0, 17859, 17936, 17943, 17934, 17943, 17939, 17944),
		XGMILinkSpeedGTs:       filled(38),
		XGMILinkWidth:          filled(16),
	}, gm)
}

// SMU 15.0.8 reports XGMI as scalars and CURRENT_UCLK for 2 AIDs: those fields
// are skipped and the others decoded.
func TestParseGPUMetricsSMU1508(t *testing.T) {
	gm, err := ParseGPUMetrics(readGPUMetricsFixture(t, "smu15_0_8_synthetic"))
	require.NoError(t, err)
	assert.False(t, gm.gfxBusyMean.Valid, "no XCD filled")
	gm.gfxActivityMask = 0
	assert.Equal(t, &GPUMetrics{
		Version:                GPUMetricsVersion{Format: 1, Content: 9},
		SystemClockNs:          filled(1e12),
		AccumulationCounter:    filled(1e6),
		ProchotResidency:       filled(0),
		PPTResidency:           filled(5e5),
		SocketThermalResidency: filled(1000),
		PCIeReplayCount:        filled(3),
		GFXActivityAcc:         filled(1e8),
		MemActivityAcc:         filled(1e7),
		XGMILinkSpeedGTs:       Counter{Value: math.MaxUint16},
		XGMILinkWidth:          Counter{Value: math.MaxUint16},
		Skipped: []SkippedField{
			{Name: "xgmi_read_data_acc", Encoding: "U64/DATA_1/x1"},
			{Name: "xgmi_write_data_acc", Encoding: "U64/DATA_1/x1"},
			{Name: "current_uclk", Encoding: "U16/CLOCK_1/x2"},
		},
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

func TestReadRealGPUMetrics(t *testing.T) {
	for name, host := range map[string]func(*testing.T) *FakeSysfs{"mi300x_vf": mi300xVFHost, "mi350x_vf": mi350xVFHost} {
		t.Run(name, func(t *testing.T) {
			devices, err := Discover(host(t).Root)
			require.NoError(t, err)
			require.Len(t, devices, 1)
			m, err := devices[0].ReadMetrics()
			require.NoError(t, err)
			require.NoError(t, m.GPUMetricsErr)
			require.NotNil(t, m.GPUMetrics)
			assert.Empty(t, m.GPUMetrics.Skipped)
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
		testAttr{attrXGMIWriteDataAcc, typeU64, unitDataKB, []uint64{1, 2, 3, 4, 5, 6, 7, 8}}, // valid
		testAttr{attrAccumulationCounter, typeU64, unitNone, []uint64{math.MaxUint64}},        // unfilled
		testAttr{attrSocketThmResidencyAcc, typeU32, unitNone, []uint64{math.MaxUint32}},      // unfilled
		testAttr{attrProchotResidencyAcc, typeU32, unitNone, []uint64{math.MaxUint32 - 1}},    // filled
		testAttr{13, typeU16, unitNone, []uint64{1}},                                          // VR_THM_RESIDENCY_ACC: not used
		testAttr{999, typeS64, 200, []uint64{1, 2}},                                           // unknown attribute
	))
	require.NoError(t, err)
	assert.Equal(t, counters(1, 2, 3, 4, 5, 6, 7, 8), gm.XGMIWriteKB)
	assert.Equal(t, Counter{Value: math.MaxUint64}, gm.AccumulationCounter)
	assert.Equal(t, Counter{Value: math.MaxUint32}, gm.SocketThermalResidency)
	assert.Equal(t, filled(math.MaxUint32-1), gm.ProchotResidency)
	assert.Empty(t, gm.Skipped)

	// A used field with an unexpected type or unit, an anchor with an unknown
	// instance count, or a duplicated ID rejects the whole table.
	for name, tc := range map[string]struct {
		bad     []testAttr
		message string
	}{
		"wrong type":               {[]testAttr{{attrPPTResidencyAcc, typeU16, unitNone, []uint64{42}}}, "ppt_residency_acc U16/NONE/x1"},
		"wrong unit":               {[]testAttr{{attrSocketThmResidencyAcc, typeU32, unitPowerW, []uint64{42}}}, "socket_thm_residency_acc U32/POWER_1/x1"},
		"anchor instance count":    {[]testAttr{{attrAccumulationCounter, typeU64, unitNone, []uint64{1, 2}}}, "accumulation_counter U64/NONE/x2"},
		"memory clock of 4 levels": {[]testAttr{{attrCurrentUCLK, typeU16, unitClockMHz, make([]uint64, 4)}}, "current_uclk U16/CLOCK_1/x4"},
		"unknown unit":             {[]testAttr{{attrMemActivityAcc, typeU32, 99, []uint64{1}}}, "mem_activity_acc U32/unit99/x1"},
		"duplicated": {[]testAttr{{attrPPTResidencyAcc, typeU32, unitNone, []uint64{1}}, {attrPPTResidencyAcc, typeU32, unitNone, []uint64{2}}},
			"ppt_residency_acc duplicated"},
	} {
		t.Run(name, func(t *testing.T) {
			attrs := append([]testAttr{{attrXGMIWriteDataAcc, typeU64, unitDataKB, make([]uint64, 8)}}, tc.bad...)
			gm, err := ParseGPUMetrics(buildGPUMetrics(1, 9, attrs...))
			require.ErrorIs(t, err, ErrGPUMetricsUnexpectedEncoding)
			assert.ErrorContains(t, err, tc.message)
			requireNoGPUMetricsValues(t, gm)
		})
	}

	// Another instance count of an expected type and unit only skips the field.
	gm, err = ParseGPUMetrics(buildGPUMetrics(1, 9,
		pptAttr(42),
		testAttr{attrXGMIReadDataAcc, typeU64, unitDataKB, make([]uint64, 16)},
		testAttr{attrCurrentUCLK, typeU16, unitClockMHz, []uint64{1300, 1300}},
		testAttr{attrGFXBusyAcc, typeU64, unitNone, make([]uint64, 4)},
	))
	require.NoError(t, err)
	assert.Equal(t, filled(42), gm.PPTResidency)
	assert.Nil(t, gm.XGMIReadKB)
	assert.Equal(t, []SkippedField{
		{Name: "xgmi_read_data_acc", Encoding: "U64/DATA_1/x16"},
		{Name: "current_uclk", Encoding: "U16/CLOCK_1/x2"},
		{Name: "gfx_busy_acc", Encoding: "U64/NONE/x4"},
	}, gm.Skipped)
}

// No signature may hold an unset encoding, which would accept U8 fields.
func TestGPUMetricsSignaturesComplete(t *testing.T) {
	for id, sig := range gpuMetricsSignatures {
		for k, e := range sig.encodings {
			assert.NotEqual(t, gpuMetricsEncoding{}, e, "%s (ID %d), kernel %d", sig.name, id, k)
		}
		assert.Positive(t, sig.instances, sig.name)
	}
	assert.Panics(t, func() { byKernel("short", u32None, u64None) })
}

// GFX_ACTIVITY_ACC increases as the mean of the per-XCD GFX_BUSY_ACC does,
// modulo its width, whatever the offset between their totals.
func TestGFXActivityMatchesBusy(t *testing.T) {
	busy := func(values ...uint64) testAttr { return testAttr{attrGFXBusyAcc, typeU64, unitPercent, values} }
	activity := func(v uint64) testAttr { return testAttr{attrGFXActivityAcc, typeU32, unitPercent, []uint64{v}} }
	parse := func(attrs ...testAttr) *GPUMetrics {
		gm, err := ParseGPUMetrics(buildGPUMetrics(1, 9, attrs...))
		require.NoError(t, err)
		require.True(t, gm.GFXActivityAcc.Valid)
		return gm
	}
	unfilled := uint64(math.MaxUint64)
	xcds := func(mean uint64) testAttr {
		return busy(mean-1200, mean-400, mean+400, mean+1200, unfilled, unfilled, unfilled, unfilled)
	}
	const periods = 15000 // 15 s at 1 ms per period
	tolerance := gfxBusyTolerance(periods)
	require.Equal(t, uint64(1900), tolerance)
	const mean, offset, increase = uint64(40e9), uint64(1e6), uint64(356250)
	previous := parse(activity((mean+offset)%u32), xcds(mean))
	for name, tc := range map[string]struct {
		current *GPUMetrics
		valid   bool
	}{
		"same increase, offset totals": {parse(activity((mean+offset+increase)%u32), xcds(mean+increase)), true},
		"within the tolerance":         {parse(activity((mean+offset+increase+tolerance)%u32), xcds(mean+increase)), true},
		"beyond the tolerance":         {parse(activity((mean+offset+increase+tolerance+1)%u32), xcds(mean+increase)), false},
		"another quantity":             {parse(activity((mean+offset+2*increase)%u32), xcds(mean+increase)), false},
		"across the wrap":              {parse(activity((mean+offset+u32)%u32), xcds(mean+u32)), true},
		"no XCD filled":                {parse(activity(7), busy(unfilled, unfilled, unfilled, unfilled, unfilled, unfilled, unfilled, unfilled)), true},
		"no GFX_BUSY_ACC":              {parse(activity(7)), true},
	} {
		assert.Equal(t, tc.valid, GFXActivityMatchesBusy(previous, tc.current, periods), name)
	}

	// On Linux 7.3 GFX_ACTIVITY_ACC is U64: no modulo 2^32.
	activity64 := func(v uint64) testAttr { return testAttr{attrGFXActivityAcc, typeU64, unitNone, []uint64{v}} }
	busy64 := func(mean uint64) testAttr {
		return testAttr{attrGFXBusyAcc, typeU64, unitNone, []uint64{mean, mean, unfilled, unfilled, unfilled, unfilled, unfilled, unfilled}}
	}
	previous = parse(activity64(mean+offset), busy64(mean))
	assert.True(t, GFXActivityMatchesBusy(previous, parse(activity64(mean+offset+increase), busy64(mean+increase)), periods))
	assert.False(t, GFXActivityMatchesBusy(previous, parse(activity64(mean+offset+increase-u32), busy64(mean+increase)), periods))
}

const u32 = uint64(math.MaxUint32) + 1

// Each supported kernel encodes the accumulators differently under the same
// version; only these exact encodings are accepted.
func TestParseGPUMetricsKernelEncodings(t *testing.T) {
	for name, attrs := range map[string][]testAttr{
		"amdgpu-dkms 6.19": {
			{attrAccumulationCounter, typeU32, unitNone, []uint64{7}},
			{attrPPTResidencyAcc, typeU32, unitNone, []uint64{1}},
			{attrGFXActivityAcc, typeU32, unitPercent, []uint64{2}},
			{attrMemActivityAcc, typeU32, unitPercent, []uint64{3}},
		},
		"Linux 7.0": {
			{attrAccumulationCounter, typeU64, unitNone, []uint64{7}},
			{attrPPTResidencyAcc, typeU32, unitNone, []uint64{1}},
			{attrGFXActivityAcc, typeU32, unitPercent, []uint64{2}},
			{attrMemActivityAcc, typeU32, unitPercent, []uint64{3}},
		},
		"Linux 7.3": {
			{attrAccumulationCounter, typeU64, unitNone, []uint64{7}},
			{attrPPTResidencyAcc, typeU64, unitNone, []uint64{1}},
			{attrGFXActivityAcc, typeU64, unitNone, []uint64{2}},
			{attrMemActivityAcc, typeU64, unitNone, []uint64{3}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			gm, err := ParseGPUMetrics(buildGPUMetrics(1, 9, attrs...))
			require.NoError(t, err)
			assert.Equal(t, filled(7), gm.AccumulationCounter)
			assert.Equal(t, filled(1), gm.PPTResidency)
			assert.Equal(t, filled(2), gm.GFXActivityAcc)
			assert.Equal(t, filled(3), gm.MemActivityAcc)
		})
	}

	for name, attr := range map[string]testAttr{
		"PCIE_BANDWIDTH_ACC up to 7.2": {attrGFXActivityAcc, typeU64, unitPercent, []uint64{2}},
		"U32 without a unit":           {attrMemActivityAcc, typeU32, unitNone, []uint64{3}},
		"data unit":                    {attrMemActivityAcc, typeU32, unitDataKB, []uint64{3}},
		"U64 residency in percent":     {attrPPTResidencyAcc, typeU64, unitPercent, []uint64{1}},
		"U16 accumulation counter":     {attrAccumulationCounter, typeU16, unitNone, []uint64{7}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseGPUMetrics(buildGPUMetrics(1, 9, attr))
			require.ErrorIs(t, err, ErrGPUMetricsUnexpectedEncoding)
		})
	}
}

// shiftGPUMetricsIDs rewrites a table as if an attribute ID were inserted at
// k: every ID from k on is incremented.
func shiftGPUMetricsIDs(data []byte, k gpuMetricsAttrID) []byte {
	out := clone(data)
	offset := gpuMetricsHeaderSize
	for range binary.LittleEndian.Uint32(out[4:8]) {
		encoding := binary.LittleEndian.Uint64(out[offset:])
		if id := gpuMetricsAttrID(encoding >> 10 & 0x3ff); id >= k {
			encoding = encoding&^(0x3ff<<10) | uint64(id+1)<<10
			binary.LittleEndian.PutUint64(out[offset:], encoding)
		}
		offset += gpuMetricsEncodingSize + int(encoding&0x3ff)*gpuMetricsType(encoding>>20&0xf).size()
	}
	return out
}

// A kernel inserting an attribute ID shifts the fields after it onto the IDs
// of their neighbours: every such table of the real captures and of the SMU
// 15.0.8 layout that moves a field onto a used ID is rejected.
func TestParseGPUMetricsShiftedIDs(t *testing.T) {
	for _, fixture := range []string{"mi300x_vf", "mi350x_vf", "smu15_0_8_synthetic"} {
		data := readGPUMetricsFixture(t, fixture)
		_, err := ParseGPUMetrics(data)
		require.NoError(t, err, fixture)
		rejected, accepted := 0, 0
		for k := range attrGFXBusyAcc + 12 {
			shifted := shiftGPUMetricsIDs(data, k)
			attrs, err := decodeGPUMetricsAttrs(shifted, len(shifted))
			require.NoError(t, err)
			// A used ID after k now holds the attribute that preceded it.
			wrongSource := false
			for id := range gpuMetricsSignatures {
				_, present := attrs[id]
				wrongSource = wrongSource || (id > k && present)
			}
			gm, err := ParseGPUMetrics(shifted)
			if wrongSource {
				rejected++
				assert.ErrorIs(t, err, ErrGPUMetricsUnexpectedEncoding, "%s: insertion at %d", fixture, k)
				requireNoGPUMetricsValues(t, gm)
			} else {
				accepted++
				assert.NoError(t, err, "%s: insertion at %d only removes fields", fixture, k)
			}
		}
		assert.Equal(t, int(attrGFXBusyAcc), rejected, "%s: insertions at 0 to 42", fixture)
		assert.Equal(t, 12, accepted, fixture)
	}

	data := readGPUMetricsFixture(t, "mi300x_vf")
	// At 24, PCIE_L0_TO_RECOV_COUNT_ACC (U64, no unit) would be read as the
	// replay counter, and only the encodings of other fields reveal the shift.
	_, err := ParseGPUMetrics(shiftGPUMetricsIDs(data, 24))
	require.ErrorIs(t, err, ErrGPUMetricsUnexpectedEncoding)
	// From 29 to 36, CURRENT_UCLK receives another clock of the same type and
	// unit: its anchor instance count rejects the table, where XGMI_WRITE_DATA_ACC
	// would otherwise hold the read counters at 29.
	for k := gpuMetricsAttrID(29); k <= 36; k++ {
		_, err := ParseGPUMetrics(shiftGPUMetricsIDs(data, k))
		assert.ErrorContains(t, err, "current_uclk U16/CLOCK_1/x", "insertion at %d", k)
	}
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
