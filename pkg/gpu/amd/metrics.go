// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package amd

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Reading is a single telemetry value. Valid is false when the device does not
// expose the attribute, so that absent values are not confused with zeros.
type Reading struct {
	Value float64
	Valid bool
}

func valid(v float64) Reading {
	return Reading{Value: v, Valid: true}
}

// Metrics is a snapshot of the telemetry of one device, already converted to
// the units used by the GPU check.
type Metrics struct {
	GPUBusyPercent    Reading // gpu_busy_percent, 0-100
	MemoryBusyPercent Reading // mem_busy_percent, 0-100
	VRAMTotalBytes    Reading // mem_info_vram_total
	VRAMUsedBytes     Reading // mem_info_vram_used

	EdgeTemperatureC     Reading // hwmon sensor labeled "edge"
	JunctionTemperatureC Reading // hwmon sensor labeled "junction" (hotspot)
	MemoryTemperatureC   Reading // hwmon sensor labeled "mem" (HBM/VRAM)

	PowerMilliwatts    Reading // hwmon power1_average, or power1_input when averaging is not exposed
	PowerCapMilliwatts Reading // hwmon power1_cap

	GraphicsClockMHz Reading // hwmon sclk, or else the active level of pp_dpm_sclk
	MemoryClockMHz   Reading // hwmon mclk, or else the active level of pp_dpm_mclk

	PCIeLinkWidth       Reading // current_link_width, lanes
	PCIeMaxLinkWidth    Reading // max_link_width, lanes
	PCIeLinkSpeedGTs    Reading // current_link_speed, GT/s per lane
	PCIeMaxLinkSpeedGTs Reading // max_link_speed, GT/s per lane

	// ECC error counts summed over the RAS blocks, like the amd-smi totals.
	// They are invalid when a block count cannot be read or parsed, or no
	// enabled block has one, and the deferred count also when no block
	// reports deferred errors.
	ECCUncorrected Reading
	ECCCorrected   Reading
	ECCDeferred    Reading

	// GPUMetrics is the decoded gpu_metrics table, nil when the device has
	// none. When GPUMetricsErr is set (ErrGPUMetricsUnsupported or
	// ErrGPUMetricsMalformed), it only holds the version from the header, or
	// is nil when the header itself is truncated.
	GPUMetrics    *GPUMetrics
	GPUMetricsErr error
	// XGMIHiveSize is the number of GPUs in the XGMI hive of the device, 0
	// when it is not in a hive.
	XGMIHiveSize int
}

// ReadOptions selects the optional parts of a telemetry read.
type ReadOptions struct {
	// SkipGPUMetrics leaves the gpu_metrics table and the XGMI hive size unread.
	SkipGPUMetrics bool
}

// ReadMetrics reads the current telemetry of the device, gpu_metrics included.
func (d *Device) ReadMetrics() (Metrics, error) {
	return d.ReadMetricsWithOptions(ReadOptions{})
}

// ReadMetricsWithOptions reads the current telemetry of the device. Attributes
// the device does not implement are left invalid without an error; the
// returned error only reports attributes that exist but could not be read or
// parsed, and the returned metrics still contain every value that was read
// successfully.
func (d *Device) ReadMetricsWithOptions(opts ReadOptions) (Metrics, error) {
	r := reader{}
	var m Metrics

	m.GPUBusyPercent = r.activity(d.devicePath, "gpu_busy_percent")
	m.MemoryBusyPercent = r.activity(d.devicePath, "mem_busy_percent")
	m.VRAMTotalBytes = r.uint(filepath.Join(d.devicePath, "mem_info_vram_total"), 1)
	m.VRAMUsedBytes = r.uint(filepath.Join(d.devicePath, "mem_info_vram_used"), 1)

	m.PCIeLinkWidth = r.linkWidth(filepath.Join(d.devicePath, "current_link_width"))
	m.PCIeMaxLinkWidth = r.linkWidth(filepath.Join(d.devicePath, "max_link_width"))
	m.PCIeLinkSpeedGTs = r.linkSpeed(filepath.Join(d.devicePath, "current_link_speed"))
	m.PCIeMaxLinkSpeedGTs = r.linkSpeed(filepath.Join(d.devicePath, "max_link_speed"))

	hwmonDir := r.hwmonDir(d.devicePath)
	var hwmonEntries []os.DirEntry
	if hwmonDir != "" {
		hwmonEntries = r.readDir(hwmonDir)
		r.readHwmon(hwmonDir, hwmonEntries, &m)
	}
	m.GraphicsClockMHz = r.clock(hwmonDir, hwmonEntries, "sclk", filepath.Join(d.devicePath, "pp_dpm_sclk"))
	m.MemoryClockMHz = r.clock(hwmonDir, hwmonEntries, "mclk", filepath.Join(d.devicePath, "pp_dpm_mclk"))
	r.readECC(filepath.Join(d.devicePath, "ras"), &m)

	if !opts.SkipGPUMetrics {
		m.GPUMetrics, m.GPUMetricsErr = r.gpuMetrics(d.devicePath)
		m.XGMIHiveSize = d.XGMIHiveSize()
	}

	return m, errors.Join(r.errs...)
}

// hwmonDir returns the hwmon directory of the device, or "" if it has none.
func (r *reader) hwmonDir(devicePath string) string {
	dir := filepath.Join(devicePath, "hwmon")
	for _, entry := range r.readDir(dir) {
		if strings.HasPrefix(entry.Name(), "hwmon") {
			return filepath.Join(dir, entry.Name())
		}
	}
	return ""
}

// reader accumulates the errors of a collection pass.
type reader struct {
	errs []error
}

// read returns the trimmed attribute content. ok is false when the attribute
// is absent or not supported by the device in its current state.
func (r *reader) read(path string) (string, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		if !isUnsupported(err) {
			r.errs = append(r.errs, err)
		}
		return "", false
	}
	return sysfsValue(content), true
}

func (r *reader) readDir(path string) []os.DirEntry {
	entries, err := os.ReadDir(path)
	if err != nil && !isUnsupported(err) {
		r.errs = append(r.errs, err)
	}
	return entries
}

// isUnsupported reports whether a read error means the attribute is not
// available on this device, as opposed to a real failure such as a permission
// error. amdgpu returns these errnos for attributes an ASIC does not implement.
func isUnsupported(err error) bool {
	return errors.Is(err, os.ErrNotExist) ||
		errors.Is(err, syscall.EOPNOTSUPP) ||
		errors.Is(err, syscall.ENODATA) ||
		errors.Is(err, syscall.EINVAL) ||
		errors.Is(err, syscall.ENODEV)
}

func (r *reader) uint(path string, scale float64) Reading {
	content, ok := r.read(path)
	if !ok {
		return Reading{}
	}
	return r.parseUint(path, content, scale)
}

func (r *reader) parseUint(path, content string, scale float64) Reading {
	value, err := strconv.ParseUint(content, 10, 64)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("parse %s: %w", path, err))
		return Reading{}
	}
	return valid(float64(value) * scale)
}

// readActivityFile reads a busy percentage attribute; tests replace it.
var readActivityFile = os.ReadFile

// activity reads a busy percentage. amdgpu returns EPERM for it while the
// device is runtime-suspended (or suspending or resuming), which reads as
// idle, but also during a GPU reset, which stays an error.
func (r *reader) activity(devicePath, name string) Reading {
	path := filepath.Join(devicePath, name)
	content, err := readActivityFile(path)
	if errors.Is(err, syscall.EPERM) && runtimeSuspended(devicePath) {
		return valid(0)
	}
	if err != nil {
		if !isUnsupported(err) {
			r.errs = append(r.errs, err)
		}
		return Reading{}
	}
	return r.parseUint(path, sysfsValue(content), 1)
}

// runtimeSuspended reports whether the device is runtime-suspended, or
// suspending or resuming, when amdgpu also returns EPERM.
func runtimeSuspended(devicePath string) bool {
	content, err := os.ReadFile(filepath.Join(devicePath, "power", "runtime_status"))
	if err != nil {
		return false
	}
	switch sysfsValue(content) {
	case "suspended", "suspending", "resuming":
		return true
	}
	return false
}

// linkWidth reads a PCI link width. A width of 0 (no link trained) is unavailable.
func (r *reader) linkWidth(path string) Reading {
	width := r.uint(path, 1)
	if width.Value == 0 {
		return Reading{}
	}
	return width
}

func (r *reader) int(path string, scale float64) Reading {
	content, ok := r.read(path)
	if !ok {
		return Reading{}
	}
	value, err := strconv.ParseInt(content, 10, 64)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("parse %s: %w", path, err))
		return Reading{}
	}
	return valid(float64(value) * scale)
}

// activeDPMLevel returns the frequency, in MHz, of the level marked with "*"
// in a pp_dpm_* table. Lines look like "1: 1700Mhz *", "S: 19Mhz *" (deep
// sleep) or "0:        872Mhz"; the value and its unit may be separated by
// spaces. This follows the parser of ROCm SMI (rocm_smi.cc, freq_lines).
func (r *reader) activeDPMLevel(path, content string) Reading {
	for line := range strings.Lines(content) {
		if !strings.Contains(line, "*") {
			continue
		}
		mhz, err := parseDPMFrequency(line)
		if err != nil {
			r.errs = append(r.errs, fmt.Errorf("parse %s: %q: %w", path, strings.TrimSpace(line), err))
			return Reading{}
		}
		return valid(mhz)
	}
	return Reading{}
}

// parseDPMFrequency parses the frequency of one pp_dpm_* line, in MHz.
func parseDPMFrequency(line string) (float64, error) {
	_, level, found := strings.Cut(line, ":")
	if !found {
		return 0, errors.New("missing level index")
	}
	level = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(level), "*"))
	end := strings.IndexFunc(level, func(c rune) bool { return (c < '0' || c > '9') && c != '.' })
	if end <= 0 {
		return 0, errors.New("missing frequency")
	}
	value, err := strconv.ParseFloat(level[:end], 64)
	if err != nil {
		return 0, err
	}
	var mhz float64
	switch unit := strings.ToLower(strings.TrimSpace(level[end:])); {
	case strings.HasPrefix(unit, "ghz"):
		mhz = value * 1000
	case strings.HasPrefix(unit, "mhz"):
		mhz = value
	case strings.HasPrefix(unit, "khz"):
		mhz = value / 1000
	default:
		return 0, fmt.Errorf("unknown frequency unit %q", unit)
	}
	if math.IsInf(mhz, 0) {
		return 0, fmt.Errorf("frequency %q out of range", level[:end])
	}
	return mhz, nil
}

// linkSpeed parses a PCI link speed such as "32.0 GT/s PCIe".
// Unknown speeds are unavailable rather than parse failures.
func (r *reader) linkSpeed(path string) Reading {
	content, ok := r.read(path)
	if !ok || content == "Unknown" || content == "Unknown speed" {
		return Reading{}
	}
	rate, suffix, found := strings.Cut(content, " GT/s")
	if !found || (suffix != "" && suffix != " PCIe") {
		r.errs = append(r.errs, fmt.Errorf("parse %s: invalid link speed %q", path, content))
		return Reading{}
	}
	value, err := strconv.ParseFloat(rate, 64)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("parse %s: %w", path, err))
		return Reading{}
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		r.errs = append(r.errs, fmt.Errorf("parse %s: invalid transfer rate %q", path, rate))
		return Reading{}
	}
	return valid(value)
}

const (
	milliDegreesPerDegree   = 1000
	microwattsPerMilliwatts = 1000
	hertzPerMegahertz       = 1000000
)

func (r *reader) readHwmon(dir string, entries []os.DirEntry, m *Metrics) {
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "temp") || !strings.HasSuffix(name, "_label") {
			continue
		}
		labelPath := filepath.Join(dir, name)
		label, ok := r.read(labelPath)
		if !ok {
			continue
		}
		inputPath := strings.TrimSuffix(labelPath, "_label") + "_input"
		switch label {
		case "edge":
			m.EdgeTemperatureC = r.int(inputPath, 1.0/milliDegreesPerDegree)
		case "junction":
			m.JunctionTemperatureC = r.int(inputPath, 1.0/milliDegreesPerDegree)
		case "mem":
			m.MemoryTemperatureC = r.int(inputPath, 1.0/milliDegreesPerDegree)
		}
	}

	m.PowerMilliwatts = r.uint(filepath.Join(dir, "power1_average"), 1.0/microwattsPerMilliwatts)
	if !m.PowerMilliwatts.Valid {
		m.PowerMilliwatts = r.uint(filepath.Join(dir, "power1_input"), 1.0/microwattsPerMilliwatts)
	}
	m.PowerCapMilliwatts = r.uint(filepath.Join(dir, "power1_cap"), 1.0/microwattsPerMilliwatts)
}

// clock reads a clock in MHz from hwmon, on AMD Instinct GPUs the SMU's current
// clock (amd-smi's gfx_0 for sclk), or else from the level marked in pp_dpm_*,
// which since Linux 7.0 and recent amdgpu-dkms marks none between levels.
// pp_dpm_* is read first: during a RAS interrupt it reads "unavailable" or
// fails, while hwmon returns a value the driver did not set.
func (r *reader) clock(hwmonDir string, hwmonEntries []os.DirEntry, label, dpmPath string) Reading {
	errs := len(r.errs)
	table, ok := r.read(dpmPath)
	if (!ok && len(r.errs) > errs) || table == "unavailable" {
		return Reading{}
	}
	if c := r.hwmonClock(hwmonDir, hwmonEntries, label); c.Valid {
		return c
	}
	return r.activeDPMLevel(dpmPath, table)
}

// hwmonClock reads the hwmon clock with the given label, in MHz.
func (r *reader) hwmonClock(dir string, entries []os.DirEntry, label string) Reading {
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "freq") || !strings.HasSuffix(name, "_label") {
			continue
		}
		labelPath := filepath.Join(dir, name)
		if l, ok := r.read(labelPath); ok && l == label {
			return r.uint(strings.TrimSuffix(labelPath, "_label")+"_input", 1.0/hertzPerMegahertz)
		}
	}
	return Reading{}
}

// rasBlocks are the RAS blocks amd-smi adds up for its total ECC counts, with
// their bit in ras/features (amdsmi_get_gpu_total_ecc_count and
// rsmi_dev_ecc_count_get in ROCm/rocm-systems projects/amdsmi).
var rasBlocks = []struct {
	name string
	bit  uint
}{{"umc", 0}, {"sdma", 1}, {"gfx", 2}, {"mmhub", 3}, {"pcie_bif", 5}, {"hdp", 6}, {"xgmi_wafl", 7}}

// rasQueryInaccessible is what amdgpu returns for an error count attribute
// while it cannot query errors, such as during a GPU reset.
const rasQueryInaccessible = "Query currently inaccessible"

// readRASFile reads an error count attribute; tests replace it.
var readRASFile = os.ReadFile

// readECC sums the error counts of the RAS blocks enabled in ras/features, as
// amd-smi does. A block without a count attribute is left out. Nothing is
// reported when a count attribute cannot be read or parsed: the driver fails
// the query during a GPU reset, and a partial sum would undercount.
func (r *reader) readECC(rasDir string, m *Metrics) {
	featuresPath := filepath.Join(rasDir, "features")
	features, ok := r.read(featuresPath)
	if !ok {
		return
	}
	mask, err := parseRASFeatures(features)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("parse %s: %w", featuresPath, err))
		return
	}

	var uncorrected, corrected, deferred uint64
	var found, foundDeferred bool
	for _, block := range rasBlocks {
		if mask&(1<<block.bit) == 0 {
			continue
		}
		path := filepath.Join(rasDir, block.name+"_err_count")
		data, err := readRASFile(path)
		if errors.Is(err, os.ErrNotExist) {
			// Linux 6.15 to 7.1 name it aca_<block> on MI300/MI350-class
			// GPUs (ACA).
			path = filepath.Join(rasDir, "aca_"+block.name)
			data, err = readRASFile(path)
		}
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			r.errs = append(r.errs, err)
			return
		}
		content := sysfsValue(data)
		if content == rasQueryInaccessible {
			return
		}
		counts, err := parseErrCount(content)
		if err != nil {
			r.errs = append(r.errs, fmt.Errorf("parse %s: %w", path, err))
			return
		}
		found = true
		uncorrected += counts["ue"]
		corrected += counts["ce"]
		if de, ok := counts["de"]; ok {
			foundDeferred = true
			deferred += de
		}
	}
	if found {
		m.ECCUncorrected = valid(float64(uncorrected))
		m.ECCCorrected = valid(float64(corrected))
	}
	if foundDeferred {
		m.ECCDeferred = valid(float64(deferred))
	}
}

// parseRASFeatures parses ras/features, such as "feature mask: 0x20065bff".
func parseRASFeatures(content string) (uint64, error) {
	value, found := strings.CutPrefix(content, "feature mask:")
	if !found {
		return 0, fmt.Errorf("invalid feature mask %q", content)
	}
	return strconv.ParseUint(strings.TrimPrefix(strings.TrimSpace(value), "0x"), 16, 64)
}

// parseErrCount parses an error count attribute, such as "ue: 0\nce: 2\nde: 0"
// (de is only reported by UMC, or by all blocks with ACA). The ue and ce counts
// are required.
func parseErrCount(content string) (map[string]uint64, error) {
	counts := make(map[string]uint64, 3)
	for line := range strings.Lines(content) {
		key, value, _ := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		if key != "ue" && key != "ce" && key != "de" {
			continue
		}
		count, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s count: %w", key, err)
		}
		counts[key] = count
	}
	for _, key := range []string{"ue", "ce"} {
		if _, ok := counts[key]; !ok {
			return nil, fmt.Errorf("missing %s count", key)
		}
	}
	return counts, nil
}
