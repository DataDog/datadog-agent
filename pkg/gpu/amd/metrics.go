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

	GraphicsClockMHz Reading // active level of pp_dpm_sclk
	MemoryClockMHz   Reading // active level of pp_dpm_mclk

	PCIeLinkWidth       Reading // current_link_width, lanes
	PCIeMaxLinkWidth    Reading // max_link_width, lanes
	PCIeLinkSpeedGTs    Reading // current_link_speed, GT/s per lane
	PCIeMaxLinkSpeedGTs Reading // max_link_speed, GT/s per lane
}

// ReadMetrics reads the current telemetry of the device. Attributes the device
// does not implement are left invalid without an error; the returned error
// only reports attributes that exist but could not be read or parsed, and the
// returned metrics still contain every value that was read successfully.
func (d *Device) ReadMetrics() (Metrics, error) {
	r := reader{}
	var m Metrics

	m.GPUBusyPercent = r.uint(filepath.Join(d.devicePath, "gpu_busy_percent"), 1)
	m.MemoryBusyPercent = r.uint(filepath.Join(d.devicePath, "mem_busy_percent"), 1)
	m.VRAMTotalBytes = r.uint(filepath.Join(d.devicePath, "mem_info_vram_total"), 1)
	m.VRAMUsedBytes = r.uint(filepath.Join(d.devicePath, "mem_info_vram_used"), 1)

	m.GraphicsClockMHz = r.activeDPMLevel(filepath.Join(d.devicePath, "pp_dpm_sclk"))
	m.MemoryClockMHz = r.activeDPMLevel(filepath.Join(d.devicePath, "pp_dpm_mclk"))

	m.PCIeLinkWidth = r.uint(filepath.Join(d.devicePath, "current_link_width"), 1)
	m.PCIeMaxLinkWidth = r.uint(filepath.Join(d.devicePath, "max_link_width"), 1)
	m.PCIeLinkSpeedGTs = r.linkSpeed(filepath.Join(d.devicePath, "current_link_speed"))
	m.PCIeMaxLinkSpeedGTs = r.linkSpeed(filepath.Join(d.devicePath, "max_link_speed"))

	if hwmonDir := r.hwmonDir(d.devicePath); hwmonDir != "" {
		r.readHwmon(hwmonDir, &m)
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
	return strings.TrimSpace(string(content)), true
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
	value, err := strconv.ParseUint(content, 10, 64)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("parse %s: %w", path, err))
		return Reading{}
	}
	return valid(float64(value) * scale)
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
func (r *reader) activeDPMLevel(path string) Reading {
	content, ok := r.read(path)
	if !ok {
		return Reading{}
	}
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
	switch unit := strings.ToLower(strings.TrimSpace(level[end:])); {
	case strings.HasPrefix(unit, "ghz"):
		return value * 1000, nil
	case strings.HasPrefix(unit, "mhz"):
		return value, nil
	case strings.HasPrefix(unit, "khz"):
		return value / 1000, nil
	default:
		return 0, fmt.Errorf("unknown frequency unit %q", unit)
	}
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
)

func (r *reader) readHwmon(dir string, m *Metrics) {
	for _, entry := range r.readDir(dir) {
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
