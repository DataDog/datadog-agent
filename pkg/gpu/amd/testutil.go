// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && test

package amd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// FakeSysfs builds a minimal sysfs tree with the layout amdgpu exposes:
// /sys/class/drm/cardN/device is a link to the PCI device directory, whose
// driver link points to the bound driver.
type FakeSysfs struct {
	t    testing.TB
	Root string
}

// NewFakeSysfs creates an empty fake sysfs root in a temporary directory.
func NewFakeSysfs(t testing.TB) *FakeSysfs {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "class", "drm"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "bus", "pci", "devices"), 0o755))
	return &FakeSysfs{t: t, Root: root}
}

// AddPCIDevice creates a PCI device directory with the given attributes and
// binds it to driver (no driver link when driver is empty). It returns the
// device directory.
func (f *FakeSysfs) AddPCIDevice(pciBusID, driver string, attributes map[string]string) string {
	f.t.Helper()
	devDir := filepath.Join(f.Root, "devices", "pci0000:00", pciBusID)
	require.NoError(f.t, os.MkdirAll(devDir, 0o755))
	if driver != "" {
		driverDir := filepath.Join(f.Root, "bus", "pci", "drivers", driver)
		require.NoError(f.t, os.MkdirAll(driverDir, 0o755))
		require.NoError(f.t, os.Symlink(driverDir, filepath.Join(devDir, "driver")))
	}
	f.WriteFiles(devDir, attributes)
	require.NoError(f.t, os.Symlink(devDir, filepath.Join(f.Root, "bus", "pci", "devices", pciBusID)))
	return devDir
}

// AddPlatformDevice creates a platform device directory, as used by amdgpu
// for compute partitions (amdgpu_xcp_N).
func (f *FakeSysfs) AddPlatformDevice(name string) string {
	f.t.Helper()
	devDir := filepath.Join(f.Root, "devices", "platform", name)
	require.NoError(f.t, os.MkdirAll(devDir, 0o755))
	return devDir
}

// AddCard creates /sys/class/drm/<card> with a device link to devDir.
func (f *FakeSysfs) AddCard(card, devDir string) {
	f.t.Helper()
	cardDir := filepath.Join(f.Root, "class", "drm", card)
	require.NoError(f.t, os.MkdirAll(cardDir, 0o755))
	require.NoError(f.t, os.Symlink(devDir, filepath.Join(cardDir, "device")))
}

// AddHwmon creates <devDir>/hwmon/<name> with the given attributes.
func (f *FakeSysfs) AddHwmon(devDir, name string, attributes map[string]string) {
	f.t.Helper()
	f.WriteFiles(filepath.Join(devDir, "hwmon", name), attributes)
}

// WriteFiles writes each attribute as a file under dir.
func (f *FakeSysfs) WriteFiles(dir string, attributes map[string]string) {
	f.t.Helper()
	require.NoError(f.t, os.MkdirAll(dir, 0o755))
	for name, content := range attributes {
		require.NoError(f.t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
}

// SetDriverVersion writes /sys/module/amdgpu/version.
func (f *FakeSysfs) SetDriverVersion(version string) {
	f.t.Helper()
	f.WriteFiles(filepath.Join(f.Root, "module", amdgpuDriver), map[string]string{"version": version + "\n"})
}

// MI300XAttributes returns PCI attributes shaped like an MI300X (device ID
// 0x74a1) with the given unique_id (omitted when empty). Values are synthetic.
func MI300XAttributes(uniqueID string) map[string]string {
	attrs := map[string]string{
		"vendor":              "0x1002\n",
		"device":              "0x74a1\n",
		"gpu_busy_percent":    "37\n",
		"mem_busy_percent":    "12\n",
		"mem_info_vram_total": "206141652992\n",
		"mem_info_vram_used":  "294965248\n",
		"pp_dpm_sclk":         "S: 131Mhz\n1: 1420Mhz *\n2: 2100Mhz\n",
		"pp_dpm_mclk":         "0: 900Mhz\n1: 1300Mhz *\n",
		"current_link_speed":  "32.0 GT/s PCIe\n",
		"max_link_speed":      "32.0 GT/s PCIe\n",
		"current_link_width":  "16\n",
		"max_link_width":      "16\n",
	}
	if uniqueID != "" {
		attrs["unique_id"] = uniqueID + "\n"
	}
	return attrs
}

// JunctionOnlyHwmon returns hwmon attributes of a device without an edge
// sensor that reports instantaneous power only (power1_input). Values are
// synthetic.
func JunctionOnlyHwmon() map[string]string {
	return map[string]string{
		"temp2_label":  "junction\n",
		"temp2_input":  "41000\n",
		"temp3_label":  "mem\n",
		"temp3_input":  "35000\n",
		"power1_input": "142000000\n",
		"power1_cap":   "750000000\n",
	}
}

// AddKFDNode creates /sys/class/kfd/kfd/topology/nodes/<node> for a GPU node
// (gpuID != 0) or a CPU node (gpuID == 0). locationID is bus<<8 | dev<<3 | fn.
func (f *FakeSysfs) AddKFDNode(node int, gpuID uint64, domain, locationID uint32, gfxTargetVersion uint64) {
	f.t.Helper()
	f.WriteFiles(filepath.Join(f.Root, "class", "kfd", "kfd", "topology", "nodes", strconv.Itoa(node)), map[string]string{
		"gpu_id": strconv.FormatUint(gpuID, 10) + "\n",
		"properties": fmt.Sprintf("cpu_cores_count 0\ngfx_target_version %d\nvendor_id 4098\nlocation_id %d\ndomain %d\n",
			gfxTargetVersion, locationID, domain),
	})
}

// SetKFDProperty appends a "name value" line to the properties of a KFD node
// created with AddKFDNode.
func (f *FakeSysfs) SetKFDProperty(node int, name string, value uint64) {
	f.t.Helper()
	path := filepath.Join(f.Root, "class", "kfd", "kfd", "topology", "nodes", strconv.Itoa(node), "properties")
	props, err := os.ReadFile(path)
	require.NoError(f.t, err)
	require.NoError(f.t, os.WriteFile(path, append(props, []byte(fmt.Sprintf("%s %d\n", name, value))...), 0o644))
}

// AddKFDMemBank creates mem_banks/<bank>/properties of a KFD node. heapType 1
// is the CPU-visible video memory, 0 is system memory.
func (f *FakeSysfs) AddKFDMemBank(node, bank int, heapType uint64, memClockMHz, widthBits uint32) {
	f.t.Helper()
	f.WriteFiles(filepath.Join(f.Root, "class", "kfd", "kfd", "topology", "nodes", strconv.Itoa(node), "mem_banks", strconv.Itoa(bank)), map[string]string{
		"properties": fmt.Sprintf("heap_type %d\nsize_in_bytes 1024\nflags 0\nwidth %d\nmem_clk_max %d\n", heapType, widthBits, memClockMHz),
	})
}

// DenyKFDNode makes the gpu_id and properties of a KFD node unreadable with a
// permission error, as the kernel does for a GPU whose device node is not
// allowed in the reader's device cgroup. It skips the test when the file stays
// readable (running as root), since the denial cannot be reproduced there.
func (f *FakeSysfs) DenyKFDNode(node int) {
	f.t.Helper()
	nodeDir := filepath.Join(f.Root, "class", "kfd", "kfd", "topology", "nodes", strconv.Itoa(node))
	for _, name := range []string{"gpu_id", "properties"} {
		require.NoError(f.t, os.Chmod(filepath.Join(nodeDir, name), 0))
	}
	if _, err := os.ReadFile(filepath.Join(nodeDir, "gpu_id")); err == nil {
		f.t.Skip("file permissions are not enforced for this user (root?)")
	}
}

// AddKFDProcess records that pid holds vramBytes on the KFD node gpuID
// (/sys/class/kfd/kfd/proc/<pid>/vram_<gpuID>).
func (f *FakeSysfs) AddKFDProcess(pid int, gpuID, vramBytes uint64) {
	f.t.Helper()
	f.WriteFiles(filepath.Join(f.Root, "class", "kfd", "kfd", "proc", strconv.Itoa(pid)), map[string]string{
		"vram_" + strconv.FormatUint(gpuID, 10): strconv.FormatUint(vramBytes, 10) + "\n",
	})
}

// AddKFDProcessOccupancy records that pid occupies cus compute units on the KFD
// node gpuID (/sys/class/kfd/kfd/proc/<pid>/stats_<gpuID>/cu_occupancy).
func (f *FakeSysfs) AddKFDProcessOccupancy(pid int, gpuID, cus uint64) {
	f.t.Helper()
	f.WriteFiles(filepath.Join(f.Root, "class", "kfd", "kfd", "proc", strconv.Itoa(pid), "stats_"+strconv.FormatUint(gpuID, 10)), map[string]string{
		"cu_occupancy": strconv.FormatUint(cus, 10) + "\n",
		"evicted_ms":   "0\n",
	})
}

// SetKFDRenderMinor adds drm_render_minor to the properties of a KFD node
// created with AddKFDNode.
func (f *FakeSysfs) SetKFDRenderMinor(node, minor int) {
	f.t.Helper()
	path := filepath.Join(f.Root, "class", "kfd", "kfd", "topology", "nodes", strconv.Itoa(node), "properties")
	props, err := os.ReadFile(path)
	require.NoError(f.t, err)
	require.NoError(f.t, os.WriteFile(path, append(props, []byte(fmt.Sprintf("drm_render_minor %d\n", minor))...), 0o644))
}

// AddPartitionRenderNode creates the render node of a compute partition:
// /sys/devices/platform/<xcp>/drm/renderD<minor>.
func (f *FakeSysfs) AddPartitionRenderNode(xcp string, minor int) {
	f.t.Helper()
	require.NoError(f.t, os.MkdirAll(filepath.Join(f.Root, "devices", "platform", xcp, "drm", "renderD"+strconv.Itoa(minor)), 0o755))
}
