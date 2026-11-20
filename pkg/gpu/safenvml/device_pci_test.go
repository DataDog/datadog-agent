// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// writePCIDevice adds a device at path (e.g. "pci0000:89/0000:89:00.0/0000:8d:00.0")
// to a fake sysfs mount, linked from bus/pci/devices like the kernel does.
func writePCIDevice(t *testing.T, sysRoot, path, numaNode string) {
	t.Helper()
	dir := filepath.Join(sysRoot, "devices", path)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "numa_node"), []byte(numaNode+"\n"), 0o644))
	busDir := filepath.Join(sysRoot, "bus", "pci", "devices")
	require.NoError(t, os.MkdirAll(busDir, 0o755))
	require.NoError(t, os.Symlink(filepath.Join("..", "..", "..", "devices", path), filepath.Join(busDir, filepath.Base(path))))
}

// hgxB200Device mocks the NVML view of GPU0 on an HGX B200 (driver 580.95.05).
func hgxB200Device(d *testutil.MockDevice) {
	d.GetPciInfoFunc = func() (nvml.PciInfo, nvml.Return) {
		return nvml.PciInfo{Domain: 0, Bus: 0x8d, Device: 0, PciDeviceId: 0x290110de, PciSubSystemId: 0x199910de}, nvml.SUCCESS
	}
	d.GetBoardPartNumberFunc = func() (string, nvml.Return) { return " 692-2G525-0220-000 ", nvml.SUCCESS }
	d.GetBAR1MemoryInfoFunc = func() (nvml.BAR1Memory, nvml.Return) {
		return nvml.BAR1Memory{Bar1Total: 256 << 30, Bar1Used: 1 << 20}, nvml.SUCCESS
	}
	d.GetPlatformInfoFunc = func() (nvml.PlatformInfo, nvml.Return) {
		return nvml.PlatformInfo{SlotNumber: 255, TrayIndex: 255, HostId: 1, ModuleId: 7}, nvml.SUCCESS
	}
	d.GetMaxPcieLinkGenerationFunc = func() (int, nvml.Return) { return 5, nvml.SUCCESS }
	d.GetGpuMaxPcieLinkGenerationFunc = func() (int, nvml.Return) { return 6, nvml.SUCCESS }
	d.GetMaxPcieLinkWidthFunc = func() (int, nvml.Return) { return 16, nvml.SUCCESS }
	// Current values differ from the maximum and must not be cached.
	d.GetCurrPcieLinkGenerationFunc = func() (int, nvml.Return) { panic("current PCIe generation is a metric") }
	d.GetCurrPcieLinkWidthFunc = func() (int, nvml.Return) { panic("current PCIe width is a metric") }
}

func TestDeviceCachesTopologyFromSysfs(t *testing.T) {
	mockNvml := testutil.NewMockNVML(testutil.WithDeviceOptions(0, testutil.WithCustomHook(hgxB200Device)))
	WithMockNVML(t, mockNvml)
	sysRoot := t.TempDir()
	writePCIDevice(t, sysRoot, "pci0000:89/0000:89:00.0/0000:8a:00.0/0000:8b:01.0/0000:8d:00.0", "0")
	WithMockPCISysfs(t, t.TempDir(), sysRoot) // the first mount lacks the device, like a partial host mount

	device, err := NewPhysicalDevice(mockNvml.Device(0))
	require.NoError(t, err)
	require.Equal(t, "0000:8d:00.0", device.PCIBusID)
	require.Equal(t, uint32(0x290110de), device.PCIDeviceID)
	require.Equal(t, uint32(0x199910de), device.PCISubsystemID)
	require.Equal(t, "692-2G525-0220-000", device.BoardPartNumber)
	require.Equal(t, uint64(256<<30), device.BAR1TotalBytes)
	require.Equal(t, uint8(1), device.HostID)
	require.Equal(t, uint8(7), device.ModuleID)
	require.NotNil(t, device.NUMANode)
	require.Equal(t, 0, *device.NUMANode, "node 0 is valid")
	require.Equal(t, []string{"0000:8b:01.0", "0000:8a:00.0", "0000:89:00.0"}, device.PCIePath)
	require.Equal(t, HostLinkInfo{PCIeMaxGeneration: 5, PCIeGPUMaxGeneration: 6, PCIeMaxWidth: 16}, device.HostLink,
		"the C2C mode symbol is missing, so the host-link protocol stays unknown")
}

func TestDeviceIgnoresNVMLNUMANode(t *testing.T) {
	// NVML may report the GPU's own memory node on platforms where GPUs are NUMA
	// nodes; the host node always comes from sysfs.
	mockNvml := testutil.NewMockNVML(testutil.WithDeviceOptions(0, testutil.WithCustomHook(func(d *testutil.MockDevice) {
		d.GetNumaNodeIdFunc = func() (int, nvml.Return) { panic("the NUMA node must come from sysfs") }
	})))
	WithMockNVML(t, mockNvml)
	sysRoot := t.TempDir()
	writePCIDevice(t, sysRoot, "pci0000:00/0000:00:1e.0", "0")
	WithMockPCISysfs(t, sysRoot)

	device, err := NewPhysicalDevice(mockNvml.Device(0))
	require.NoError(t, err)
	require.Equal(t, 0, *device.NUMANode)
	require.Equal(t, []string{}, device.PCIePath, "a GPU on the root complex has an empty path")
}

func TestDeviceTopologyUnknownValues(t *testing.T) {
	mockNvml := testutil.NewMockNVML(testutil.WithDeviceOptions(0, testutil.WithCustomHook(func(d *testutil.MockDevice) {
		d.GetBAR1MemoryInfoFunc = func() (nvml.BAR1Memory, nvml.Return) { return nvml.BAR1Memory{}, nvml.ERROR_NOT_SUPPORTED }
		d.GetMaxPcieLinkGenerationFunc = func() (int, nvml.Return) { return 0, nvml.ERROR_NOT_SUPPORTED }
		d.GetGpuMaxPcieLinkGenerationFunc = func() (int, nvml.Return) { return 0, nvml.ERROR_NOT_SUPPORTED }
		d.GetMaxPcieLinkWidthFunc = func() (int, nvml.Return) { return 0, nvml.ERROR_UNKNOWN }
	})))
	WithMockNVML(t, mockNvml)
	sysRoot := t.TempDir()
	writePCIDevice(t, sysRoot, "pci0000:00/0000:00:1e.0", "-1")
	WithMockPCISysfs(t, sysRoot)

	device, err := NewPhysicalDevice(mockNvml.Device(0))
	require.NoError(t, err)
	require.Nil(t, device.NUMANode, "the kernel reports -1 when it does not know the node")
	require.Empty(t, device.BoardPartNumber)
	require.Zero(t, device.BAR1TotalBytes)
	require.Equal(t, HostLinkInfo{}, device.HostLink)
}

func TestDeviceTopologyWithoutPCIInfo(t *testing.T) {
	mockNvml := testutil.NewMockNVML(testutil.WithDeviceOptions(0, testutil.WithCustomHook(func(d *testutil.MockDevice) {
		d.GetPciInfoFunc = func() (nvml.PciInfo, nvml.Return) { return nvml.PciInfo{}, nvml.ERROR_UNKNOWN }
	})))
	WithMockNVML(t, mockNvml)

	device, err := NewPhysicalDevice(mockNvml.Device(0))
	require.NoError(t, err)
	require.Empty(t, device.PCIBusID)
	require.Zero(t, device.PCIDeviceID)
	require.Nil(t, device.NUMANode)
	require.Nil(t, device.PCIePath)
}
