// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"fmt"
	"strings"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

type fakeRemote struct {
	deviceType nvml.IntNvLinkDeviceType
	pci        nvml.PciInfo
	typeErr    error
	pciErr     error
}

type fakeP2P struct {
	status nvml.GpuP2PStatus
	err    error
}

// fakeLinkDevice implements only the calls the topology scan makes; any other call panics on
// the nil embedded SafeDevice.
type fakeLinkDevice struct {
	SafeDevice
	remotes map[int]fakeRemote
	p2p     map[string]map[nvml.GpuP2PCapsIndex]fakeP2P // by peer UUID
	version int
}

func (f *fakeLinkDevice) GetNvLinkRemoteDeviceType(link int) (nvml.IntNvLinkDeviceType, error) {
	remote, ok := f.remotes[link]
	if !ok {
		return 0, NewNvmlAPIErrorOrNil("GetNvLinkRemoteDeviceType", nvml.ERROR_NOT_SUPPORTED)
	}
	return remote.deviceType, remote.typeErr
}

func (f *fakeLinkDevice) GetNvLinkRemotePciInfo(link int) (nvml.PciInfo, error) {
	remote, ok := f.remotes[link]
	if !ok {
		return nvml.PciInfo{}, NewNvmlAPIErrorOrNil("GetNvLinkRemotePciInfo", nvml.ERROR_NOT_SUPPORTED)
	}
	return remote.pci, remote.pciErr
}

func (f *fakeLinkDevice) GetNvLinkVersion(int) (int, error) {
	if f.version == 0 {
		return 0, NewNvmlAPIErrorOrNil("GetNvLinkVersion", nvml.ERROR_NOT_SUPPORTED)
	}
	return f.version, nil
}

func (f *fakeLinkDevice) GetP2PStatus(peer SafeDevice, index nvml.GpuP2PCapsIndex) (nvml.GpuP2PStatus, error) {
	result, ok := f.p2p[peer.(*PhysicalDevice).UUID][index]
	if !ok {
		return 0, NewNvmlAPIErrorOrNil("GetP2PStatus", nvml.ERROR_NOT_SUPPORTED)
	}
	return result.status, result.err
}

func gpuUUID(i int) string { return fmt.Sprintf("GPU-AAAAAAAA-0000-0000-0000-%012d", i) }

func gpuBDF(i int) nvml.PciInfo { return nvml.PciInfo{Domain: 0, Bus: uint32(0x18 + i), Device: 0} }

func gpuBDFString(i int) string { return fmt.Sprintf("0000:%02x:00.0", 0x18+i) }

func newFakeGPU(i, linkCount int, dev *fakeLinkDevice) *PhysicalDevice {
	return &PhysicalDevice{
		SafeDevice: dev,
		DeviceInfo: DeviceInfo{UUID: gpuUUID(i), PCIBusID: gpuBDFString(i), NVLinkLinkCount: linkCount},
	}
}

func switchRemote(i int) fakeRemote {
	return fakeRemote{deviceType: nvml.NVLINK_DEVICE_TYPE_SWITCH, pci: nvml.PciInfo{Domain: 0, Bus: uint32(0x80 + i), Device: 0}}
}

func gpuRemote(i int) fakeRemote {
	return fakeRemote{deviceType: nvml.NVLINK_DEVICE_TYPE_GPU, pci: gpuBDF(i)}
}

func allP2P(status nvml.GpuP2PStatus) map[nvml.GpuP2PCapsIndex]fakeP2P {
	result := map[nvml.GpuP2PCapsIndex]fakeP2P{}
	for _, q := range p2pQueries {
		result[q.index] = fakeP2P{status: status}
	}
	return result
}

func boolPtr(b bool) *bool { return &b }

func scanByUUID(devices []*PhysicalDevice, previous map[string]*PeerTopology) map[string]*PeerTopology {
	return scanPeerTopology(devices, previous)
}

func lower(i int) string { return strings.ToLower(gpuUUID(i)) }

// TestScanHGXSwitchConnected models an HGX H100 as captured on real hardware: every GPU has 18
// links to NVSwitches, and every pair reports read/write/atomics/NVLink P2P as OK and PCI P2P as
// not supported. No remote is a GPU, so there are no direct links.
func TestScanHGXSwitchConnected(t *testing.T) {
	const gpus, links = 8, 18
	devices := make([]*PhysicalDevice, gpus)
	for i := range gpus {
		dev := &fakeLinkDevice{remotes: map[int]fakeRemote{}, p2p: map[string]map[nvml.GpuP2PCapsIndex]fakeP2P{}, version: 6}
		for link := range links {
			dev.remotes[link] = switchRemote(link % 4)
		}
		for peer := range gpus {
			if peer != i {
				caps := allP2P(nvml.P2P_STATUS_OK)
				caps[nvml.P2P_CAPS_INDEX_PCI] = fakeP2P{status: nvml.P2P_STATUS_NOT_SUPPORTED}
				dev.p2p[gpuUUID(peer)] = caps
			}
		}
		devices[i] = newFakeGPU(i, links, dev)
	}

	result := scanByUUID(devices, nil)
	require.Len(t, result, gpus)
	for i := range gpus {
		topology := result[lower(i)]
		require.True(t, topology.Complete)
		require.Len(t, topology.Ports, links)
		for link, port := range topology.Ports {
			require.Equal(t, link, port.Port, "ports are ordered")
			require.Equal(t, NVLinkRemoteSwitch, port.RemoteType)
			require.Equal(t, fmt.Sprintf("0000:%02x:00.0", 0x80+link%4), port.RemotePCIBusID)
			require.Equal(t, "4.0", port.Version)
			require.Empty(t, port.PeerUUID, "a switch is never a GPU peer")
		}
		require.Len(t, topology.P2P, gpus-1)
		require.NotContains(t, topology.P2P, lower(i), "a GPU is not its own peer")
		for peer, caps := range topology.P2P {
			require.NotEqual(t, lower(i), peer)
			require.Equal(t, P2PCapabilities{
				Read: boolPtr(true), Write: boolPtr(true), Atomics: boolPtr(true), NVLink: boolPtr(true), PCI: boolPtr(false),
			}, caps)
		}
	}
}

// TestScanDirectNVLinks covers the classification of ports that are not switch-connected:
// parallel links to a local GPU, a remote GPU that is not on this host, other and unknown
// devices, and a port that reports itself.
func TestScanDirectNVLinks(t *testing.T) {
	remotes := map[int]fakeRemote{
		0: gpuRemote(1), // parallel links to GPU 1
		1: gpuRemote(1),
		2: gpuRemote(2),
		3: {deviceType: nvml.NVLINK_DEVICE_TYPE_GPU, pci: nvml.PciInfo{Domain: 7, Bus: 0x40, Device: 0}}, // not on this host
		4: {deviceType: nvml.NVLINK_DEVICE_TYPE_IBMNPU, pci: nvml.PciInfo{Bus: 1}},
		5: {deviceType: nvml.NVLINK_DEVICE_TYPE_UNKNOWN},
		6: {deviceType: nvml.IntNvLinkDeviceType(77)}, // a value newer than this Agent
		7: gpuRemote(0),                               // reports itself
	}
	devices := []*PhysicalDevice{
		newFakeGPU(0, 8, &fakeLinkDevice{remotes: remotes}),
		newFakeGPU(1, 0, &fakeLinkDevice{}),
		newFakeGPU(2, 0, &fakeLinkDevice{}),
	}

	topology := scanByUUID(devices, nil)[lower(0)]
	require.True(t, topology.Complete)
	require.Equal(t, []NVLinkPort{
		{Port: 0, RemoteType: NVLinkRemoteGPU, RemotePCIBusID: gpuBDFString(1), PeerUUID: lower(1)},
		{Port: 1, RemoteType: NVLinkRemoteGPU, RemotePCIBusID: gpuBDFString(1), PeerUUID: lower(1)},
		{Port: 2, RemoteType: NVLinkRemoteGPU, RemotePCIBusID: gpuBDFString(2), PeerUUID: lower(2)},
		{Port: 3, RemoteType: NVLinkRemoteGPU, RemotePCIBusID: "0007:40:00.0"},
		{Port: 4, RemoteType: NVLinkRemoteOther, RemotePCIBusID: "0000:01:00.0"},
		{Port: 5, RemoteType: NVLinkRemoteUnknown, RemotePCIBusID: "0000:00:00.0"},
		{Port: 6, RemoteType: NVLinkRemoteUnknown, RemotePCIBusID: "0000:00:00.0"},
		{Port: 7, RemoteType: NVLinkRemoteGPU, RemotePCIBusID: gpuBDFString(0)},
	}, topology.Ports)
}

func TestScanPCIeOnlyAndUnsupportedDriver(t *testing.T) {
	t.Run("PCIe only keeps explicit negatives", func(t *testing.T) {
		// No NVLink at all: every port query is unsupported. P2P works over PCI for the
		// pair on the same switch and is explicitly unsupported for the rest.
		devices := []*PhysicalDevice{
			newFakeGPU(0, 0, &fakeLinkDevice{p2p: map[string]map[nvml.GpuP2PCapsIndex]fakeP2P{
				gpuUUID(1): {nvml.P2P_CAPS_INDEX_PCI: {status: nvml.P2P_STATUS_OK}, nvml.P2P_CAPS_INDEX_NVLINK: {status: nvml.P2P_STATUS_NOT_SUPPORTED}, nvml.P2P_CAPS_INDEX_READ: {status: nvml.P2P_STATUS_OK}},
				gpuUUID(2): {nvml.P2P_CAPS_INDEX_PCI: {status: nvml.P2P_STATUS_CHIPSET_NOT_SUPPORTED}, nvml.P2P_CAPS_INDEX_READ: {status: nvml.P2P_STATUS_IOH_TOPOLOGY_NOT_SUPPORTED}},
			}}),
			newFakeGPU(1, 0, &fakeLinkDevice{}),
			newFakeGPU(2, 0, &fakeLinkDevice{}),
		}
		topology := scanByUUID(devices, nil)[lower(0)]
		require.True(t, topology.Complete)
		require.Empty(t, topology.Ports)
		require.Equal(t, map[string]P2PCapabilities{
			lower(1): {PCI: boolPtr(true), NVLink: boolPtr(false), Read: boolPtr(true)},
			lower(2): {PCI: boolPtr(false), Read: boolPtr(false)},
		}, topology.P2P)
	})

	t.Run("unsupported driver reports nothing and is not incomplete", func(t *testing.T) {
		// Every call returns NOT_SUPPORTED: the facts are unknown, which must neither be
		// published as false nor flag the snapshot as incomplete.
		devices := []*PhysicalDevice{
			newFakeGPU(0, 4, &fakeLinkDevice{}),
			newFakeGPU(1, 4, &fakeLinkDevice{}),
		}
		for _, topology := range scanByUUID(devices, nil) {
			require.True(t, topology.Complete)
			require.Empty(t, topology.Ports)
			require.Empty(t, topology.P2P)
		}
	})

	t.Run("unknown and out-of-range statuses stay absent", func(t *testing.T) {
		devices := []*PhysicalDevice{
			newFakeGPU(0, 0, &fakeLinkDevice{p2p: map[string]map[nvml.GpuP2PCapsIndex]fakeP2P{
				gpuUUID(1): {nvml.P2P_CAPS_INDEX_READ: {status: nvml.P2P_STATUS_UNKNOWN}, nvml.P2P_CAPS_INDEX_WRITE: {status: nvml.GpuP2PStatus(99)}},
			}}),
			newFakeGPU(1, 0, &fakeLinkDevice{}),
		}
		require.Empty(t, scanByUUID(devices, nil)[lower(0)].P2P)
	})
}

// TestScanAsymmetricP2PKeepsQueryDirection checks that A→B and B→A are reported separately.
func TestScanAsymmetricP2PKeepsQueryDirection(t *testing.T) {
	devices := []*PhysicalDevice{
		newFakeGPU(0, 0, &fakeLinkDevice{p2p: map[string]map[nvml.GpuP2PCapsIndex]fakeP2P{
			gpuUUID(1): {nvml.P2P_CAPS_INDEX_READ: {status: nvml.P2P_STATUS_OK}, nvml.P2P_CAPS_INDEX_WRITE: {status: nvml.P2P_STATUS_NOT_SUPPORTED}},
		}}),
		newFakeGPU(1, 0, &fakeLinkDevice{p2p: map[string]map[nvml.GpuP2PCapsIndex]fakeP2P{
			gpuUUID(0): {nvml.P2P_CAPS_INDEX_READ: {status: nvml.P2P_STATUS_DISABLED_BY_REGKEY}, nvml.P2P_CAPS_INDEX_WRITE: {status: nvml.P2P_STATUS_OK}},
		}}),
	}
	result := scanByUUID(devices, nil)
	require.Equal(t, P2PCapabilities{Read: boolPtr(true), Write: boolPtr(false)}, result[lower(0)].P2P[lower(1)])
	require.Equal(t, P2PCapabilities{Read: boolPtr(false), Write: boolPtr(true)}, result[lower(1)].P2P[lower(0)])
}

// TestScanFailedQueryIsUnknownNotRemoval checks that transient failures keep the previous
// observation and flag the scan as incomplete, while a clean rescan recovers.
func TestScanFailedQueryIsUnknownNotRemoval(t *testing.T) {
	healthy := func() []*PhysicalDevice {
		return []*PhysicalDevice{
			newFakeGPU(0, 2, &fakeLinkDevice{
				remotes: map[int]fakeRemote{0: gpuRemote(1), 1: switchRemote(0)},
				p2p:     map[string]map[nvml.GpuP2PCapsIndex]fakeP2P{gpuUUID(1): allP2P(nvml.P2P_STATUS_OK)},
			}),
			newFakeGPU(1, 0, &fakeLinkDevice{}),
		}
	}
	first := scanByUUID(healthy(), nil)
	require.True(t, first[lower(0)].Complete)
	require.Len(t, first[lower(0)].Ports, 2)

	gpuLost := NewNvmlAPIErrorOrNil("GetNvLinkRemoteDeviceType", nvml.ERROR_GPU_IS_LOST)
	failing := []*PhysicalDevice{
		newFakeGPU(0, 2, &fakeLinkDevice{
			remotes: map[int]fakeRemote{0: {typeErr: gpuLost}, 1: switchRemote(0)},
			p2p: map[string]map[nvml.GpuP2PCapsIndex]fakeP2P{gpuUUID(1): {
				nvml.P2P_CAPS_INDEX_READ:  {err: NewNvmlAPIErrorOrNil("GetP2PStatus", nvml.ERROR_UNKNOWN)},
				nvml.P2P_CAPS_INDEX_WRITE: {status: nvml.P2P_STATUS_NOT_SUPPORTED}, // a real change must still be picked up
			}},
		}),
		newFakeGPU(1, 0, &fakeLinkDevice{}),
	}
	secondAll := scanByUUID(failing, first)
	second := secondAll[lower(0)]
	require.False(t, second.Complete, "a failed query makes the snapshot incomplete")
	require.Equal(t, first[lower(0)].Ports, second.Ports, "the failed port keeps its last observation")
	require.Equal(t, boolPtr(true), second.P2P[lower(1)].Read, "the failed capability keeps its last value")
	require.Equal(t, boolPtr(false), second.P2P[lower(1)].Write, "queries that succeeded are updated")
	require.Equal(t, boolPtr(true), second.P2P[lower(1)].Atomics, "a query that now reports unsupported is unknown, not removal: the last value is kept")

	third := scanByUUID(healthy(), secondAll)[lower(0)]
	require.True(t, third.Complete, "a clean rescan is complete again")
	require.Equal(t, first[lower(0)], third)
}

// TestScanKeepsPortsWhenLinkCountIsLost covers a refresh where the cached link count is gone
// but ports were known before.
func TestScanKeepsPortsWhenLinkCountIsLost(t *testing.T) {
	first := scanByUUID([]*PhysicalDevice{
		newFakeGPU(0, 2, &fakeLinkDevice{remotes: map[int]fakeRemote{0: switchRemote(0), 1: switchRemote(1)}}),
	}, nil)
	second := scanByUUID([]*PhysicalDevice{newFakeGPU(0, 0, &fakeLinkDevice{})}, first)[lower(0)]
	require.False(t, second.Complete)
	require.Equal(t, first[lower(0)].Ports, second.Ports)
}

// TestScanFlagsDroppedPeersAsIncomplete: a GPU that disappears takes its P2P entries with it,
// but the scan of its former peers cannot be called complete.
func TestScanFlagsDroppedPeersAsIncomplete(t *testing.T) {
	two := []*PhysicalDevice{
		newFakeGPU(0, 0, &fakeLinkDevice{p2p: map[string]map[nvml.GpuP2PCapsIndex]fakeP2P{gpuUUID(1): allP2P(nvml.P2P_STATUS_OK)}}),
		newFakeGPU(1, 0, &fakeLinkDevice{}),
	}
	first := scanByUUID(two, nil)
	second := scanByUUID(two[:1], first)[lower(0)]
	require.False(t, second.Complete)
	require.Empty(t, second.P2P)
}

func TestScanSkipsVGPU(t *testing.T) {
	vgpu := newFakeGPU(1, 0, &fakeLinkDevice{})
	vgpu.VirtualizationMode = nvml.GPU_VIRTUALIZATION_MODE_VGPU
	result := scanByUUID([]*PhysicalDevice{newFakeGPU(0, 0, &fakeLinkDevice{}), vgpu}, nil)
	require.Len(t, result, 1)
	require.Contains(t, result, lower(0))
}

// TestDeviceCacheScansPeerTopologyOnRefresh runs the scan through the real wrappers on the NVML
// mock, and checks it happens on refresh and not on reads.
func TestDeviceCacheScansPeerTopologyOnRefresh(t *testing.T) {
	uuids := testutil.GPUUUIDs[:3]
	pci := func(i int) nvml.PciInfo { return nvml.PciInfo{Bus: uint32(0x10 + i)} }
	p2p := func(peer string) map[string]map[nvml.GpuP2PCapsIndex]testutil.MockP2PStatus {
		return map[string]map[nvml.GpuP2PCapsIndex]testutil.MockP2PStatus{
			peer: {nvml.P2P_CAPS_INDEX_READ: {Status: nvml.P2P_STATUS_OK}, nvml.P2P_CAPS_INDEX_PCI: {Status: nvml.P2P_STATUS_NOT_SUPPORTED}},
		}
	}

	var p2pCalls int
	counting := testutil.WithCustomHook(func(d *testutil.MockDevice) {
		orig := d.GetP2PStatusFunc
		d.GetP2PStatusFunc = func(peer nvml.Device, idx nvml.GpuP2PCapsIndex) (nvml.GpuP2PStatus, nvml.Return) {
			p2pCalls++
			return orig(peer, idx)
		}
	})
	mockNvml := testutil.NewMockNVML(
		testutil.WithSymbolsMock(allSymbols),
		testutil.WithPhysicalDeviceUUIDs(uuids),
		testutil.WithDeviceOptions(0,
			testutil.WithPCIInfo(pci(0)),
			testutil.WithCapabilities(testutil.Capabilities{NvLinkGenerationSupported: 5, NvLinkLinkCount: 2}),
			testutil.WithNVLinkRemotes(map[int]testutil.NVLinkRemote{
				0: {Type: nvml.NVLINK_DEVICE_TYPE_GPU, PCI: pci(1)},
				1: {Type: nvml.NVLINK_DEVICE_TYPE_SWITCH, PCI: nvml.PciInfo{Bus: 0x90}},
			}),
			testutil.WithP2PStatuses(p2p(uuids[1])),
			counting,
		),
		testutil.WithDeviceOptions(1, testutil.WithPCIInfo(pci(1)), testutil.WithP2PStatuses(p2p(uuids[0]))),
		testutil.WithDeviceOptions(2, testutil.WithPCIInfo(pci(2))),
	)
	WithMockNVML(t, mockNvml)

	cache := NewDeviceCache()
	require.NoError(t, cache.Refresh())
	require.Positive(t, p2pCalls)
	scanCalls := p2pCalls

	dev0, err := cache.GetByUUID(uuids[0])
	require.NoError(t, err)
	topology := dev0.(*PhysicalDevice).PeerTopology()
	require.NotNil(t, topology)
	require.True(t, topology.Complete)
	require.Equal(t, []NVLinkPort{
		{Port: 0, RemoteType: NVLinkRemoteGPU, RemotePCIBusID: "0000:11:00.0", PeerUUID: strings.ToLower(uuids[1]), Version: "3.1"},
		{Port: 1, RemoteType: NVLinkRemoteSwitch, RemotePCIBusID: "0000:90:00.0", Version: "3.1"},
	}, topology.Ports)
	require.Equal(t, map[string]P2PCapabilities{
		strings.ToLower(uuids[1]): {Read: boolPtr(true), PCI: boolPtr(false)},
	}, topology.P2P)

	// Reads (what a metric collection does) never trigger a scan.
	_, err = cache.AllPhysicalDevices()
	require.NoError(t, err)
	_, err = cache.GetByUUID(uuids[0])
	require.NoError(t, err)
	require.Equal(t, scanCalls, p2pCalls)

	// A GPU with no NVLink and no P2P facts has an empty, complete topology.
	dev2, err := cache.GetByUUID(uuids[2])
	require.NoError(t, err)
	require.Empty(t, dev2.(*PhysicalDevice).PeerTopology().Ports)
	require.Empty(t, dev2.(*PhysicalDevice).PeerTopology().P2P)
}
