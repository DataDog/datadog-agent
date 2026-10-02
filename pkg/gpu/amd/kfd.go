// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package amd

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// The KFD (the amdgpu compute interface) describes GPU nodes in
// /sys/class/kfd/kfd/topology/nodes/<N>/{gpu_id,properties} and the processes
// holding GPU memory in /sys/class/kfd/kfd/proc/<pid>/vram_<gpu_id>, where
// <pid> is the PID in the host PID namespace. These are the paths ROCm SMI
// reads (rocm_smi/src/rocm_smi_kfd.cc in ROCm/rocm-systems projects/amdsmi).
// Each compute partition of an ASIC is a separate KFD node with its own
// gpu_id; partitions of one ASIC share the PCI domain and bus/device and use
// the function bits of location_id.

// kfdGPU is the KFD view of one physical GPU.
type kfdGPU struct {
	gfxTarget    string   // e.g. gfx942, empty if unknown
	gpuIDs       []uint64 // one per KFD node (partition) of the GPU
	renderMinors []int    // DRM render node minors (renderD<minor>) of the KFD nodes

	nodes             int    // KFD nodes (partitions) of the GPU
	computeUnits      uint32 // compute units over all nodes (simd_count / simd_per_cu)
	maxEngineClockMHz uint32 // max_engine_clk_fcompute, the highest over the nodes
	maxMemoryClockMHz uint32 // mem_clk_max of the video memory banks
	vramBanks         int    // video memory banks over all nodes
	banksComplete     bool   // every memory-bank entry was readable
	busWidthBits      uint32 // width of the video memory bank, meaningful for one node with one bank
}

// readKFDTopology returns KFD GPU nodes keyed by their physical PCI address.
// Function bits can encode partitions, so the address comes from DRM rather
// than assuming every physical function is zero. Missing KFD is not an error.
//
// denied is the number of GPU nodes that could not be read because of a
// permission error. That is not reported as an error: inside a container the
// kernel (kfd_devcgroup_check_permission) refuses to show a GPU's node unless
// the container's device cgroup allows the GPU's render node, which is a
// deployment property rather than a failure to retry or report on every call.
//
// complete is false when any GPU node could not be mapped to a physical GPU,
// whatever the cause (denied, unreadable, removed during the scan, or with an
// invalid PCI location): the gpu_id of such a node may belong to any GPU, so
// process sums over the mapped gpu_id would be truncated.
func readKFDTopology(sysRoot string, pciDevices map[string]struct{}) (gpus map[string]*kfdGPU, denied int, complete bool, err error) {
	nodesDir := filepath.Join(sysRoot, "class", "kfd", "kfd", "topology", "nodes")
	entries, err := os.ReadDir(nodesDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, 0, true, nil
		}
		return nil, 0, false, fmt.Errorf("list %s: %w", nodesDir, err)
	}

	// Most slots have one physical GPU. Multiple PCI functions require the
	// render-node link to disambiguate function bits from partition IDs.
	pciSlots := make(map[string]string, len(pciDevices))
	for pciBusID := range pciDevices {
		slot := pciBusID[:len(pciBusID)-1]
		if _, exists := pciSlots[slot]; exists {
			pciSlots[slot] = ""
		} else {
			pciSlots[slot] = pciBusID
		}
	}
	gpus = make(map[string]*kfdGPU)
	var errs []error
	topologyComplete := true
	for _, entry := range entries {
		if _, err := strconv.ParseUint(entry.Name(), 10, 32); err != nil || !entry.IsDir() {
			continue
		}
		nodeDir := filepath.Join(nodesDir, entry.Name())
		gpuID, err := readUint(filepath.Join(nodeDir, "gpu_id"))
		if err != nil {
			topologyComplete = false
			switch {
			case errors.Is(err, fs.ErrPermission):
				denied++
			case !isUnsupported(err):
				errs = append(errs, err)
			}
			continue
		}
		if gpuID == 0 { // CPU node
			continue
		}
		props, err := readKFDProperties(filepath.Join(nodeDir, "properties"))
		if err != nil {
			topologyComplete = false
			switch {
			case errors.Is(err, fs.ErrPermission):
				denied++
			case !errors.Is(err, fs.ErrNotExist):
				errs = append(errs, err)
			}
			continue
		}
		location, hasLocation := props["location_id"]
		domain, hasDomain := props["domain"]
		if !hasLocation || !hasDomain || location > 0xffff || domain > 0xffffffff {
			topologyComplete = false
			errs = append(errs, fmt.Errorf("%s: missing or invalid PCI location_id/domain", nodeDir))
			continue
		}
		slot := fmt.Sprintf("%04x:%02x:%02x.", domain, (location>>8)&0xff, (location>>3)&0x1f)
		pciBusID, known := pciSlots[slot]
		if !known {
			continue
		}
		if minor, hasMinor := props["drm_render_minor"]; hasMinor && minor > 0 && minor <= 0xfffff {
			path, err := filepath.EvalSymlinks(filepath.Join(sysRoot, "class", "drm", fmt.Sprintf("renderD%d", minor), "device"))
			address := strings.ToLower(filepath.Base(path))
			if err == nil && pciAddressRegex.MatchString(address) {
				if _, known := pciDevices[address]; !known {
					continue
				}
				if !strings.HasPrefix(address, slot) {
					topologyComplete = false
					errs = append(errs, fmt.Errorf("%s: DRM PCI address disagrees with KFD location", nodeDir))
					continue
				}
				pciBusID = address
			}
		}
		if pciBusID == "" {
			topologyComplete = false
			errs = append(errs, fmt.Errorf("%s: cannot disambiguate physical PCI function from KFD partition", nodeDir))
			continue
		}

		gpu, ok := gpus[pciBusID]
		if !ok {
			gpu = &kfdGPU{banksComplete: true}
			gpus[pciBusID] = gpu
		}
		if gpu.gfxTarget == "" {
			gpu.gfxTarget = gfxTargetName(props["gfx_target_version"])
		}
		gpu.gpuIDs = append(gpu.gpuIDs, gpuID)
		if minor, ok := props["drm_render_minor"]; ok && minor > 0 && minor <= 0xfffff {
			gpu.renderMinors = append(gpu.renderMinors, int(minor))
		}
		gpu.nodes++
		gpu.maxEngineClockMHz = max(gpu.maxEngineClockMHz, uint32OrZero(props["max_engine_clk_fcompute"]))
		if simdPerCU := props["simd_per_cu"]; simdPerCU > 0 {
			gpu.computeUnits += uint32OrZero(props["simd_count"] / simdPerCU)
		}
		banks, complete, err := readKFDVRAMBanks(nodeDir)
		gpu.banksComplete = gpu.banksComplete && complete
		if err != nil {
			errs = append(errs, err)
		}
		for _, bank := range banks {
			// The kernel fills every bank of a node with the same mem_clk_max.
			gpu.maxMemoryClockMHz = max(gpu.maxMemoryClockMHz, bank.memClockMHz)
			gpu.vramBanks++
			gpu.busWidthBits = bank.widthBits
		}
	}
	for _, gpu := range gpus {
		slices.Sort(gpu.gpuIDs)
		gpu.gpuIDs = slices.Compact(gpu.gpuIDs)
		slices.Sort(gpu.renderMinors)
		gpu.renderMinors = slices.Compact(gpu.renderMinors)
		// The bus width of a partitioned GPU, or of one with several video
		// memory banks, cannot be told from the width of a single bank: only
		// the unpartitioned layout (validated on an MI350X) is reported.
		if !topologyComplete || len(errs) > 0 || !gpu.banksComplete || gpu.nodes != 1 || gpu.vramBanks != 1 {
			gpu.busWidthBits = 0
		}
	}
	return gpus, denied, topologyComplete, errors.Join(errs...)
}

// readKFDProperties parses a KFD node properties file ("name value" lines).
func readKFDProperties(path string) (map[string]uint64, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	props := make(map[string]uint64)
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue // some properties are not integers; none of them are used
		}
		props[fields[0]] = value
	}
	return props, scanner.Err()
}

// HSA heap types (libhsakmt hsakmttypes.h) of the video memory banks of a node:
// the CPU-visible and the CPU-invisible part of the GPU's local memory.
const (
	kfdHeapFrameBufferPublic  = 1
	kfdHeapFrameBufferPrivate = 2
)

// kfdVRAMBank is a video memory bank of a KFD node.
type kfdVRAMBank struct {
	memClockMHz uint32
	widthBits   uint32
}

// readKFDVRAMBanks returns readable video memory banks and whether every bank
// could be classified. A partial view can supply clocks, but not bus width.
func readKFDVRAMBanks(nodeDir string) ([]kfdVRAMBank, bool, error) {
	banksDir := filepath.Join(nodeDir, "mem_banks")
	entries, err := os.ReadDir(banksDir)
	if err != nil {
		if isUnsupported(err) || errors.Is(err, fs.ErrPermission) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var banks []kfdVRAMBank
	var errs []error
	complete := true
	for _, entry := range entries {
		if _, err := strconv.ParseUint(entry.Name(), 10, 32); err != nil || !entry.IsDir() {
			continue
		}
		props, err := readKFDProperties(filepath.Join(banksDir, entry.Name(), "properties"))
		if err != nil {
			complete = false
			if !isUnsupported(err) && !errors.Is(err, fs.ErrPermission) {
				errs = append(errs, err)
			}
			continue
		}
		if _, known := props["heap_type"]; !known {
			complete = false
			continue
		}
		if heap := props["heap_type"]; heap != kfdHeapFrameBufferPublic && heap != kfdHeapFrameBufferPrivate {
			continue
		}
		banks = append(banks, kfdVRAMBank{memClockMHz: uint32OrZero(props["mem_clk_max"]), widthBits: uint32OrZero(props["width"])})
	}
	return banks, complete, errors.Join(errs...)
}

// uint32OrZero converts a KFD property to uint32, treating a value that does
// not fit as unknown (0).
func uint32OrZero(v uint64) uint32 {
	if v > math.MaxUint32 {
		return 0
	}
	return uint32(v)
}

// gfxTargetName converts a KFD gfx_target_version (major*10000 + minor*100 +
// stepping) to the LLVM target name, e.g. 90402 -> gfx942 and 90010 ->
// gfx90a, with the same formula as ROCm SMI (rsmi_get_gfx_target_version).
func gfxTargetName(version uint64) string {
	if version == 0 {
		return ""
	}
	major := version / 10000
	minor := version % 10000 / 100
	stepping := version % 100
	return "gfx" + strconv.FormatUint(major*10+minor, 10) + strconv.FormatUint(stepping, 16)
}

// ProcessMemory is the GPU memory a process holds on one device.
type ProcessMemory struct {
	PID        int
	DeviceUUID string
	VRAMBytes  uint64
}

type processMemoryKey struct {
	pid  int
	uuid string
}

// ReadProcessMemory returns the VRAM held by each process on each of the given
// devices, from /sys/class/kfd/kfd/proc. Processes are reported with their PID
// in the host PID namespace. Memory across partitions and secondary
// context_<id> directories is summed. Missing attributes tolerate exits.
// Only nonzero VRAM allocations establish an association: KFD creates vram_*
// attributes for process-device entries even when that GPU is unused (see
// kfd_procfs_add_sysfs_files in drivers/gpu/drm/amd/amdkfd/kfd_process.c).
// Processes using only system memory are not identified by this VRAM reader.
// complete distinguishes an empty snapshot from unavailable attribution. A KFD
// node that could not be mapped (denied, unreadable or removed during discovery)
// has unknown ownership, so no physical-GPU sums are returned until the topology
// is fully mapped. Available results from other read errors are
// returned with complete=false and an error.
func ReadProcessMemory(sysRoot string, devices []*Device) ([]ProcessMemory, bool, error) {
	var gpuIDToUUID map[uint64]string
	for _, dev := range devices {
		if dev.KFDAccessDenied || dev.kfdTopologyIncomplete {
			return nil, false, nil
		}
		if len(dev.kfdGPUIDs) > 0 && gpuIDToUUID == nil {
			gpuIDToUUID = make(map[uint64]string)
		}
		for _, id := range dev.kfdGPUIDs {
			gpuIDToUUID[id] = dev.UUID
		}
	}
	if len(gpuIDToUUID) == 0 {
		return nil, true, nil
	}

	procDir := filepath.Join(sysRoot, "class", "kfd", "kfd", "proc")
	entries, err := os.ReadDir(procDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("list %s: %w", procDir, err)
	}

	usage := make(map[processMemoryKey]uint64)
	var errs []error
	for _, entry := range entries {
		pid, err := strconv.ParseUint(entry.Name(), 10, 31)
		if err != nil || pid == 0 || !entry.IsDir() {
			continue
		}
		if err := readKFDProcessMemory(filepath.Join(procDir, entry.Name()), int(pid), gpuIDToUUID, usage, true); err != nil {
			errs = append(errs, err)
		}
	}

	result := make([]ProcessMemory, 0, len(usage))
	for k, vram := range usage {
		result = append(result, ProcessMemory{PID: k.pid, DeviceUUID: k.uuid, VRAMBytes: vram})
	}
	slices.SortFunc(result, func(a, b ProcessMemory) int {
		if a.PID != b.PID {
			return a.PID - b.PID
		}
		return strings.Compare(a.DeviceUUID, b.DeviceUUID)
	})
	err = errors.Join(errs...)
	return result, err == nil, err
}

func readKFDProcessMemory(dir string, pid int, gpuIDToUUID map[uint64]string, usage map[processMemoryKey]uint64, includeContexts bool) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // process or context exited
		}
		return err
	}
	var errs []error
	for _, file := range files {
		if file.IsDir() {
			if !includeContexts {
				continue
			}
			id, context := strings.CutPrefix(file.Name(), "context_")
			if !context {
				continue
			}
			if _, err := strconv.ParseUint(id, 10, 32); err == nil {
				// Secondary contexts are direct children of the primary PID;
				// do not recurse into arbitrary or nested directories.
				if err := readKFDProcessMemory(filepath.Join(dir, file.Name()), pid, gpuIDToUUID, usage, false); err != nil {
					errs = append(errs, err)
				}
			}
			continue
		}
		id, vramAttribute := strings.CutPrefix(file.Name(), "vram_")
		if !vramAttribute {
			continue
		}
		gpuID, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			continue
		}
		uuid, known := gpuIDToUUID[gpuID]
		if !known {
			continue
		}
		vram, err := readUint(filepath.Join(dir, file.Name()))
		if err != nil {
			if !isUnsupported(err) {
				errs = append(errs, err)
			}
			continue
		}
		if vram > 0 {
			key := processMemoryKey{pid, uuid}
			if vram > math.MaxUint64-usage[key] {
				errs = append(errs, fmt.Errorf("%s: aggregate VRAM overflows uint64", dir))
				continue
			}
			usage[key] += vram
		}
	}
	return errors.Join(errs...)
}

// readUint reads a sysfs attribute holding an unsigned decimal integer.
func readUint(path string) (uint64, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(content)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	return value, nil
}
