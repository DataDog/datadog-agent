// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && test

package amd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverWithoutDRMClass(t *testing.T) {
	devices, err := Discover(t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, devices)
}

func TestDiscoverFiltersAndOrdersDevices(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.SetDriverVersion("6.14.14")

	// Listed out of PCI order on purpose: devices are sorted by PCI address.
	second := fs.AddPCIDevice("0000:c1:00.0", "amdgpu", MI300XAttributes("ABCDEF0123456789"))
	first := fs.AddPCIDevice("0000:11:00.0", "amdgpu", MI300XAttributes(""))
	fs.AddCard("card1", second)
	fs.AddCard("card0", first)

	// Same PCI device behind another card node: reported once.
	fs.AddCard("card7", second)
	// Display connector of card0: not a card node.
	fs.AddCard("card0-DP-1", first)
	// Compute partition of an ASIC: platform device, reported through its physical device.
	fs.AddCard("card2", fs.AddPlatformDevice("amdgpu_xcp_1"))
	// NVIDIA GPU.
	fs.AddCard("card3", fs.AddPCIDevice("0000:21:00.0", "nvidia", map[string]string{"vendor": "0x10de\n", "device": "0x2330\n"}))
	// AMD GPU passed through to a VM: not bound to amdgpu.
	fs.AddCard("card4", fs.AddPCIDevice("0000:31:00.0", "vfio-pci", MI300XAttributes("")))
	// AMD GPU with an unknown device ID and an explicit product name.
	named := MI300XAttributes("")
	named["device"] = "0x7551\n"
	named["product_name"] = "AMD Radeon AI PRO R9700\n"
	fs.AddCard("card5", fs.AddPCIDevice("0000:41:00.0", "amdgpu", named))
	// AMD GPU with an unknown device ID and no product name.
	unnamed := MI300XAttributes("")
	unnamed["device"] = "0x7fff\n"
	fs.AddCard("card6", fs.AddPCIDevice("0000:51:00.0", "amdgpu", unnamed))

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	require.Len(t, devices, 4)

	expected := []Device{
		{UUID: "amd-0000-11-00-0", Index: 0, Name: "AMD Instinct MI300X", PCIBusID: "0000:11:00.0", DeviceID: 0x74a1, DriverVersion: "6.14.14", MemoryTotal: 206141652992},
		{UUID: "amd-0000-41-00-0", Index: 1, Name: "AMD Radeon AI PRO R9700", PCIBusID: "0000:41:00.0", DeviceID: 0x7551, DriverVersion: "6.14.14", MemoryTotal: 206141652992},
		{UUID: "amd-0000-51-00-0", Index: 2, Name: "AMD GPU 0x7fff", PCIBusID: "0000:51:00.0", DeviceID: 0x7fff, DriverVersion: "6.14.14", MemoryTotal: 206141652992},
		{UUID: "amd-abcdef0123456789", Index: 3, Name: "AMD Instinct MI300X", PCIBusID: "0000:c1:00.0", DeviceID: 0x74a1, DriverVersion: "6.14.14", MemoryTotal: 206141652992},
	}
	for i, dev := range devices {
		got := *dev
		got.devicePath = ""
		assert.Equal(t, expected[i], got, "device %d", i)
	}
}

func TestDiscoverUsesUeventWithoutDriverLink(t *testing.T) {
	fs := NewFakeSysfs(t)
	attrs := MI300XAttributes("")
	attrs["uevent"] = "PCI_ID=1002:74A1\nDRIVER=amdgpu\n"
	fs.AddCard("card0", fs.AddPCIDevice("0000:11:00.0", "", attrs))

	unbound := MI300XAttributes("")
	unbound["uevent"] = "PCI_ID=1002:74A1\n"
	fs.AddCard("card1", fs.AddPCIDevice("0000:21:00.0", "", unbound))

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, "0000:11:00.0", devices[0].PCIBusID)
	assert.Empty(t, devices[0].DriverVersion)
}

func TestDiscoverRejectsInvalidUniqueID(t *testing.T) {
	for _, uniqueID := range []string{"N/A", "0", "0000000000000000", "10000000000000000", "-1", "0x1234"} {
		t.Run(uniqueID, func(t *testing.T) {
			fs := NewFakeSysfs(t)
			fs.AddCard("card0", fs.AddPCIDevice("0000:11:00.0", "amdgpu", MI300XAttributes(uniqueID)))
			fs.AddCard("card1", fs.AddPCIDevice("0000:21:00.0", "amdgpu", MI300XAttributes(uniqueID)))

			devices, err := Discover(fs.Root)
			require.NoError(t, err)
			require.Len(t, devices, 2)
			assert.Equal(t, "amd-0000-11-00-0", devices[0].UUID)
			assert.Equal(t, "amd-0000-21-00-0", devices[1].UUID)
		})
	}
}

func TestDiscoverReportsUnreadableDeviceIDAndKeepsOthers(t *testing.T) {
	fs := NewFakeSysfs(t)
	broken := MI300XAttributes("")
	broken["device"] = "garbage\n"
	fs.AddCard("card0", fs.AddPCIDevice("0000:11:00.0", "amdgpu", broken))
	fs.AddCard("card1", fs.AddPCIDevice("0000:21:00.0", "amdgpu", MI300XAttributes("")))

	devices, err := Discover(fs.Root)
	require.Error(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, "0000:21:00.0", devices[0].PCIBusID)
	assert.Equal(t, 0, devices[0].Index)
}

func TestDiscoverIgnoresDanglingDeviceLink(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", filepath.Join(fs.Root, "devices", "gone"))

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	assert.Empty(t, devices)
}

func TestDiscoverReportsVendorAndDriverReadErrors(t *testing.T) {
	for _, attribute := range []string{"vendor", "uevent"} {
		t.Run(attribute, func(t *testing.T) {
			fs := NewFakeSysfs(t)
			attrs := MI300XAttributes("")
			delete(attrs, attribute)
			broken := fs.AddPCIDevice("0000:11:00.0", "", attrs)
			require.NoError(t, os.Mkdir(filepath.Join(broken, attribute), 0o755))
			fs.AddCard("card0", broken)
			fs.AddCard("card1", fs.AddPCIDevice("0000:21:00.0", "amdgpu", MI300XAttributes("")))

			devices, err := Discover(fs.Root)
			require.ErrorContains(t, err, attribute)
			require.Len(t, devices, 1)
			assert.Equal(t, "0000:21:00.0", devices[0].PCIBusID)
		})
	}
}

func TestDiscoverDriverLinkTakesPrecedenceOverUevent(t *testing.T) {
	fs := NewFakeSysfs(t)
	attrs := MI300XAttributes("")
	attrs["uevent"] = "DRIVER=amdgpu\n"
	fs.AddCard("card0", fs.AddPCIDevice("0000:11:00.0", "vfio-pci", attrs))

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	assert.Empty(t, devices)
}

func TestDiscoverSharedSerialKeepsDistinctPCIIdentities(t *testing.T) {
	fs := NewFakeSysfs(t)
	for i, pci := range []string{"0000:27:00.0", "0000:27:00.1", "0000:27:00.2"} {
		fs.AddCard("card"+string(rune('0'+i)), fs.AddPCIDevice(pci, "amdgpu", MI300XAttributes("00c0ffee00c0ffee")))
	}
	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	var uuids []string
	for _, dev := range devices {
		uuids = append(uuids, dev.UUID)
	}
	assert.Equal(t, []string{"amd-0000-27-00-0", "amd-0000-27-00-1", "amd-0000-27-00-2"}, uuids)
}
