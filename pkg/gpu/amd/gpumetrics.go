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
	"maps"
	"math"
	"math/bits"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
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
	// ErrGPUMetricsUnexpectedEncoding reports a gpu_metrics table in which a
	// used field has a type or unit no supported kernel gives it, an anchor
	// field has an unknown instance count, or an ID is duplicated. The
	// attribute IDs may have shifted, so no value of the table is used.
	ErrGPUMetricsUnexpectedEncoding = errors.New("unexpected gpu_metrics field encoding")
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
	// gfxBusySkewPeriods and gfxBusyDriftPeriods bound the difference between
	// the increases of GFX_ACTIVITY_ACC and of the mean of GFX_BUSY_ACC, in
	// periods at 100%; see gfxBusyTolerance.
	gfxBusySkewPeriods  = 4
	gfxBusyDriftPeriods = 1000
)

// gpuMetricsAttrID is an attribute ID of the dynamic gpu_metrics format, from
// enum amdgpu_metrics_attr_id. IDs 0 to 47 have kept their values in every
// released kernel, but the enum is not formally append-only (TEMPERATURE_MID
// was inserted after TEMPERATURE_HBM in v7.1), so the encoding of each used
// field is checked to detect shifted IDs.
type gpuMetricsAttrID uint16

const (
	attrSystemClockCounter    gpuMetricsAttrID = 8
	attrAccumulationCounter   gpuMetricsAttrID = 9
	attrProchotResidencyAcc   gpuMetricsAttrID = 10
	attrPPTResidencyAcc       gpuMetricsAttrID = 11
	attrSocketThmResidencyAcc gpuMetricsAttrID = 12
	attrXGMILinkWidth         gpuMetricsAttrID = 18
	attrXGMILinkSpeed         gpuMetricsAttrID = 19
	attrGFXActivityAcc        gpuMetricsAttrID = 20
	attrMemActivityAcc        gpuMetricsAttrID = 21
	attrPCIeReplayCountAcc    gpuMetricsAttrID = 25
	attrXGMIReadDataAcc       gpuMetricsAttrID = 29
	attrXGMIWriteDataAcc      gpuMetricsAttrID = 30
	attrCurrentUCLK           gpuMetricsAttrID = 37
	attrGFXBusyAcc            gpuMetricsAttrID = 43
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

var gpuMetricsTypeNames = [typeCount]string{"U8", "S8", "U16", "S16", "U32", "S32", "U64", "S64"}

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
	unitSpeedGTs gpuMetricsUnit = 6
	unitDataKB   gpuMetricsUnit = 9
	unitPercent  gpuMetricsUnit = 10
	unitCount    gpuMetricsUnit = 11
)

var gpuMetricsUnitNames = [unitCount]string{"NONE", "CLOCK_1", "TEMP_1", "POWER_1", "TIME_1", "TIME_2", "SPEED_1", "SPEED_2", "BW_1", "DATA_1", "PERCENT"}

// numXGMILinks is the number of XGMI link instances amdgpu reports
// (NUM_XGMI_LINKS).
const numXGMILinks = 8

// gpuMetricsEncoding is the type and unit of a field.
type gpuMetricsEncoding struct {
	typ  gpuMetricsType
	unit gpuMetricsUnit
}

// encodingName names the encoding of an attribute of a known type, e.g.
// "U64/DATA_1/x8".
func encodingName(typ gpuMetricsType, unit gpuMetricsUnit, instances int) string {
	u := "unit" + strconv.Itoa(int(unit))
	if unit < unitCount {
		u = gpuMetricsUnitNames[unit]
	}
	return gpuMetricsTypeNames[typ] + "/" + u + "/x" + strconv.Itoa(instances)
}

// gpuMetricsKernel indexes the supported kernel layouts of the used fields.
type gpuMetricsKernel int

const (
	kernel619 gpuMetricsKernel = iota // Linux 6.19 / ROCm 7.2: U32 accumulation counter
	kernel70                          // Linux 7.0 to 7.2, amdgpu-dkms 6.19.14: U64 accumulation counter
	kernel73                          // Linux 7.3 and SMU 15.0.8: U64 accumulators, activity without a unit
	kernelCount
)

// gpuMetricsSignature is the decoded instance count of a used field and its
// type and unit in each supported kernel. A field with a known type and unit
// and another instance count is skipped, except for an anchor: only the counts
// in skip are skipped, and any other rejects the table, as a shifted
// neighbour of the same type and unit would otherwise go unnoticed.
type gpuMetricsSignature struct {
	name      string
	instances int
	encodings [kernelCount]gpuMetricsEncoding
	anchor    bool
	skip      []int
}

// same returns the signature of a field encoded alike in every kernel.
func same(name string, instances int, typ gpuMetricsType, unit gpuMetricsUnit) gpuMetricsSignature {
	s := gpuMetricsSignature{name: name, instances: instances}
	for k := range s.encodings {
		s.encodings[k] = gpuMetricsEncoding{typ, unit}
	}
	return s
}

// byKernel returns the signature of a scalar field with an encoding for each kernel.
func byKernel(name string, encodings ...gpuMetricsEncoding) gpuMetricsSignature {
	if len(encodings) != int(kernelCount) {
		panic(fmt.Sprintf("gpu_metrics field %s: %d encodings for %d kernels", name, len(encodings), kernelCount))
	}
	s := gpuMetricsSignature{name: name, instances: 1}
	copy(s.encodings[:], encodings)
	return s
}

// anchored makes a signature an anchor that also skips the given instance counts.
func (s gpuMetricsSignature) anchored(skip ...int) gpuMetricsSignature {
	s.anchor, s.skip = true, skip
	return s
}

// fieldAction is what the decoder does with a used field.
type fieldAction int

const (
	fieldUse fieldAction = iota
	fieldSkip
	fieldReject
)

// action returns what to do with a used field of the given encoding.
func (s gpuMetricsSignature) action(typ gpuMetricsType, unit gpuMetricsUnit, instances int) fieldAction {
	known := false
	for _, e := range s.encodings {
		known = known || (e.typ == typ && e.unit == unit)
	}
	switch {
	case !known:
		return fieldReject
	case instances == s.instances:
		return fieldUse
	case !s.anchor || slices.Contains(s.skip, instances):
		return fieldSkip
	}
	return fieldReject
}

var (
	u32None    = gpuMetricsEncoding{typeU32, unitNone}
	u64None    = gpuMetricsEncoding{typeU64, unitNone}
	u32Percent = gpuMetricsEncoding{typeU32, unitPercent}
	u64Percent = gpuMetricsEncoding{typeU64, unitPercent}
)

// gpuMetricsSignatures are the fields the GPU check uses, with the encodings
// amdgpu gives them (smu_v13_0_6_ppt.h and smu_v15_0_8_ppt.h). The system
// clock and accumulation counters are anchors, as is CURRENT_UCLK, which is
// only checked: it is the one used field after the XGMI counters, and SMU
// 15.0.8 reports it for 2 AIDs.
var gpuMetricsSignatures = map[gpuMetricsAttrID]gpuMetricsSignature{
	attrSystemClockCounter:    same("system_clock_counter", 1, typeU64, unitTimeNs).anchored(),
	attrAccumulationCounter:   byKernel("accumulation_counter", u32None, u64None, u64None).anchored(),
	attrProchotResidencyAcc:   byKernel("prochot_residency_acc", u32None, u32None, u64None),
	attrPPTResidencyAcc:       byKernel("ppt_residency_acc", u32None, u32None, u64None),
	attrSocketThmResidencyAcc: byKernel("socket_thm_residency_acc", u32None, u32None, u64None),
	attrXGMILinkWidth:         same("xgmi_link_width", 1, typeU16, unitNone),
	attrXGMILinkSpeed:         same("xgmi_link_speed", 1, typeU16, unitSpeedGTs),
	attrGFXActivityAcc:        byKernel("gfx_activity_acc", u32Percent, u32Percent, u64None),
	attrMemActivityAcc:        byKernel("mem_activity_acc", u32Percent, u32Percent, u64None),
	attrPCIeReplayCountAcc:    same("pcie_replay_count_acc", 1, typeU64, unitNone),
	// Per link on SMU 13.0.6 and 13.0.12 (XgmiRead/WriteDataSizeAcc, KB). SMU
	// 15.0.8 has one instance, from XgmiReadBandwidthAcc, which may not be a
	// byte count, so it is skipped. TODO: decode it once validated on MI4xx.
	attrXGMIReadDataAcc:  same("xgmi_read_data_acc", numXGMILinks, typeU64, unitDataKB),
	attrXGMIWriteDataAcc: same("xgmi_write_data_acc", numXGMILinks, typeU64, unitDataKB),
	attrCurrentUCLK:      same("current_uclk", 1, typeU16, unitClockMHz).anchored(2),
	attrGFXBusyAcc: {name: "gfx_busy_acc", instances: 8,
		encodings: [kernelCount]gpuMetricsEncoding{u64Percent, u64Percent, u64None}},
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
// is checked against its signature before use.
func (v GPUMetricsVersion) dynamic() bool {
	return v.Format == 1 && v.Content >= 9
}

// Counter is a raw gpu_metrics value. Valid is false when the field is
// absent or was not filled by the firmware.
type Counter struct {
	Value uint64
	Valid bool
}

// SkippedField is a used field left out of a table because its instance count
// is not the decoded one, with its encoding (e.g. "U64/DATA_1/x1").
type SkippedField struct {
	Name     string
	Encoding string
}

// GPUMetrics holds the fields of a gpu_metrics table the GPU check uses.
type GPUMetrics struct {
	Version GPUMetricsVersion

	// SystemClockNs is the boot time, in ns, at which the driver filled the table.
	SystemClockNs Counter
	// AccumulationCounter counts the firmware accumulation periods, about 1
	// ms each: the residency counters below increase by one for each period
	// throttled.
	AccumulationCounter    Counter
	ProchotResidency       Counter
	PPTResidency           Counter
	SocketThermalResidency Counter

	// GFXActivityAcc accumulates the graphics activity of the socket, the
	// mean over its XCDs, in percent per accumulation period. MemActivityAcc
	// accumulates the HBM bandwidth utilization, in percent of the peak per
	// period.
	GFXActivityAcc Counter
	MemActivityAcc Counter
	// gfxBusyMean is the mean of the filled per-XCD GFX_BUSY_ACC, invalid when
	// none is filled, and gfxActivityMask the width of GFX_ACTIVITY_ACC.
	gfxBusyMean     Counter
	gfxActivityMask uint64

	// PCIeReplayCount is a cumulative firmware counter of PCIe replays; on a
	// VF it can persist across guest driver loads.
	PCIeReplayCount Counter

	// XGMIReadKB and XGMIWriteKB are cumulative firmware counters of the data,
	// in KB, read and written over each XGMI link. They are nil when absent
	// or skipped.
	XGMIReadKB  []Counter
	XGMIWriteKB []Counter
	// XGMILinkSpeedGTs and XGMILinkWidth are the speed per lane and the width
	// of the XGMI links.
	XGMILinkSpeedGTs Counter
	XGMILinkWidth    Counter

	// Skipped lists the used fields left out for their instance count.
	Skipped []SkippedField
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
// an inconsistent structure returns ErrGPUMetricsMalformed, and one with a used
// field of unexpected encoding ErrGPUMetricsUnexpectedEncoding, with no value.
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
	var rejected []string
	values := make(map[gpuMetricsAttrID][]Counter, len(gpuMetricsSignatures))
	for _, id := range slices.Sorted(maps.Keys(gpuMetricsSignatures)) {
		attr, present := attrs[id]
		if !present {
			continue
		}
		sig := gpuMetricsSignatures[id]
		encoding := encodingName(attr.typ, attr.unit, len(attr.values))
		switch action := sig.action(attr.typ, attr.unit, len(attr.values)); {
		case attr.duplicate:
			rejected = append(rejected, sig.name+" duplicated")
		case action == fieldReject:
			rejected = append(rejected, sig.name+" "+encoding)
		case action == fieldSkip:
			gm.Skipped = append(gm.Skipped, SkippedField{Name: sig.name, Encoding: encoding})
		default:
			values[id] = counterValues(attr)
		}
	}
	if len(rejected) > 0 {
		return &GPUMetrics{Version: version}, fmt.Errorf("%w (version %s): %s", ErrGPUMetricsUnexpectedEncoding, version, strings.Join(rejected, ", "))
	}

	scalar := func(id gpuMetricsAttrID) Counter {
		if v := values[id]; v != nil {
			return v[0]
		}
		return Counter{}
	}
	gm.SystemClockNs = scalar(attrSystemClockCounter)
	gm.AccumulationCounter = scalar(attrAccumulationCounter)
	gm.ProchotResidency = scalar(attrProchotResidencyAcc)
	gm.PPTResidency = scalar(attrPPTResidencyAcc)
	gm.SocketThermalResidency = scalar(attrSocketThmResidencyAcc)
	gm.PCIeReplayCount = scalar(attrPCIeReplayCountAcc)
	gm.GFXActivityAcc = scalar(attrGFXActivityAcc)
	gm.MemActivityAcc = scalar(attrMemActivityAcc)
	gm.XGMIReadKB = values[attrXGMIReadDataAcc]
	gm.XGMIWriteKB = values[attrXGMIWriteDataAcc]
	gm.XGMILinkSpeedGTs = scalar(attrXGMILinkSpeed)
	gm.XGMILinkWidth = scalar(attrXGMILinkWidth)
	if gm.GFXActivityAcc.Valid {
		gm.gfxActivityMask = uint64(math.MaxUint64) >> (64 - 8*attrs[attrGFXActivityAcc].typ.size())
		gm.gfxBusyMean = meanCounter(values[attrGFXBusyAcc])
	}
	return gm, nil
}

// counterValues returns the values of an attribute. Values the firmware did
// not fill (all bits set) are invalid.
func counterValues(attr *gpuMetricsAttr) []Counter {
	unfilled := uint64(math.MaxUint64) >> (64 - 8*attr.typ.size())
	values := make([]Counter, len(attr.values))
	for i, v := range attr.values {
		values[i] = Counter{Value: v, Valid: v != unfilled}
	}
	return values
}

// meanCounter returns the mean of the valid values, invalid when there is none.
func meanCounter(values []Counter) Counter {
	var hi, lo, n uint64
	for _, v := range values {
		if v.Valid {
			var carry uint64
			lo, carry = bits.Add64(lo, v.Value, 0)
			hi += carry
			n++
		}
	}
	if n == 0 {
		return Counter{}
	}
	mean, _ := bits.Div64(hi, lo, n) // hi < n, as n values of 64 bits were added
	return Counter{Value: mean, Valid: true}
}

// GFXActivityMatchesBusy reports whether GFX_ACTIVITY_ACC increased between
// two tables, over periods accumulation periods, as the mean of the per-XCD
// GFX_BUSY_ACC did, modulo its width and within gfxBusyTolerance. It is true
// when either table has no GFX_BUSY_ACC to compare with.
func GFXActivityMatchesBusy(previous, current *GPUMetrics, periods uint64) bool {
	if !previous.gfxBusyMean.Valid || !current.gfxBusyMean.Valid || previous.gfxActivityMask != current.gfxActivityMask {
		return true
	}
	mask := current.gfxActivityMask
	activity := (current.GFXActivityAcc.Value - previous.GFXActivityAcc.Value) & mask
	busy := (current.gfxBusyMean.Value - previous.gfxBusyMean.Value) & mask
	diff := (activity - busy) & mask
	return min(diff, (mask-diff+1)&mask) <= gfxBusyTolerance(periods)
}

// gfxBusyTolerance is the largest difference, in percent-periods, between the
// increases of GFX_ACTIVITY_ACC and of the mean of GFX_BUSY_ACC over periods.
// Counters sampled up to a period apart at each end of the interval differ by
// up to gfxBusySkewPeriods at 100%; rounding drift adds a 1/gfxBusyDriftPeriods
// share of the interval, far above the 131 over 36M periods of a real MI350X.
func gfxBusyTolerance(periods uint64) uint64 {
	return 100 * (gfxBusySkewPeriods + periods/gfxBusyDriftPeriods)
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

// readGPUMetricsFile reads the gpu_metrics attribute; tests replace it.
var readGPUMetricsFile = readFileLimited

// readFileLimited reads at most limit bytes of a file through one file
// descriptor: amdgpu generates the whole table on the first read, so the
// table is consistent.
func readFileLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}

// gpuMetrics reads and decodes the gpu_metrics attribute of a device in a
// single read. It returns nil without an error when the device has none or is
// runtime-suspended.
func (r *reader) gpuMetrics(devicePath string) (*GPUMetrics, error) {
	path := filepath.Join(devicePath, "gpu_metrics")
	data, err := readGPUMetricsFile(path, maxGPUMetricsSize+1)
	if err != nil {
		// amdgpu returns EPERM while the device is runtime-suspended, as for
		// the busy percentages; during a GPU reset it stays an error.
		if !isUnsupported(err) && (!errors.Is(err, syscall.EPERM) || !runtimeSuspended(devicePath)) {
			r.errs = append(r.errs, err)
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
