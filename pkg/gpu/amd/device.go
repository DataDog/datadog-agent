// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package amd discovers AMD GPUs driven by the amdgpu kernel driver and reads
// their telemetry from sysfs. It has no dependency on ROCm or amd-smi.
// AMD attributes are documented at
// https://docs.kernel.org/gpu/amdgpu/driver-misc.html and
// https://docs.kernel.org/gpu/amdgpu/thermal.html.
package amd

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	// Vendor is the vendor name used for AMD devices in tags.
	Vendor = "amd"

	amdPCIVendorID = 0x1002
	amdgpuDriver   = "amdgpu"
)

var (
	// cardDirRegex matches DRM card nodes (card0, card12) but not connectors (card0-DP-1).
	cardDirRegex = regexp.MustCompile(`^card[0-9]+$`)
	// pciAddressRegex matches a PCI function address such as 0000:c1:00.0.
	pciAddressRegex = regexp.MustCompile(`^[0-9a-f]{4,8}:[0-9a-f]{2}:[0-9a-f]{2}\.[0-7]$`)
	// uniqueIDRegex matches the amdgpu unique_id attribute (a hex serial).
	uniqueIDRegex = regexp.MustCompile(`^[0-9a-f]+$`)
	pciIDReplacer = strings.NewReplacer(":", "-", ".", "-")
)

// Device is a physical AMD GPU bound to the amdgpu driver.
type Device struct {
	// UUID is a stable identifier: "amd-" followed by the unique_id attribute
	// when it is a valid nonzero 64-bit hex serial, or by the PCI address otherwise.
	UUID string
	// Index is the position of the device when sorted by PCI address.
	Index int
	// Name is the product name, from product_name or a device ID table.
	Name string
	// PCIBusID is the PCI function address, e.g. 0000:c1:00.0.
	PCIBusID string
	// DeviceID is the PCI device ID, e.g. 0x74a1.
	DeviceID uint16
	// DriverVersion is the amdgpu module version, empty when the module does
	// not export one (in-tree driver builds).
	DriverVersion string
	// Architecture is the LLVM gfx target from the KFD topology (e.g. gfx942),
	// empty when the compute driver does not report it.
	Architecture string
	// MemoryTotal is the VRAM size in bytes (mem_info_vram_total), 0 if unknown.
	MemoryTotal uint64
	// MaxEngineClockMHz is the highest compute engine clock
	// (max_engine_clk_fcompute) from the KFD topology, 0 if unknown.
	MaxEngineClockMHz uint32
	// MaxMemoryClockMHz is the video memory clock (mem_clk_max) from the KFD
	// topology, 0 if unknown.
	MaxMemoryClockMHz uint32
	// MemoryBusWidthBits is the width of the video memory bus in bits, from the
	// KFD topology. It is 0 when unknown, and for compute-partitioned GPUs.
	MemoryBusWidthBits uint32
	// KFDAccessDenied marks process topology made incomplete by a denied KFD
	// node. A denied node's PCI owner is unknown, so all discovered GPUs are
	// conservatively affected. Readable architecture and device data remain
	// available, but complete physical-GPU process sums cannot be established.
	KFDAccessDenied bool

	// devicePath is the sysfs directory of the PCI device.
	devicePath string
	// kfdGPUIDs are the KFD gpu_id of the device, one per compute partition.
	kfdGPUIDs []uint64
	// renderMinors are the DRM render node minors of the device's KFD nodes.
	renderMinors []int
	// kfdTopologyIncomplete marks a KFD topology with a GPU node that could not
	// be mapped to a physical GPU, for any reason. Like KFDAccessDenied, it
	// affects all discovered GPUs and withholds process sums.
	kfdTopologyIncomplete bool
}

// Discover returns the AMD GPUs found under sysRoot (normally /sys), sorted
// by PCI address. A missing DRM class directory is not an error.
// Compute partitions (XCP) with platform-device card nodes are skipped:
// the ASIC is reported once, through its physical PCI device.
func Discover(sysRoot string) ([]*Device, error) {
	drmDir := filepath.Join(sysRoot, "class", "drm")
	entries, err := os.ReadDir(drmDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("list %s: %w", drmDir, err)
	}

	driverVersion := readTrimmed(filepath.Join(sysRoot, "module", amdgpuDriver, "version"))

	seen := make(map[string]struct{})
	byUUID := make(map[string]*Device)
	var devices []*Device
	var errs []error
	for _, entry := range entries {
		if !cardDirRegex.MatchString(entry.Name()) {
			continue
		}
		dev, err := probeCard(filepath.Join(drmDir, entry.Name(), "device"))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry.Name(), err))
			continue
		}
		if dev == nil {
			continue
		}
		if _, dup := seen[dev.PCIBusID]; dup {
			continue
		}
		seen[dev.PCIBusID] = struct{}{}
		dev.DriverVersion = driverVersion
		if previous, duplicate := byUUID[dev.UUID]; duplicate {
			// A shared serial cannot identify distinct PCI functions. Give
			// both devices PCI identities rather than merging their metrics.
			previous.UUID = pciUUID(previous.PCIBusID)
			dev.UUID = pciUUID(dev.PCIBusID)
		} else {
			byUUID[dev.UUID] = dev
		}
		devices = append(devices, dev)
	}

	slices.SortFunc(devices, func(a, b *Device) int { return cmp.Compare(a.PCIBusID, b.PCIBusID) })
	for i, dev := range devices {
		dev.Index = i
	}

	if len(devices) > 0 {
		kfd, denied, complete, err := readKFDTopology(sysRoot, seen)
		if err != nil {
			errs = append(errs, fmt.Errorf("KFD topology: %w", err))
		}
		for _, dev := range devices {
			dev.KFDAccessDenied = denied > 0
			dev.kfdTopologyIncomplete = !complete
			if gpu, ok := kfd[dev.PCIBusID]; ok {
				dev.Architecture = gpu.gfxTarget
				dev.kfdGPUIDs = gpu.gpuIDs
				dev.renderMinors = gpu.renderMinors
				dev.MaxEngineClockMHz = gpu.maxEngineClockMHz
				dev.MaxMemoryClockMHz = gpu.maxMemoryClockMHz
				dev.MemoryBusWidthBits = gpu.busWidthBits
			}
		}
	}
	return devices, errors.Join(errs...)
}

// KFDAccessDeniedWarning describes incomplete process topology caused by a
// permission denial, or returns "" when the topology has no denied nodes. It is the
// one place that explains the cause, so that callers can log it through a rate
// limiter instead of reporting a raw read error on every call.
func KFDAccessDeniedWarning(devices []*Device) string {
	var ids []string
	for _, dev := range devices {
		if dev.KFDAccessDenied {
			ids = append(ids, dev.PCIBusID)
		}
	}
	if len(ids) == 0 {
		return ""
	}
	return fmt.Sprintf("complete process attribution for AMD GPU(s) %s is unavailable because some KFD topology nodes are not readable (operation not permitted): "+
		"in a container, the kernel requires read/write access to each GPU partition's render node in the device cgroup. "+
		"Configure GPU device access in the container runtime; mounting /dev alone is insufficient and full container privilege is not required. "+
		"Device metrics and architecture from readable nodes are still reported, "+
		"but per-process metrics (gpu.process.memory.usage) are omitted until the topology is fully readable",
		strings.Join(ids, ", "))
}

// probeCard returns the device behind a DRM card, or nil when the card is not
// an AMD GPU bound to amdgpu.
func probeCard(linkPath string) (*Device, error) {
	devicePath, err := filepath.EvalSymlinks(linkPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve device link: %w", err)
	}

	pciBusID := strings.ToLower(filepath.Base(devicePath))
	if !pciAddressRegex.MatchString(pciBusID) {
		// Platform devices, such as amdgpu_xcp_* partition nodes.
		return nil, nil
	}

	vendorID, err := readHexID(filepath.Join(devicePath, "vendor"))
	if err != nil {
		if isUnsupported(err) {
			return nil, nil
		}
		return nil, err
	}
	if vendorID != amdPCIVendorID {
		return nil, nil
	}
	bound, err := boundToAmdgpu(devicePath)
	if err != nil {
		return nil, err
	}
	if !bound {
		return nil, nil
	}

	deviceID, err := readHexID(filepath.Join(devicePath, "device"))
	if err != nil {
		return nil, fmt.Errorf("unreadable PCI device ID for %s: %w", pciBusID, err)
	}

	dev := &Device{
		PCIBusID:   pciBusID,
		DeviceID:   deviceID,
		devicePath: devicePath,
	}
	if total, err := readUint(filepath.Join(devicePath, "mem_info_vram_total")); err == nil {
		dev.MemoryTotal = total
	}

	uniqueID := strings.ToLower(readTrimmed(filepath.Join(devicePath, "unique_id")))
	serial, err := strconv.ParseUint(uniqueID, 16, 64)
	if err == nil && serial != 0 && uniqueIDRegex.MatchString(uniqueID) {
		dev.UUID = Vendor + "-" + uniqueID
	} else {
		dev.UUID = pciUUID(pciBusID)
	}

	dev.Name = cleanName(readTrimmed(filepath.Join(devicePath, "product_name")))
	if dev.Name == "" {
		dev.Name = cleanName(deviceName(deviceID))
	}
	return dev, nil
}

func pciUUID(pciBusID string) string {
	return Vendor + "-" + pciIDReplacer.Replace(pciBusID)
}

func boundToAmdgpu(devicePath string) (bool, error) {
	target, err := os.Readlink(filepath.Join(devicePath, "driver"))
	if err == nil {
		return filepath.Base(target) == amdgpuDriver, nil
	}
	if !isUnsupported(err) {
		return false, err
	}
	content, err := os.ReadFile(filepath.Join(devicePath, "uevent"))
	if err != nil {
		if isUnsupported(err) {
			return false, nil
		}
		return false, err
	}
	for line := range strings.Lines(string(content)) {
		if strings.TrimSpace(line) == "DRIVER="+amdgpuDriver {
			return true, nil
		}
	}
	return false, nil
}

// readTrimmed returns the trimmed content of a sysfs attribute, or "" when
// the attribute is missing or unreadable.
func readTrimmed(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

// readHexID parses a sysfs ID attribute such as "0x1002".
func readHexID(path string) (uint16, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	value := strings.TrimSpace(string(content))
	id, err := strconv.ParseUint(strings.TrimPrefix(value, "0x"), 16, 16)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	return uint16(id), nil
}
