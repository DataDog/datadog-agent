// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && test

package amd

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The expected names follow ROCm SMI's rsmi_get_gfx_target_version; the input
// values are the gfx_target_version of the MI300X/MI308X (90402) and MI210
// (90010) KFD captures in ROCm/k8s-device-plugin testdata.
func TestGfxTargetName(t *testing.T) {
	for version, name := range map[uint64]string{
		90402:  "gfx942",
		90010:  "gfx90a",
		90008:  "gfx908",
		110000: "gfx1100",
		120001: "gfx1201",
		0:      "",
	} {
		assert.Equal(t, name, gfxTargetName(version), "version %d", version)
	}
}

func TestDiscoverAttachesKFDTopology(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("00c0ffee00c0ffee")))
	fs.AddCard("card1", fs.AddPCIDevice("0001:0b:00.0", "amdgpu", MI300XAttributes("")))

	fs.AddKFDNode(0, 0, 0, 0, 0) // CPU node
	// Two partitions of 0000:27:00.0 (location 0x2700 and 0x2701).
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)
	fs.AddKFDNode(2, 4102, 0, 0x2701, 90402)
	// Unpartitioned GPU in PCI domain 1.
	fs.AddKFDNode(3, 5100, 1, 0x0b00, 90010)
	// KFD node of a GPU not bound to amdgpu through DRM: ignored.
	fs.AddKFDNode(4, 6100, 0, 0x4100, 90402)

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	require.Len(t, devices, 2)

	assert.Equal(t, "gfx942", devices[0].Architecture)
	assert.Equal(t, []uint64{4101, 4102}, devices[0].kfdGPUIDs)
	assert.Equal(t, uint64(206141652992), devices[0].MemoryTotal)
	assert.Equal(t, "gfx90a", devices[1].Architecture)
	assert.Equal(t, []uint64{5100}, devices[1].kfdGPUIDs)
}

func TestDiscoverWithoutKFD(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("")))

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Empty(t, devices[0].Architecture)
	assert.Empty(t, devices[0].kfdGPUIDs)

	usage, _, err := ReadProcessMemory(fs.Root, devices)
	require.NoError(t, err)
	assert.Empty(t, usage)
}

// A denied KFD node has no readable PCI identity, so process attribution is
// conservatively incomplete for all discovered GPUs. Readable architecture is
// retained and permission denial remains a deployment warning, not a pull error.
func TestDiscoverKFDAccessDenied(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("00c0ffee00c0ffee")))
	fs.AddCard("card1", fs.AddPCIDevice("0000:41:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddKFDNode(0, 0, 0, 0, 0) // the CPU node is always readable
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)
	fs.AddKFDNode(2, 5100, 0, 0x4100, 90402)
	fs.DenyKFDNode(1)

	devices, err := Discover(fs.Root)
	require.NoError(t, err, "a permission-denied KFD node is not a discovery error")
	require.Len(t, devices, 2)

	assert.True(t, devices[0].KFDAccessDenied)
	assert.Empty(t, devices[0].Architecture)
	assert.Empty(t, devices[0].kfdGPUIDs)
	assert.True(t, devices[1].KFDAccessDenied)
	assert.Equal(t, "gfx942", devices[1].Architecture)
}

func TestKFDAccessDeniedWarningEmptyWhenReadable(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	assert.Empty(t, KFDAccessDeniedWarning(devices))

	// No KFD at all (driver without compute, or sysfs not mounted) is not a denial either.
	fs = NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("")))
	devices, err = Discover(fs.Root)
	require.NoError(t, err)
	assert.Empty(t, KFDAccessDeniedWarning(devices))
}
func TestReadProcessMemoryDoesNotReportPartialKFDTopology(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddCard("card1", fs.AddPCIDevice("0000:41:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)
	fs.AddKFDNode(2, 4102, 0, 0x2701, 90402)
	fs.AddKFDNode(3, 5100, 0, 0x4100, 90010)
	fs.AddKFDProcess(100, 4101, 10)
	fs.AddKFDProcess(100, 4102, 20)
	fs.AddKFDProcess(100, 5100, 50)
	fs.AddKFDProcess(200, 4102, 40)
	expected := []ProcessMemory{
		{PID: 100, DeviceUUID: "amd-0000-27-00-0", VRAMBytes: 30},
		{PID: 100, DeviceUUID: "amd-0000-41-00-0", VRAMBytes: 50},
		{PID: 200, DeviceUUID: "amd-0000-27-00-0", VRAMBytes: 40},
	}
	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	usage, complete, err := ReadProcessMemory(fs.Root, devices)
	require.NoError(t, err)
	assert.True(t, complete)
	assert.Equal(t, expected, usage)

	fs.DenyKFDNode(2)
	devices, err = Discover(fs.Root)
	require.NoError(t, err)
	usage, complete, err = ReadProcessMemory(fs.Root, devices)
	require.NoError(t, err)
	assert.False(t, complete)
	assert.Empty(t, usage, "a denied node has unknown ownership; do not publish incomplete physical-GPU sums")
	assert.Equal(t, "gfx942", devices[0].Architecture)
	assert.Equal(t, "gfx90a", devices[1].Architecture)

	for _, name := range []string{"gpu_id", "properties"} {
		require.NoError(t, os.Chmod(filepath.Join(fs.Root, "class/kfd/kfd/topology/nodes/2", name), 0o644))
	}
	devices, err = Discover(fs.Root)
	require.NoError(t, err)
	usage, complete, err = ReadProcessMemory(fs.Root, devices)
	require.NoError(t, err)
	assert.True(t, complete)
	assert.Equal(t, expected, usage)
}

// A node removed during the scan is not a permission denial, but its gpu_id is
// just as unmapped: sums over the surviving partitions would be truncated.
func TestReadProcessMemoryDoesNotReportUnmappedKFDNode(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddCard("card1", fs.AddPCIDevice("0000:41:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)
	fs.AddKFDNode(2, 4102, 0, 0x2701, 90402)
	fs.AddKFDNode(3, 5100, 0, 0x4100, 90010)
	fs.AddKFDProcess(100, 4101, 10)
	fs.AddKFDProcess(100, 4102, 20)
	fs.AddKFDProcess(200, 4102, 40)
	require.NoError(t, os.Remove(filepath.Join(fs.Root, "class/kfd/kfd/topology/nodes/2/properties")))

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	require.Len(t, devices, 2)
	assert.False(t, devices[0].KFDAccessDenied)
	assert.Empty(t, KFDAccessDeniedWarning(devices))

	usage, complete, err := ReadProcessMemory(fs.Root, devices)
	require.NoError(t, err)
	assert.False(t, complete)
	assert.Empty(t, usage)
	assert.Equal(t, "gfx942", devices[0].Architecture)
	assert.Equal(t, "gfx90a", devices[1].Architecture)

	// An invalid PCI location leaves the node unmapped too.
	fs.WriteFiles(filepath.Join(fs.Root, "class/kfd/kfd/topology/nodes/2"), map[string]string{
		"properties": "gfx_target_version 90402\n",
	})
	devices, _ = Discover(fs.Root)
	_, complete, err = ReadProcessMemory(fs.Root, devices)
	require.NoError(t, err)
	assert.False(t, complete)
}

func TestDiscoverKFDClocksAndBusWidth(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("00c0ffee00c0ffee")))
	fs.AddCard("card1", fs.AddPCIDevice("0000:41:00.0", "amdgpu", MI300XAttributes("")))

	// Unpartitioned GPU: one node and one video memory bank. The system-memory
	// bank (heap type 0) is not video memory.
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)
	fs.SetKFDProperty(1, "max_engine_clk_fcompute", 2100)
	fs.AddKFDMemBank(1, 0, 1, 1300, 4096)
	fs.AddKFDMemBank(1, 1, 0, 0, 64)

	// Partitioned GPU: the clocks are per ASIC, but the width of one partition's
	// bank does not tell the width of the whole bus.
	fs.AddKFDNode(2, 5100, 0, 0x4100, 90402)
	fs.AddKFDNode(3, 5101, 0, 0x4101, 90402)
	fs.SetKFDProperty(2, "max_engine_clk_fcompute", 2100)
	fs.SetKFDProperty(3, "max_engine_clk_fcompute", 2200)
	fs.AddKFDMemBank(2, 0, 1, 1300, 1024)
	fs.AddKFDMemBank(3, 0, 1, 1300, 1024)

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	require.Len(t, devices, 2)

	assert.Equal(t, uint32(2100), devices[0].MaxEngineClockMHz)
	assert.Equal(t, uint32(1300), devices[0].MaxMemoryClockMHz)
	assert.Equal(t, uint32(4096), devices[0].MemoryBusWidthBits)

	assert.Equal(t, uint32(2200), devices[1].MaxEngineClockMHz, "the highest clock of the partitions")
	assert.Equal(t, uint32(1300), devices[1].MaxMemoryClockMHz)
	assert.Zero(t, devices[1].MemoryBusWidthBits, "unknown for a partitioned GPU")
}

func TestDiscoverKFDWithoutMemBanksOrClocks(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, "gfx942", devices[0].Architecture)
	assert.Zero(t, devices[0].MaxEngineClockMHz)
	assert.Zero(t, devices[0].MaxMemoryClockMHz)
	assert.Zero(t, devices[0].MemoryBusWidthBits)
}

func TestUint32OrZero(t *testing.T) {
	assert.Equal(t, uint32(2200), uint32OrZero(2200))
	assert.Equal(t, uint32(math.MaxUint32), uint32OrZero(math.MaxUint32))
	assert.Zero(t, uint32OrZero(math.MaxUint32+1), "a value that does not fit is unknown")
}

func TestReadProcessMemory(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("00c0ffee00c0ffee")))
	fs.AddCard("card1", fs.AddPCIDevice("0000:41:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)
	fs.AddKFDNode(2, 4102, 0, 0x2701, 90402)
	fs.AddKFDNode(3, 5100, 0, 0x4100, 90402)

	fs.AddKFDProcess(100, 4101, 1<<30)
	fs.AddKFDProcess(100, 4102, 1<<20) // same device, other partition: summed
	fs.AddKFDProcess(100, 5100, 4096)
	fs.AddKFDProcess(200, 5100, 0)    // an unused device entry does not establish an association
	fs.AddKFDProcess(300, 9999, 1234) // unknown gpu_id: ignored
	fs.WriteFiles(fs.Root+"/class/kfd/kfd/proc/400", map[string]string{"pasid": "5\n", "vram_x": "1\n"})

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	usage, _, err := ReadProcessMemory(fs.Root, devices)
	require.NoError(t, err)

	assert.Equal(t, []ProcessMemory{
		{PID: 100, DeviceUUID: "amd-0000-41-00-0", VRAMBytes: 4096},
		{PID: 100, DeviceUUID: "amd-00c0ffee00c0ffee", VRAMBytes: 1<<30 + 1<<20},
	}, usage)
}

func TestReadProcessMemoryReportsParseErrors(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)
	fs.AddKFDProcess(100, 4101, 42)
	fs.WriteFiles(fs.Root+"/class/kfd/kfd/proc/200", map[string]string{"vram_4101": "lots\n"})

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	usage, _, err := ReadProcessMemory(fs.Root, devices)
	require.ErrorContains(t, err, "vram_4101")
	assert.Equal(t, []ProcessMemory{{PID: 100, DeviceUUID: "amd-0000-27-00-0", VRAMBytes: 42}}, usage)
}

// Maps the captured KFD location IDs of real hosts (topology_test.go) back to
// the DRM PCI devices: every partition must land on its physical GPU.
func TestCapturedTopologiesKFDMapping(t *testing.T) {
	for name, gpus := range capturedTopologies {
		t.Run(name, func(t *testing.T) {
			fs := buildCapturedSysfs(t, gpus)
			node, gpuID := 0, uint64(1000)
			for _, gpu := range gpus {
				for p := range gpu.partitions {
					node++
					gpuID++
					fs.AddKFDNode(node, gpuID, gpu.domain, gpu.locationID+uint32(p), 90402)
				}
			}

			devices, err := Discover(fs.Root)
			require.NoError(t, err)
			require.Len(t, devices, len(gpus))
			byPCI := map[string]*Device{}
			for _, dev := range devices {
				byPCI[dev.PCIBusID] = dev
			}
			for _, gpu := range gpus {
				dev := byPCI[gpu.pciBusID()]
				require.NotNil(t, dev, gpu.pciBusID())
				assert.Len(t, dev.kfdGPUIDs, gpu.partitions, "partitions of %s", gpu.pciBusID())
				assert.Equal(t, "gfx942", dev.Architecture)
			}
		})
	}
}

func TestDiscoverRejectsIncompleteKFDAddresses(t *testing.T) {
	for name, properties := range map[string]string{
		"missing location": "domain 0\n",
		"missing domain":   "location_id 0\n",
		"invalid location": "domain 0\nlocation_id invalid\n",
		"invalid domain":   "domain invalid\nlocation_id 0\n",
		"wide location":    "domain 0\nlocation_id 65536\n",
		"wide domain":      "domain 4294967296\nlocation_id 0\n",
	} {
		t.Run(name, func(t *testing.T) {
			fs := NewFakeSysfs(t)
			fs.AddCard("card0", fs.AddPCIDevice("0000:00:00.0", "amdgpu", MI300XAttributes("")))
			fs.AddCard("card1", fs.AddPCIDevice("12345:ab:1e.0", "amdgpu", MI300XAttributes("")))
			fs.WriteFiles(filepath.Join(fs.Root, "class/kfd/kfd/topology/nodes/1"), map[string]string{
				"gpu_id":     "4101\n",
				"properties": properties,
			})
			fs.AddKFDNode(2, 5100, 0x12345, 0xabf7, 90402)

			devices, err := Discover(fs.Root)
			require.ErrorContains(t, err, "PCI location_id/domain")
			require.Len(t, devices, 2)
			assert.Empty(t, devices[0].kfdGPUIDs, "must not attribute malformed nodes to 0000:00:00.0")
			assert.Equal(t, []uint64{5100}, devices[1].kfdGPUIDs)
			assert.Equal(t, "gfx942", devices[1].Architecture)
		})
	}
}

func TestDiscoverUsesAvailablePartitionArchitecture(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddKFDNode(0, 0, 0, 0, 0)
	fs.AddKFDNode(1, 4101, 0, 0x2700, 0)
	fs.AddKFDNode(2, 4102, 0, 0x2701, 90402)
	// A node removed between reading gpu_id and properties is harmless.
	fs.WriteFiles(filepath.Join(fs.Root, "class/kfd/kfd/topology/nodes/3"), map[string]string{"gpu_id": "4103\n"})
	fs.WriteFiles(filepath.Join(fs.Root, "class/kfd/kfd/topology/nodes/not-a-node"), map[string]string{"gpu_id": "invalid\n"})

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, "gfx942", devices[0].Architecture)
	assert.Equal(t, []uint64{4101, 4102}, devices[0].kfdGPUIDs)
}

func TestReadProcessMemoryIgnoresNonPIDsAndMissingAttributes(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)
	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	usage, _, err := ReadProcessMemory(fs.Root, devices)
	require.NoError(t, err)
	assert.Empty(t, usage, "missing KFD proc is optional")

	procDir := filepath.Join(fs.Root, "class/kfd/kfd/proc")
	for _, name := range []string{"0", "-1", "+100", "2147483648", "not-a-pid"} {
		fs.WriteFiles(filepath.Join(procDir, name), map[string]string{"vram_4101": "42\n"})
	}
	fs.WriteFiles(procDir, map[string]string{"999": "not a directory"})
	fs.AddKFDProcess(100, 4101, 42)
	require.NoError(t, os.MkdirAll(filepath.Join(procDir, "200"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(procDir, "gone"), filepath.Join(procDir, "200/vram_4101")))

	usage, _, err = ReadProcessMemory(fs.Root, devices)
	require.NoError(t, err)
	assert.Equal(t, []ProcessMemory{{PID: 100, DeviceUUID: "amd-0000-27-00-0", VRAMBytes: 42}}, usage)
}

func TestReadProcessMemoryAggregatesSecondaryContexts(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)
	fs.AddKFDNode(2, 4102, 0, 0x2701, 90402)
	fs.AddKFDProcess(100, 4101, 10)
	fs.WriteFiles(filepath.Join(fs.Root, "class/kfd/kfd/proc/100/context_1"), map[string]string{"vram_4101": "20\n", "vram_4102": "30\n"})
	fs.WriteFiles(filepath.Join(fs.Root, "class/kfd/kfd/proc/200/context_2"), map[string]string{"vram_4102": "40\n"})
	fs.WriteFiles(filepath.Join(fs.Root, "class/kfd/kfd/proc/100/context_invalid"), map[string]string{"vram_4101": "999\n"})
	fs.WriteFiles(filepath.Join(fs.Root, "class/kfd/kfd/proc/100/context_1/context_3"), map[string]string{"vram_4101": "999\n"})
	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	usage, _, err := ReadProcessMemory(fs.Root, devices)
	require.NoError(t, err)
	assert.Equal(t, []ProcessMemory{
		{PID: 100, DeviceUUID: "amd-0000-27-00-0", VRAMBytes: 60},
		{PID: 200, DeviceUUID: "amd-0000-27-00-0", VRAMBytes: 40},
	}, usage)

	fs.WriteFiles(filepath.Join(fs.Root, "class/kfd/kfd/proc/100/context_2"), map[string]string{"vram_4101": "invalid\n"})
	usage, _, err = ReadProcessMemory(fs.Root, devices)
	require.ErrorContains(t, err, "context_2")
	assert.Equal(t, uint64(60), usage[0].VRAMBytes, "a bad context must not discard readable allocations")
	require.NoError(t, os.RemoveAll(filepath.Join(fs.Root, "class/kfd/kfd/proc/100")))
	usage, _, err = ReadProcessMemory(fs.Root, devices)
	require.NoError(t, err)
	assert.Equal(t, []ProcessMemory{{PID: 200, DeviceUUID: "amd-0000-27-00-0", VRAMBytes: 40}}, usage)
}

func TestReadProcessMemoryRejectsAggregateOverflow(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)
	fs.AddKFDProcess(100, 4101, 1)
	fs.WriteFiles(filepath.Join(fs.Root, "class/kfd/kfd/proc/100/context_1"), map[string]string{"vram_4101": "18446744073709551615\n"})
	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	usage, _, err := ReadProcessMemory(fs.Root, devices)
	require.ErrorContains(t, err, "overflows")
	assert.Equal(t, []ProcessMemory{{PID: 100, DeviceUUID: "amd-0000-27-00-0", VRAMBytes: math.MaxUint64}}, usage)
}

func TestDiscoverNonzeroPCIFunctionAndSiblingFunctions(t *testing.T) {
	fs := NewFakeSysfs(t)
	first := fs.AddPCIDevice("0000:27:00.1", "amdgpu", MI300XAttributes("1111"))
	fs.AddCard("card0", first)
	fs.AddCard("renderD129", first)
	fs.AddKFDNode(1, 4101, 0, 0x2701, 90402)
	fs.SetKFDRenderMinor(1, 129)
	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	assert.Equal(t, []uint64{4101}, devices[0].kfdGPUIDs)

	second := fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("2222"))
	fs.AddCard("card1", second)
	fs.AddCard("renderD128", second)
	fs.AddKFDNode(2, 4102, 0, 0x2700, 90402)
	fs.SetKFDRenderMinor(2, 128)
	// Partition bits collide with the sibling's real PCI function. DRM resolves it.
	fs.AddKFDNode(3, 4103, 0, 0x2701, 90402)
	fs.SetKFDRenderMinor(3, 128)
	devices, err = Discover(fs.Root)
	require.NoError(t, err)
	assert.Equal(t, []uint64{4102, 4103}, devices[0].kfdGPUIDs)
	assert.Equal(t, []uint64{4101}, devices[1].kfdGPUIDs)

	require.NoError(t, os.Remove(filepath.Join(fs.Root, "class/drm/renderD128/device")))
	devices, err = Discover(fs.Root)
	require.ErrorContains(t, err, "disambiguate")
	assert.Empty(t, devices[0].kfdGPUIDs, "ambiguous nodes must not be attributed to the sibling GPU")
	assert.Equal(t, []uint64{4101}, devices[1].kfdGPUIDs)
}

func TestDiscoverIncompleteMemoryBanksDoesNotPublishBusWidth(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)
	fs.AddKFDMemBank(1, 0, 1, 1300, 4096)
	fs.WriteFiles(filepath.Join(fs.Root, "class/kfd/kfd/topology/nodes/1/mem_banks/1"), map[string]string{"properties": "heap_type invalid\n"})
	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	assert.Zero(t, devices[0].MemoryBusWidthBits)
	assert.Equal(t, uint32(1300), devices[0].MaxMemoryClockMHz)
}

func FuzzReadProcessMemory(f *testing.F) {
	for _, input := range []string{"0", "42", "9223372036854775808", "18446744073709551615", "-1", "invalid", ""} {
		f.Add(input)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 4096 {
			t.Skip("sysfs integer attributes are small")
		}
		fs := NewFakeSysfs(t)
		fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("")))
		fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)
		fs.WriteFiles(filepath.Join(fs.Root, "class/kfd/kfd/proc/100"), map[string]string{"vram_4101": input})
		fs.WriteFiles(filepath.Join(fs.Root, "class/kfd/kfd/proc/100/context_1"), map[string]string{"vram_4101": input})
		devices, err := Discover(fs.Root)
		require.NoError(t, err)
		usage, _, err := ReadProcessMemory(fs.Root, devices)
		value, parseErr := strconv.ParseUint(strings.TrimSpace(input), 10, 64)
		switch {
		case parseErr != nil:
			require.Error(t, err)
			assert.Empty(t, usage)
		case value == 0:
			require.NoError(t, err)
			assert.Empty(t, usage)
		case value > math.MaxUint64/2:
			require.ErrorContains(t, err, "overflows")
			assert.Equal(t, []ProcessMemory{{PID: 100, DeviceUUID: "amd-0000-27-00-0", VRAMBytes: value}}, usage)
		default:
			require.NoError(t, err)
			assert.Equal(t, []ProcessMemory{{PID: 100, DeviceUUID: "amd-0000-27-00-0", VRAMBytes: 2 * value}}, usage)
		}
	})
}
