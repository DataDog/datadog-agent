// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package amd

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The gpu_metrics sysfs attribute holds a binary table the amdgpu driver fills
// from the SMU firmware. Format 1.9 and later (Instinct MI300 and MI350 with a
// recent amdgpu) is self-describing: a header, an attribute count, and for
// each attribute a 64-bit encoding followed by its packed little-endian
// values. The encoding and the attribute IDs come from
// drivers/gpu/drm/amd/include/kgd_pp_interface.h; the decoding follows ROCm
// SMI (rocm_smi_dyn_gpu_metrics.cc).
//
// kgd_pp_interface.h is:
//
//	Copyright 2017 Advanced Micro Devices, Inc.
//
//	Permission is hereby granted, free of charge, to any person obtaining a
//	copy of this software and associated documentation files (the "Software"),
//	to deal in the Software without restriction, including without limitation
//	the rights to use, copy, modify, merge, publish, distribute, sublicense,
//	and/or sell copies of the Software, and to permit persons to whom the
//	Software is furnished to do so, subject to the following conditions:
//
//	The above copyright notice and this permission notice shall be included in
//	all copies or substantial portions of the Software.
//
//	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
//	IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
//	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.  IN NO EVENT SHALL
//	THE COPYRIGHT HOLDER(S) OR AUTHOR(S) BE LIABLE FOR ANY CLAIM, DAMAGES OR
//	OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
//	ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
//	OTHER DEALINGS IN THE SOFTWARE.

var (
	// ErrGPUMetricsUnsupported reports a gpu_metrics table whose version is
	// not decoded: the fixed layouts before 1.9 and the APU formats (2.x, 3.x).
	ErrGPUMetricsUnsupported = errors.New("unsupported gpu_metrics version")
	// ErrGPUMetricsMalformed reports a gpu_metrics table whose structure is
	// inconsistent. No value of such a table is used.
	ErrGPUMetricsMalformed = errors.New("malformed gpu_metrics table")
	// ErrGPUMetricsUncalibrated reports a gpu_metrics table with no field that
	// could be compared with sysfs.
	ErrGPUMetricsUncalibrated = errors.New("no gpu_metrics field could be compared with sysfs")
)

const (
	// gpuMetricsHeaderSize is the size of the header (structure_size,
	// format_revision, content_revision) and of attr_count.
	gpuMetricsHeaderSize = 8
	// gpuMetricsEncodingSize is the size of an attribute encoding.
	gpuMetricsEncodingSize = 8
	// maxGPUMetricsSize bounds a table: structure_size is 16-bit, and amdgpu
	// returns at most one page.
	maxGPUMetricsSize = math.MaxUint16
)

// gpuMetricsAttrID is an attribute ID of the dynamic gpu_metrics format, from
// enum amdgpu_metrics_attr_id. IDs 0 to 47 have kept their values in every
// released kernel, but the enum is not formally append-only (TEMPERATURE_MID
// was inserted after TEMPERATURE_HBM in v7.1), so every field is also checked
// against its expected type, unit and instance count, and the table is
// calibrated against independent sysfs attributes before it is trusted.
type gpuMetricsAttrID uint16

const (
	attrTemperatureHotspot    gpuMetricsAttrID = 0
	attrCurrSocketPower       gpuMetricsAttrID = 3
	attrSystemClockCounter    gpuMetricsAttrID = 8
	attrAccumulationCounter   gpuMetricsAttrID = 9
	attrProchotResidencyAcc   gpuMetricsAttrID = 10
	attrPPTResidencyAcc       gpuMetricsAttrID = 11
	attrSocketThmResidencyAcc gpuMetricsAttrID = 12
	attrVRThmResidencyAcc     gpuMetricsAttrID = 13
	attrHBMThmResidencyAcc    gpuMetricsAttrID = 14
	attrPCIeLinkWidth         gpuMetricsAttrID = 16
	attrGFXActivityAcc        gpuMetricsAttrID = 20
	attrMemActivityAcc        gpuMetricsAttrID = 21
	attrPCIeReplayCountAcc    gpuMetricsAttrID = 25
	attrXGMIReadDataAcc       gpuMetricsAttrID = 29
	attrXGMIWriteDataAcc      gpuMetricsAttrID = 30
	attrCurrentUCLK           gpuMetricsAttrID = 37
)

// gpuMetricsType is enum amdgpu_metrics_attr_type.
type gpuMetricsType uint8

const (
	typeU8 gpuMetricsType = iota
	typeS8
	typeU16
	typeS16
	typeU32
	typeS32
	typeU64
	typeS64
	typeCount
)

// size returns the size in bytes of one value of the type.
func (t gpuMetricsType) size() int {
	return 1 << (t / 2)
}

// gpuMetricsUnit is enum amdgpu_metrics_attr_unit.
type gpuMetricsUnit uint8

const (
	unitNone     gpuMetricsUnit = 0
	unitClockMHz gpuMetricsUnit = 1
	unitTempC    gpuMetricsUnit = 2
	unitPowerW   gpuMetricsUnit = 3
	unitTimeNs   gpuMetricsUnit = 4
	unitDataKB   gpuMetricsUnit = 9
	unitPercent  gpuMetricsUnit = 10
)

// numXGMILinks is the number of XGMI link instances amdgpu reports
// (NUM_XGMI_LINKS).
const numXGMILinks = 8

// gpuMetricsSignature is the expected encoding of a field: typ and unit are
// those of Linux 7.0 and amdgpu-dkms 6.19, and altType and altUnit those
// another kernel uses (typ and unit again when there are none).
type gpuMetricsSignature struct {
	name      string
	instances int
	typ       gpuMetricsType
	altType   gpuMetricsType
	unit      gpuMetricsUnit
	altUnit   gpuMetricsUnit
}

// accepts reports whether an attribute encoding matches the signature.
func (s gpuMetricsSignature) accepts(typ gpuMetricsType, unit gpuMetricsUnit) bool {
	return (typ == s.typ || typ == s.altType) && (unit == s.unit || unit == s.altUnit)
}

// gpuMetricsSignatures are the fields the GPU check uses, with the encodings
// amdgpu gives them (smu_v13_0_6_ppt.h, SMU_13_0_6_METRICS_FIELDS). Kernels
// before 7.0 report ACCUMULATION_COUNTER as U32; Linux 7.3 widened the
// residency and activity accumulators to U64 and changed the unit of the
// activity accumulators from percent to none, without changing the version.
var gpuMetricsSignatures = map[gpuMetricsAttrID]gpuMetricsSignature{
	attrTemperatureHotspot:    {"temperature_hotspot", 1, typeU16, typeU16, unitTempC, unitTempC},
	attrCurrSocketPower:       {"curr_socket_power", 1, typeU16, typeU16, unitPowerW, unitPowerW},
	attrSystemClockCounter:    {"system_clock_counter", 1, typeU64, typeU64, unitTimeNs, unitTimeNs},
	attrAccumulationCounter:   {"accumulation_counter", 1, typeU64, typeU32, unitNone, unitNone},
	attrProchotResidencyAcc:   {"prochot_residency_acc", 1, typeU32, typeU64, unitNone, unitNone},
	attrPPTResidencyAcc:       {"ppt_residency_acc", 1, typeU32, typeU64, unitNone, unitNone},
	attrSocketThmResidencyAcc: {"socket_thm_residency_acc", 1, typeU32, typeU64, unitNone, unitNone},
	attrVRThmResidencyAcc:     {"vr_thm_residency_acc", 1, typeU32, typeU64, unitNone, unitNone},
	attrHBMThmResidencyAcc:    {"hbm_thm_residency_acc", 1, typeU32, typeU64, unitNone, unitNone},
	attrPCIeLinkWidth:         {"pcie_link_width", 1, typeU16, typeU16, unitNone, unitNone},
	attrGFXActivityAcc:        {"gfx_activity_acc", 1, typeU32, typeU64, unitPercent, unitNone},
	attrMemActivityAcc:        {"mem_activity_acc", 1, typeU32, typeU64, unitPercent, unitNone},
	attrPCIeReplayCountAcc:    {"pcie_replay_count_acc", 1, typeU64, typeU64, unitNone, unitNone},
	attrXGMIReadDataAcc:       {"xgmi_read_data_acc", numXGMILinks, typeU64, typeU64, unitDataKB, unitDataKB},
	attrXGMIWriteDataAcc:      {"xgmi_write_data_acc", numXGMILinks, typeU64, typeU64, unitDataKB, unitDataKB},
	attrCurrentUCLK:           {"current_uclk", 1, typeU16, typeU16, unitClockMHz, unitClockMHz},
}

// GPUMetricsVersion is the format and content revision of a gpu_metrics table.
type GPUMetricsVersion struct {
	Format  uint8
	Content uint8
}

func (v GPUMetricsVersion) String() string {
	return strconv.Itoa(int(v.Format)) + "." + strconv.Itoa(int(v.Content))
}

// dynamic reports whether the version uses the self-describing format. Later
// 1.x revisions are decoded too: the layout describes itself, and each field
// is checked against its signature and calibrated before use.
func (v GPUMetricsVersion) dynamic() bool {
	return v.Format == 1 && v.Content >= 9
}

// Counter is a raw gpu_metrics value. Valid is false when the field is
// absent, has an unexpected encoding, or was not filled by the firmware.
type Counter struct {
	Value uint64
	Valid bool
}

// GPUMetrics holds the fields of a gpu_metrics table the GPU check uses.
type GPUMetrics struct {
	Version GPUMetricsVersion

	// Fields used to calibrate the table against other sysfs attributes.
	TemperatureHotspotC Counter // °C
	SocketPowerW        Counter // W
	PCIeLinkWidth       Counter // lanes
	MemoryClockMHz      Counter // current UCLK

	// SystemClockNs is the boot time, in ns, at which the driver filled the table.
	SystemClockNs Counter
	// AccumulationCounter counts the firmware accumulation periods: the
	// residency counters below increase by one for each period throttled.
	AccumulationCounter    Counter
	ProchotResidency       Counter
	PPTResidency           Counter
	SocketThermalResidency Counter
	VRThermalResidency     Counter
	HBMThermalResidency    Counter

	// GFXActivityAcc accumulates the graphics activity of the socket, the
	// mean over its XCDs, in percent per accumulation period. MemActivityAcc
	// accumulates the HBM bandwidth utilization, in percent of the peak per
	// period.
	GFXActivityAcc Counter
	MemActivityAcc Counter

	// PCIeReplayCount is the number of PCIe replays since the driver loaded.
	PCIeReplayCount Counter

	// XGMIReadKB and XGMIWriteKB are the data, in KB, read and written over
	// each XGMI link since the driver loaded. They are nil when absent.
	XGMIReadKB  []Counter
	XGMIWriteKB []Counter

	// InvalidFields lists the used fields skipped because of an unexpected
	// encoding or a duplicate ID.
	InvalidFields []string
}

// gpuMetricsAttr is a decoded attribute: its encoding and the raw bits of
// each value, zero-extended.
type gpuMetricsAttr struct {
	typ       gpuMetricsType
	unit      gpuMetricsUnit
	values    []uint64
	duplicate bool
}

// ParseGPUMetrics decodes a gpu_metrics table read in one piece. Only the
// dynamic format (1.9 and later) is decoded: other versions return
// ErrGPUMetricsUnsupported with the version read from the header. A table with
// an inconsistent structure returns ErrGPUMetricsMalformed and no value.
func ParseGPUMetrics(data []byte) (*GPUMetrics, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("%w: %d bytes", ErrGPUMetricsMalformed, len(data))
	}
	size := int(binary.LittleEndian.Uint16(data[0:2]))
	version := GPUMetricsVersion{Format: data[2], Content: data[3]}
	if !version.dynamic() {
		return &GPUMetrics{Version: version}, fmt.Errorf("%w %s", ErrGPUMetricsUnsupported, version)
	}
	attrs, err := decodeGPUMetricsAttrs(data, size)
	if err != nil {
		return &GPUMetrics{Version: version}, fmt.Errorf("%w (version %s): %w", ErrGPUMetricsMalformed, version, err)
	}

	gm := &GPUMetrics{Version: version}
	scalar := func(id gpuMetricsAttrID) Counter {
		values := gm.field(attrs, id)
		if values == nil {
			return Counter{}
		}
		return values[0]
	}
	gm.TemperatureHotspotC = scalar(attrTemperatureHotspot)
	gm.SocketPowerW = scalar(attrCurrSocketPower)
	gm.PCIeLinkWidth = scalar(attrPCIeLinkWidth)
	gm.MemoryClockMHz = scalar(attrCurrentUCLK)
	gm.SystemClockNs = scalar(attrSystemClockCounter)
	gm.AccumulationCounter = scalar(attrAccumulationCounter)
	gm.ProchotResidency = scalar(attrProchotResidencyAcc)
	gm.PPTResidency = scalar(attrPPTResidencyAcc)
	gm.SocketThermalResidency = scalar(attrSocketThmResidencyAcc)
	gm.VRThermalResidency = scalar(attrVRThmResidencyAcc)
	gm.HBMThermalResidency = scalar(attrHBMThmResidencyAcc)
	gm.PCIeReplayCount = scalar(attrPCIeReplayCountAcc)
	gm.GFXActivityAcc = scalar(attrGFXActivityAcc)
	gm.MemActivityAcc = scalar(attrMemActivityAcc)
	gm.XGMIReadKB = gm.field(attrs, attrXGMIReadDataAcc)
	gm.XGMIWriteKB = gm.field(attrs, attrXGMIWriteDataAcc)
	return gm, nil
}

// decodeGPUMetricsAttrs decodes the attributes of a dynamic table. The table
// must be exactly structure_size bytes and be fully consumed by attr_count
// attributes of known types.
func decodeGPUMetricsAttrs(data []byte, size int) (map[gpuMetricsAttrID]*gpuMetricsAttr, error) {
	if size != len(data) {
		return nil, fmt.Errorf("structure_size %d, read %d bytes", size, len(data))
	}
	if size < gpuMetricsHeaderSize {
		return nil, fmt.Errorf("structure_size %d is smaller than the header", size)
	}
	count := int64(int32(binary.LittleEndian.Uint32(data[4:8])))
	// Each attribute takes its encoding and at least one byte.
	if count <= 0 || count > int64(size-gpuMetricsHeaderSize)/(gpuMetricsEncodingSize+1) {
		return nil, fmt.Errorf("invalid attr_count %d for %d bytes", count, size)
	}

	attrs := make(map[gpuMetricsAttrID]*gpuMetricsAttr, count)
	offset := gpuMetricsHeaderSize
	for i := range count {
		if len(data)-offset < gpuMetricsEncodingSize {
			return nil, fmt.Errorf("attribute %d: encoding overruns the table", i)
		}
		encoding := binary.LittleEndian.Uint64(data[offset:])
		offset += gpuMetricsEncodingSize
		if encoding>>32 != 0 {
			return nil, fmt.Errorf("attribute %d: unknown encoding %#x", i, encoding)
		}
		unit := gpuMetricsUnit(encoding >> 24)
		typ := gpuMetricsType(encoding >> 20 & 0xf)
		id := gpuMetricsAttrID(encoding >> 10 & 0x3ff)
		instances := int(encoding & 0x3ff)
		if typ >= typeCount {
			return nil, fmt.Errorf("attribute %d (ID %d): unknown type %d", i, id, typ)
		}
		if instances == 0 {
			return nil, fmt.Errorf("attribute %d (ID %d): no instance", i, id)
		}
		width := typ.size()
		if (len(data)-offset)/width < instances {
			return nil, fmt.Errorf("attribute %d (ID %d): %d values overrun the table", i, id, instances)
		}
		attr := &gpuMetricsAttr{typ: typ, unit: unit}
		if _, used := gpuMetricsSignatures[id]; used {
			attr.values = make([]uint64, instances)
			for j := range attr.values {
				attr.values[j] = readUnsigned(data[offset+j*width:], width)
			}
		}
		offset += instances * width
		if previous, seen := attrs[id]; seen {
			previous.duplicate = true
			continue
		}
		attrs[id] = attr
	}
	if offset != len(data) {
		return nil, fmt.Errorf("%d bytes left after %d attributes", len(data)-offset, count)
	}
	return attrs, nil
}

// readUnsigned reads a little-endian value of width bytes.
func readUnsigned(b []byte, width int) uint64 {
	switch width {
	case 1:
		return uint64(b[0])
	case 2:
		return uint64(binary.LittleEndian.Uint16(b))
	case 4:
		return uint64(binary.LittleEndian.Uint32(b))
	default:
		return binary.LittleEndian.Uint64(b)
	}
}

// field returns the values of a used field, or nil when it is absent. A field
// whose encoding differs from its signature, or whose ID is duplicated, is
// recorded in InvalidFields and returned as nil. Values the firmware did not
// fill (all bits set) are invalid.
func (gm *GPUMetrics) field(attrs map[gpuMetricsAttrID]*gpuMetricsAttr, id gpuMetricsAttrID) []Counter {
	attr, ok := attrs[id]
	if !ok {
		return nil
	}
	sig := gpuMetricsSignatures[id]
	if attr.duplicate || !sig.accepts(attr.typ, attr.unit) || len(attr.values) != sig.instances {
		gm.InvalidFields = append(gm.InvalidFields, sig.name)
		return nil
	}
	unfilled := uint64(math.MaxUint64) >> (64 - 8*attr.typ.size())
	values := make([]Counter, len(attr.values))
	for i, v := range attr.values {
		values[i] = Counter{Value: v, Valid: v != unfilled}
	}
	return values
}

// Calibration tolerances. They are wide enough for the values to be sampled
// at slightly different times, and narrow enough to reject a field holding
// another quantity.
const (
	calibrationPowerMinW    = 50
	calibrationPowerRatio   = 0.25
	calibrationTemperatureC = 10
	calibrationClockMinMHz  = 100
	calibrationClockRatio   = 0.1
	milliwattsPerWatt       = 1000
)

// Calibrate cross-checks the table against the sysfs attributes read in the
// same pass: socket power against hwmon power, hotspot temperature against the
// hwmon junction sensor, PCIe link width against current_link_width, and the
// memory clock against the active pp_dpm_mclk level or a level next to it, as
// the clock may change between the two reads. A comparison is skipped when
// either side is unavailable, and the table fails unless at least one
// comparison was made. It returns the number of comparisons made and an error
// describing every mismatch.
func (gm *GPUMetrics) Calibrate(m *Metrics) (int, error) {
	checked := 0
	var mismatches []string
	compare := func(name string, table Counter, sysfs Reading, unit string, tolerance func(a, b float64) float64, alternatives ...float64) {
		if !table.Valid || !sysfs.Valid {
			return
		}
		checked++
		a, b := float64(table.Value), sysfs.Value
		for _, candidate := range append([]float64{b}, alternatives...) {
			if math.Abs(a-candidate) <= tolerance(a, candidate) {
				return
			}
		}
		mismatches = append(mismatches, fmt.Sprintf("%s %v%s, sysfs %v%s", name, a, unit, b, unit))
	}
	relative := func(minimum, ratio float64) func(a, b float64) float64 {
		return func(a, b float64) float64 { return max(minimum, ratio*max(a, b)) }
	}
	absolute := func(tolerance float64) func(a, b float64) float64 {
		return func(float64, float64) float64 { return tolerance }
	}

	power := m.PowerMilliwatts
	power.Value /= milliwattsPerWatt
	compare("socket power", gm.SocketPowerW, power, " W", relative(calibrationPowerMinW, calibrationPowerRatio))
	compare("hotspot temperature", gm.TemperatureHotspotC, m.JunctionTemperatureC, " °C", absolute(calibrationTemperatureC))
	compare("PCIe link width", gm.PCIeLinkWidth, m.PCIeLinkWidth, "", absolute(0))
	compare("memory clock", gm.MemoryClockMHz, m.MemoryClockMHz, " MHz", relative(calibrationClockMinMHz, calibrationClockRatio), m.MemoryClockAdjacentMHz...)
	if len(mismatches) > 0 {
		return checked, fmt.Errorf("gpu_metrics %s does not match sysfs: %s", gm.Version, strings.Join(mismatches, "; "))
	}
	if checked == 0 {
		return 0, fmt.Errorf("%w (version %s)", ErrGPUMetricsUncalibrated, gm.Version)
	}
	return checked, nil
}

// gpuMetrics reads and decodes the gpu_metrics attribute of a device in a
// single read. It returns nil without an error when the device has none.
func (r *reader) gpuMetrics(path string) (*GPUMetrics, error) {
	f, err := os.Open(path)
	if err != nil {
		if !isUnsupported(err) {
			r.errs = append(r.errs, err)
		}
		return nil, nil
	}
	defer f.Close()
	// The attribute is generated in full by the first read: reading it
	// through one file descriptor returns one consistent table.
	data, err := io.ReadAll(io.LimitReader(f, maxGPUMetricsSize+1))
	if err != nil {
		if !isUnsupported(err) {
			r.errs = append(r.errs, fmt.Errorf("read %s: %w", path, err))
		}
		return nil, nil
	}
	if len(data) == 0 {
		return nil, nil
	}
	if len(data) > maxGPUMetricsSize {
		return nil, fmt.Errorf("%w: more than %d bytes", ErrGPUMetricsMalformed, maxGPUMetricsSize)
	}
	return ParseGPUMetrics(data)
}

// XGMIHiveSize returns the number of GPUs in the XGMI hive of the device, from
// the node links of its xgmi_hive_info directory, or 0 when the device is not
// in a hive. A GPU alone in its hive has no XGMI peer to exchange data with.
func (d *Device) XGMIHiveSize() int {
	entries, err := os.ReadDir(filepath.Join(d.devicePath, "xgmi_hive_info"))
	if err != nil {
		return 0
	}
	n := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "node") {
			n++
		}
	}
	return n
}
