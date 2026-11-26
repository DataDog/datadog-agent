// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package resources

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

type fakeRemote struct {
	deviceType nvml.IntNvLinkDeviceType
	pci        nvml.PciInfo
	typeErr    error
	pciErr     error
	versionErr error
}

type fakeP2P struct {
	status nvml.GpuP2PStatus
	err    error
}

// fakeLinkDevice implements only the calls the peering scan makes; any other call panics on
// the nil embedded SafeDevice.
type fakeLinkDevice struct {
	safenvml.SafeDevice
	remotes map[int]fakeRemote
	p2p     map[string]map[nvml.GpuP2PCapsIndex]fakeP2P // by peer UUID as reported by NVML
	version int
	calls   *int
}

func unsupported(api string) error {
	return safenvml.NewNvmlAPIErrorOrNil(api, nvml.ERROR_NOT_SUPPORTED)
}

func (f *fakeLinkDevice) count() {
	if f.calls != nil {
		*f.calls++
	}
}

func (f *fakeLinkDevice) GetNvLinkRemoteDeviceType(link int) (nvml.IntNvLinkDeviceType, error) {
	f.count()
	remote, ok := f.remotes[link]
	if !ok {
		return 0, unsupported("GetNvLinkRemoteDeviceType")
	}
	return remote.deviceType, remote.typeErr
}

func (f *fakeLinkDevice) GetNvLinkRemotePciInfo(link int) (nvml.PciInfo, error) {
	f.count()
	remote, ok := f.remotes[link]
	if !ok {
		return nvml.PciInfo{}, unsupported("GetNvLinkRemotePciInfo")
	}
	return remote.pci, remote.pciErr
}

func (f *fakeLinkDevice) GetNvLinkVersion(link int) (int, error) {
	f.count()
	if err := f.remotes[link].versionErr; err != nil {
		return 0, err
	}
	if f.version == 0 {
		return 0, unsupported("GetNvLinkVersion")
	}
	return f.version, nil
}

func (f *fakeLinkDevice) GetP2PStatus(peer safenvml.SafeDevice, index nvml.GpuP2PCapsIndex) (nvml.GpuP2PStatus, error) {
	f.count()
	result, ok := f.p2p[peer.(*safenvml.PhysicalDevice).UUID][index]
	if !ok {
		return 0, unsupported("GetP2PStatus")
	}
	return result.status, result.err
}

func gpuUUID(i int) string { return fmt.Sprintf("GPU-AAAAAAAA-0000-0000-0000-%012d", i) }

func lower(i int) string { return strings.ToLower(gpuUUID(i)) }

func gpuBDF(i int) nvml.PciInfo { return nvml.PciInfo{Bus: uint32(0x18 + i)} }

func gpuBDFString(i int) string { return fmt.Sprintf("0000:%02x:00.0", 0x18+i) }

func switchRemote(i int) fakeRemote {
	return fakeRemote{deviceType: nvml.NVLINK_DEVICE_TYPE_SWITCH, pci: nvml.PciInfo{Bus: uint32(0x0b + i)}}
}

func switchBDFString(i int) string { return fmt.Sprintf("0000:%02x:00.0", 0x0b+i) }

func gpuRemote(i int) fakeRemote {
	return fakeRemote{deviceType: nvml.NVLINK_DEVICE_TYPE_GPU, pci: gpuBDF(i)}
}

func newFakeGPU(i, linkCount int, dev *fakeLinkDevice) *safenvml.PhysicalDevice {
	return &safenvml.PhysicalDevice{
		SafeDevice: dev,
		DeviceInfo: safenvml.DeviceInfo{UUID: gpuUUID(i), PCIBusID: gpuBDFString(i), NVLinkLinkCount: linkCount},
	}
}

func devices(gpus ...*safenvml.PhysicalDevice) []safenvml.Device {
	result := make([]safenvml.Device, len(gpus))
	for i, gpu := range gpus {
		result[i] = gpu
	}
	return result
}

func allP2P(status nvml.GpuP2PStatus) map[nvml.GpuP2PCapsIndex]fakeP2P {
	result := map[nvml.GpuP2PCapsIndex]fakeP2P{}
	for _, q := range p2pQueries {
		result[q.index] = fakeP2P{status: status}
	}
	return result
}

func ptr(b bool) *bool { return &b }

func scanOnce(gpus ...*safenvml.PhysicalDevice) map[string]Peering {
	return NewPeeringScanner(0).Scan(time.Unix(0, 0), devices(gpus...))
}

// hgxCaps is what an HGX H100 reports between any two GPUs (captured on real hardware):
// everything over NVLink, PCI P2P not supported.
func hgxCaps() *P2PCapabilities {
	return &P2PCapabilities{Read: ptr(true), Write: ptr(true), Atomics: ptr(true), NVLink: ptr(true), PCI: ptr(false)}
}

// TestPeeringHGXSwitchConnected models an HGX H100 as captured on real hardware: 18 links per
// GPU to 4 NVSwitches. No remote is a GPU, so every port is an unresolved switch port and
// every peer is published for its P2P capabilities alone.
func TestPeeringHGXSwitchConnected(t *testing.T) {
	const gpus, links = 8, 18
	all := make([]*safenvml.PhysicalDevice, gpus)
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
		all[i] = newFakeGPU(i, links, dev)
	}

	result := scanOnce(all...)
	require.Len(t, result, gpus)
	for i := range gpus {
		p := result[lower(i)]
		require.Len(t, p.UnresolvedNVLinks, links)
		for link, port := range p.UnresolvedNVLinks {
			require.Equal(t, UnresolvedNVLink{Port: int32(link), RemoteType: "switch", RemotePCIBusID: switchBDFString(link % 4), Version: "4.0"}, port)
		}
		require.Len(t, p.GPUPeers, gpus-1)
		for j, peer := range p.GPUPeers {
			require.NotEqual(t, lower(i), peer.UUID, "a GPU is not its own peer")
			if j > 0 {
				require.Less(t, p.GPUPeers[j-1].UUID, peer.UUID, "peers are ordered by UUID")
			}
			require.Empty(t, peer.DirectNVLinks, "a switch is never a GPU peer")
			require.Equal(t, hgxCaps(), peer.P2PCapabilities)
		}
	}
}

// TestPeeringDirectNVLinks covers ports that are not switch-connected: parallel links to a
// local GPU, a remote GPU that is not on this host, other and unknown devices, and a port that
// reports itself. Every port lands in exactly one array.
func TestPeeringDirectNVLinks(t *testing.T) {
	remotes := map[int]fakeRemote{
		0: gpuRemote(1), // parallel links to GPU 1
		1: gpuRemote(1),
		2: gpuRemote(2),
		3: {deviceType: nvml.NVLINK_DEVICE_TYPE_GPU, pci: nvml.PciInfo{Domain: 7, Bus: 0x40}}, // not on this host
		4: {deviceType: nvml.NVLINK_DEVICE_TYPE_IBMNPU, pci: nvml.PciInfo{Bus: 1}},
		5: {deviceType: nvml.NVLINK_DEVICE_TYPE_UNKNOWN, pciErr: unsupported("GetNvLinkRemotePciInfo")},
		6: {deviceType: nvml.IntNvLinkDeviceType(77), pciErr: unsupported("GetNvLinkRemotePciInfo")}, // newer than this Agent
		7: gpuRemote(0),                                                                              // reports itself
	}
	result := scanOnce(
		newFakeGPU(0, 8, &fakeLinkDevice{remotes: remotes, version: 5, p2p: map[string]map[nvml.GpuP2PCapsIndex]fakeP2P{
			// GPU 2 is attached but reports only explicit negatives: it is still a peer.
			gpuUUID(2): {nvml.P2P_CAPS_INDEX_READ: {status: nvml.P2P_STATUS_NOT_SUPPORTED}},
		}}),
		newFakeGPU(1, 0, &fakeLinkDevice{}),
		newFakeGPU(2, 0, &fakeLinkDevice{}),
	)
	p := result[lower(0)]
	require.Equal(t, []GPUPeer{
		{UUID: lower(1), DirectNVLinks: []DirectNVLink{
			{Port: 0, RemotePCIBusID: gpuBDFString(1), Version: "3.1"},
			{Port: 1, RemotePCIBusID: gpuBDFString(1), Version: "3.1"},
		}},
		{UUID: lower(2), P2PCapabilities: &P2PCapabilities{Read: ptr(false)}, DirectNVLinks: []DirectNVLink{
			{Port: 2, RemotePCIBusID: gpuBDFString(2), Version: "3.1"},
		}},
	}, p.GPUPeers)
	require.Equal(t, []UnresolvedNVLink{
		{Port: 3, RemoteType: "gpu", RemotePCIBusID: "0007:40:00.0", Version: "3.1"},
		{Port: 4, RemoteType: "other", RemotePCIBusID: "0000:01:00.0", Version: "3.1"},
		{Port: 5, RemoteType: "unknown", Version: "3.1"},
		{Port: 6, RemoteType: "unknown", Version: "3.1"},
		{Port: 7, RemoteType: "gpu", RemotePCIBusID: gpuBDFString(0), Version: "3.1"},
	}, p.UnresolvedNVLinks)
}

func TestPeeringPCIeOnly(t *testing.T) {
	// No NVLink: every port query is unsupported. Peers appear only for a supported
	// capability; GPU 2 reports nothing but explicit negatives and is not a peer.
	result := scanOnce(
		newFakeGPU(0, 0, &fakeLinkDevice{p2p: map[string]map[nvml.GpuP2PCapsIndex]fakeP2P{
			gpuUUID(1): {nvml.P2P_CAPS_INDEX_PCI: {status: nvml.P2P_STATUS_OK}, nvml.P2P_CAPS_INDEX_NVLINK: {status: nvml.P2P_STATUS_NOT_SUPPORTED}},
			gpuUUID(2): {nvml.P2P_CAPS_INDEX_PCI: {status: nvml.P2P_STATUS_CHIPSET_NOT_SUPPORTED}, nvml.P2P_CAPS_INDEX_READ: {status: nvml.P2P_STATUS_IOH_TOPOLOGY_NOT_SUPPORTED}},
		}}),
		newFakeGPU(1, 0, &fakeLinkDevice{}),
		newFakeGPU(2, 0, &fakeLinkDevice{}),
	)
	p := result[lower(0)]
	require.Empty(t, p.UnresolvedNVLinks)
	require.Equal(t, []GPUPeer{{UUID: lower(1), P2PCapabilities: &P2PCapabilities{PCI: ptr(true), NVLink: ptr(false)}}}, p.GPUPeers)
}

// TestPeeringUnsupportedDriver: every NVML call returns NOT_SUPPORTED. Nothing is known, which
// must not be published as false, and an API that was never supported is a complete answer.
func TestPeeringUnsupportedDriver(t *testing.T) {
	all := devices(newFakeGPU(0, 4, &fakeLinkDevice{}), newFakeGPU(1, 4, &fakeLinkDevice{}))
	peering := NewPeeringScanner(0).Scan(time.Unix(0, 0), all)
	for _, p := range peering {
		require.Empty(t, p.GPUPeers)
		require.Empty(t, p.UnresolvedNVLinks)
	}
	for _, d := range Snapshot(all, peering) {
		encoded, err := json.Marshal(d)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "gpu_peers")
		require.NotContains(t, string(encoded), "unresolved_nvlinks")
	}
}

func TestPeeringUninterpretableStatusIsUnknown(t *testing.T) {
	result := scanOnce(
		newFakeGPU(0, 0, &fakeLinkDevice{p2p: map[string]map[nvml.GpuP2PCapsIndex]fakeP2P{
			gpuUUID(1): {nvml.P2P_CAPS_INDEX_READ: {status: nvml.P2P_STATUS_UNKNOWN}, nvml.P2P_CAPS_INDEX_WRITE: {status: nvml.GpuP2PStatus(99)}},
		}}),
		newFakeGPU(1, 0, &fakeLinkDevice{}),
	)
	require.Empty(t, result[lower(0)].GPUPeers, "an unknown status is not a capability")
}

// TestPeeringAsymmetricP2P checks that A→B and B→A are kept apart: a pair can make one GPU a
// peer of the other without the reverse.
func TestPeeringAsymmetricP2P(t *testing.T) {
	result := scanOnce(
		newFakeGPU(0, 0, &fakeLinkDevice{p2p: map[string]map[nvml.GpuP2PCapsIndex]fakeP2P{
			gpuUUID(1): {nvml.P2P_CAPS_INDEX_READ: {status: nvml.P2P_STATUS_OK}, nvml.P2P_CAPS_INDEX_WRITE: {status: nvml.P2P_STATUS_NOT_SUPPORTED}},
		}}),
		newFakeGPU(1, 0, &fakeLinkDevice{p2p: map[string]map[nvml.GpuP2PCapsIndex]fakeP2P{
			gpuUUID(0): {nvml.P2P_CAPS_INDEX_READ: {status: nvml.P2P_STATUS_DISABLED_BY_REGKEY}, nvml.P2P_CAPS_INDEX_WRITE: {status: nvml.P2P_STATUS_NOT_SUPPORTED}},
		}}),
	)
	require.Equal(t, []GPUPeer{{UUID: lower(1), P2PCapabilities: &P2PCapabilities{Read: ptr(true), Write: ptr(false)}}}, result[lower(0)].GPUPeers)
	require.Empty(t, result[lower(1)].GPUPeers, "B→A reports no supported capability and B is not attached to A")
}

// TestPeeringFailedQueriesAreUnknown checks that each scan only publishes what NVML answered:
// a failed query leaves its field out, and nothing is carried over from an earlier scan.
func TestPeeringFailedQueriesAreUnknown(t *testing.T) {
	scanner := NewPeeringScanner(0)
	first := scanner.Scan(time.Unix(0, 0), devices(
		newFakeGPU(0, 2, &fakeLinkDevice{
			version: 5,
			remotes: map[int]fakeRemote{0: gpuRemote(1), 1: switchRemote(0)},
			p2p:     map[string]map[nvml.GpuP2PCapsIndex]fakeP2P{gpuUUID(1): allP2P(nvml.P2P_STATUS_OK)},
		}),
		newFakeGPU(1, 0, &fakeLinkDevice{}),
	))[lower(0)]
	require.Len(t, first.GPUPeers, 1)

	failed := safenvml.NewNvmlAPIErrorOrNil("GetP2PStatus", nvml.ERROR_UNKNOWN)
	second := scanner.Scan(time.Unix(1, 0), devices(
		newFakeGPU(0, 2, &fakeLinkDevice{
			version: 5,
			remotes: map[int]fakeRemote{
				0: {typeErr: safenvml.NewNvmlAPIErrorOrNil("GetNvLinkRemoteDeviceType", nvml.ERROR_GPU_IS_LOST)},
				1: {deviceType: nvml.NVLINK_DEVICE_TYPE_SWITCH, pciErr: failed, versionErr: failed},
			},
			p2p: map[string]map[nvml.GpuP2PCapsIndex]fakeP2P{gpuUUID(1): {
				nvml.P2P_CAPS_INDEX_READ:  {err: failed},
				nvml.P2P_CAPS_INDEX_WRITE: {status: nvml.P2P_STATUS_OK},
			}},
		}),
		newFakeGPU(1, 0, &fakeLinkDevice{}),
	))[lower(0)]
	require.Equal(t, []GPUPeer{{UUID: lower(1), P2PCapabilities: &P2PCapabilities{Write: ptr(true)}}}, second.GPUPeers,
		"the port whose type could not be read is gone, and only the answered capability is published")
	require.Equal(t, []UnresolvedNVLink{{Port: 1, RemoteType: "switch"}}, second.UnresolvedNVLinks,
		"an address or version that could not be read is left out")
}

// TestPeeringScanCadence: peering is static, so it is rescanned once per interval or when the
// set of GPUs changes, not on every collection.
func TestPeeringScanCadence(t *testing.T) {
	calls := 0
	gpu := func(i int) *safenvml.PhysicalDevice {
		return newFakeGPU(i, 1, &fakeLinkDevice{remotes: map[int]fakeRemote{0: switchRemote(0)}, calls: &calls})
	}
	scanner := NewPeeringScanner(time.Minute)
	start := time.Unix(1000, 0)

	first := scanner.Scan(start, devices(gpu(0), gpu(1)))
	require.Positive(t, calls)
	afterFirst := calls

	require.Equal(t, first, scanner.Scan(start.Add(30*time.Second), devices(gpu(0), gpu(1))))
	require.Equal(t, afterFirst, calls, "no NVML call within the interval")

	scanner.Scan(start.Add(40*time.Second), devices(gpu(0)))
	require.Greater(t, calls, afterFirst, "a GPU set change rescans")
	afterChange := calls

	scanner.Scan(start.Add(2*time.Minute), devices(gpu(0)))
	require.Greater(t, calls, afterChange, "the interval elapsing rescans")
}

func TestPeeringSkipsVirtualAndMIGDevices(t *testing.T) {
	vgpu := newFakeGPU(1, 0, &fakeLinkDevice{})
	vgpu.VirtualizationMode = nvml.GPU_VIRTUALIZATION_MODE_VGPU
	all := []safenvml.Device{newFakeGPU(0, 0, &fakeLinkDevice{}), vgpu, &safenvml.MIGDevice{}}
	peering := NewPeeringScanner(0).Scan(time.Unix(0, 0), all)
	require.Len(t, peering, 1)
	require.Contains(t, peering, lower(0))
}

func TestSnapshotPeeringPayloadShape(t *testing.T) {
	all := devices(
		newFakeGPU(0, 2, &fakeLinkDevice{version: 5, remotes: map[int]fakeRemote{0: gpuRemote(1), 1: switchRemote(0)}, p2p: map[string]map[nvml.GpuP2PCapsIndex]fakeP2P{
			gpuUUID(1): {nvml.P2P_CAPS_INDEX_READ: {status: nvml.P2P_STATUS_OK}, nvml.P2P_CAPS_INDEX_NVLINK: {status: nvml.P2P_STATUS_NOT_SUPPORTED}},
		}}),
		newFakeGPU(1, 0, &fakeLinkDevice{}),
	)
	d := Snapshot(all, NewPeeringScanner(0).Scan(time.Unix(0, 0), all))[0]
	encoded, err := json.Marshal(d)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))
	require.JSONEq(t, fmt.Sprintf(`[{"gpu_uuid":%q,"p2p_capabilities":{"read":true,"nvlink":false},"direct_nvlinks":[{"nvlink_port":0,"remote_pci_bus_id":%q,"version":"3.1"}]}]`, lower(1), gpuBDFString(1)), string(fields["gpu_peers"]))
	require.JSONEq(t, fmt.Sprintf(`[{"nvlink_port":1,"remote_type":"switch","remote_pci_bus_id":%q,"version":"3.1"}]`, switchBDFString(0)), string(fields["unresolved_nvlinks"]))
}

// TestPeeringThroughNVMLMock runs the scan through the real safenvml wrappers on the NVML mock.
func TestPeeringThroughNVMLMock(t *testing.T) {
	uuids := testutil.GPUUUIDs[:3]
	pci := func(i int) nvml.PciInfo { return nvml.PciInfo{Bus: uint32(0x10 + i)} }
	mockNvml := testutil.NewMockNVML(
		testutil.WithPhysicalDeviceUUIDs(uuids),
		testutil.WithDeviceOptions(0,
			testutil.WithPCIInfo(pci(0)),
			testutil.WithCapabilities(testutil.Capabilities{NvLinkGenerationSupported: 5, NvLinkLinkCount: 2}),
			testutil.WithNVLinkRemotes(map[int]testutil.NVLinkRemote{
				0: {Type: nvml.NVLINK_DEVICE_TYPE_GPU, PCI: pci(1)},
				1: {Type: nvml.NVLINK_DEVICE_TYPE_SWITCH, PCI: nvml.PciInfo{Bus: 0x90}},
			}),
			testutil.WithP2PStatuses(map[string]map[nvml.GpuP2PCapsIndex]testutil.MockP2PStatus{
				uuids[1]: {nvml.P2P_CAPS_INDEX_READ: {Status: nvml.P2P_STATUS_OK}, nvml.P2P_CAPS_INDEX_PCI: {Status: nvml.P2P_STATUS_NOT_SUPPORTED}},
			}),
		),
		testutil.WithDeviceOptions(1, testutil.WithPCIInfo(pci(1))),
		testutil.WithDeviceOptions(2, testutil.WithPCIInfo(pci(2))),
	)
	safenvml.WithMockNVML(t, mockNvml)
	cache := safenvml.NewDeviceCache()
	require.NoError(t, cache.Refresh())
	all, err := cache.AllPhysicalDevices()
	require.NoError(t, err)

	peering := NewPeeringScanner(0).Scan(time.Unix(0, 0), all)
	p := peering[strings.ToLower(uuids[0])]
	require.Equal(t, []GPUPeer{{
		UUID:            strings.ToLower(uuids[1]),
		P2PCapabilities: &P2PCapabilities{Read: ptr(true), PCI: ptr(false)},
		DirectNVLinks:   []DirectNVLink{{Port: 0, RemotePCIBusID: "0000:11:00.0", Version: "3.1"}},
	}}, p.GPUPeers)
	require.Equal(t, []UnresolvedNVLink{{Port: 1, RemoteType: "switch", RemotePCIBusID: "0000:90:00.0", Version: "3.1"}}, p.UnresolvedNVLinks)
	require.Empty(t, peering[strings.ToLower(uuids[2])].GPUPeers)
}
