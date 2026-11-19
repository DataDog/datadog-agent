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

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
)

func uuidOf(i int) string { return fmt.Sprintf("gpu-aaaaaaaa-0000-0000-0000-%012d", i) }

func ptr(b bool) *bool { return &b }

func gpuWithTopology(i int, topology *safenvml.PeerTopology) *safenvml.PhysicalDevice {
	device := &safenvml.PhysicalDevice{DeviceInfo: safenvml.DeviceInfo{UUID: strings.ToUpper(uuidOf(i))}}
	device.SetPeerTopology(topology)
	return device
}

func snapshotOf(t *testing.T, topology *safenvml.PeerTopology) Device {
	t.Helper()
	result := Snapshot([]safenvml.Device{gpuWithTopology(0, topology)})
	require.Len(t, result, 1)
	return result[0]
}

// hgxCaps is what an HGX H100 reports between any two GPUs: everything over NVLink, no PCI P2P.
func hgxCaps() safenvml.P2PCapabilities {
	return safenvml.P2PCapabilities{Read: ptr(true), Write: ptr(true), Atomics: ptr(true), NVLink: ptr(true), PCI: ptr(false)}
}

// TestSnapshotHGXSwitchConnected: an HGX GPU reaches every other GPU through switches. All
// 18 ports are unresolved switch ports, and every peer is emitted for its P2P capabilities
// alone, with no direct links.
func TestSnapshotHGXSwitchConnected(t *testing.T) {
	topology := &safenvml.PeerTopology{Complete: true, P2P: map[string]safenvml.P2PCapabilities{}}
	for link := range 18 {
		topology.Ports = append(topology.Ports, safenvml.NVLinkPort{
			Port: link, RemoteType: safenvml.NVLinkRemoteSwitch, RemotePCIBusID: fmt.Sprintf("0000:%02x:00.0", 0x80+link%4), Version: "4.0",
		})
	}
	// Inserted out of order on purpose: the output must be ordered by UUID.
	for _, peer := range []int{7, 2, 5, 1, 6, 3, 4} {
		topology.P2P[uuidOf(peer)] = hgxCaps()
	}

	d := snapshotOf(t, topology)
	require.Len(t, d.UnresolvedNVLinks, 18)
	for link, port := range d.UnresolvedNVLinks {
		require.Equal(t, UnresolvedNVLink{
			Port: int32(link), RemoteType: "switch", RemotePCIBusID: fmt.Sprintf("0000:%02x:00.0", 0x80+link%4), Version: "4.0",
		}, port)
	}
	require.Len(t, d.GPUPeers, 7)
	var uuids []string
	for _, peer := range d.GPUPeers {
		uuids = append(uuids, peer.UUID)
		require.Empty(t, peer.DirectNVLinks)
		require.Equal(t, &P2PCapabilities{Read: ptr(true), Write: ptr(true), Atomics: ptr(true), NVLink: ptr(true), PCI: ptr(false)}, peer.P2PCapabilities)
	}
	require.Equal(t, []string{uuidOf(1), uuidOf(2), uuidOf(3), uuidOf(4), uuidOf(5), uuidOf(6), uuidOf(7)}, uuids)
}

// TestSnapshotDirectNVLinks: parallel links stay separate entries of one peer, and every port
// appears exactly once across the two arrays.
func TestSnapshotDirectNVLinks(t *testing.T) {
	topology := &safenvml.PeerTopology{
		Complete: true,
		Ports: []safenvml.NVLinkPort{
			{Port: 0, RemoteType: safenvml.NVLinkRemoteGPU, RemotePCIBusID: "0000:19:00.0", PeerUUID: uuidOf(1), Version: "3.1"},
			{Port: 1, RemoteType: safenvml.NVLinkRemoteGPU, RemotePCIBusID: "0000:19:00.0", PeerUUID: uuidOf(1), Version: "3.1"},
			{Port: 2, RemoteType: safenvml.NVLinkRemoteGPU, RemotePCIBusID: "0000:1a:00.0", PeerUUID: uuidOf(2)},
			{Port: 3, RemoteType: safenvml.NVLinkRemoteGPU, RemotePCIBusID: "0007:40:00.0"}, // not on this host
			{Port: 4, RemoteType: safenvml.NVLinkRemoteSwitch, RemotePCIBusID: "0000:80:00.0"},
			{Port: 5, RemoteType: safenvml.NVLinkRemoteOther},
			{Port: 6, RemoteType: safenvml.NVLinkRemoteUnknown},
		},
		// GPU 2 is attached but reports only explicit negatives: it is still a peer.
		P2P: map[string]safenvml.P2PCapabilities{
			uuidOf(2): {Read: ptr(false), PCI: ptr(false)},
		},
	}
	d := snapshotOf(t, topology)
	require.Equal(t, []GPUPeer{
		{UUID: uuidOf(1), DirectNVLinks: []DirectNVLink{
			{Port: 0, RemotePCIBusID: "0000:19:00.0", Version: "3.1"},
			{Port: 1, RemotePCIBusID: "0000:19:00.0", Version: "3.1"},
		}},
		{UUID: uuidOf(2), P2PCapabilities: &P2PCapabilities{Read: ptr(false), PCI: ptr(false)}, DirectNVLinks: []DirectNVLink{
			{Port: 2, RemotePCIBusID: "0000:1a:00.0"},
		}},
	}, d.GPUPeers)
	require.Equal(t, []UnresolvedNVLink{
		{Port: 3, RemoteType: "gpu", RemotePCIBusID: "0007:40:00.0"},
		{Port: 4, RemoteType: "switch", RemotePCIBusID: "0000:80:00.0"},
		{Port: 5, RemoteType: "other"},
		{Port: 6, RemoteType: "unknown"},
	}, d.UnresolvedNVLinks)

	seen := map[int32]int{}
	for _, peer := range d.GPUPeers {
		for _, link := range peer.DirectNVLinks {
			seen[link.Port]++
		}
	}
	for _, link := range d.UnresolvedNVLinks {
		seen[link.Port]++
	}
	require.Len(t, seen, len(topology.Ports))
	for port, count := range seen {
		require.Equal(t, 1, count, "port %d must be in exactly one array", port)
	}
}

// TestSnapshotPCIeOnly: no NVLink. Peers appear only for a supported capability; GPUs that
// report nothing but explicit negatives are not peers.
func TestSnapshotPCIeOnly(t *testing.T) {
	d := snapshotOf(t, &safenvml.PeerTopology{Complete: true, P2P: map[string]safenvml.P2PCapabilities{
		uuidOf(1): {PCI: ptr(true), NVLink: ptr(false), Read: ptr(true)},
		uuidOf(2): {PCI: ptr(false), Read: ptr(false), NVLink: ptr(false)},
		uuidOf(3): {PCI: ptr(true)},
	}})
	require.Empty(t, d.UnresolvedNVLinks)
	require.Equal(t, []GPUPeer{
		{UUID: uuidOf(1), P2PCapabilities: &P2PCapabilities{PCI: ptr(true), NVLink: ptr(false), Read: ptr(true)}},
		{UUID: uuidOf(3), P2PCapabilities: &P2PCapabilities{PCI: ptr(true)}},
	}, d.GPUPeers)
}

// TestSnapshotUnsupportedDriver: every NVML call returned NOT_SUPPORTED, which leaves an
// empty topology. The fields are absent from the payload, not empty or false.
func TestSnapshotUnsupportedDriver(t *testing.T) {
	for name, topology := range map[string]*safenvml.PeerTopology{
		"scanned, nothing reported": {Complete: true},
		"never scanned":             nil,
	} {
		t.Run(name, func(t *testing.T) {
			d := snapshotOf(t, topology)
			require.Nil(t, d.GPUPeers)
			require.Nil(t, d.UnresolvedNVLinks)
			encoded, err := json.Marshal(d)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "gpu_peers")
			require.NotContains(t, string(encoded), "unresolved_nvlinks")
		})
	}
}

// TestSnapshotAsymmetricP2P: the capability is kept per query direction, so a pair can make
// one GPU a peer of the other without the reverse.
func TestSnapshotAsymmetricP2P(t *testing.T) {
	a := gpuWithTopology(0, &safenvml.PeerTopology{Complete: true, P2P: map[string]safenvml.P2PCapabilities{
		uuidOf(1): {Read: ptr(true), Write: ptr(false)},
	}})
	b := gpuWithTopology(1, &safenvml.PeerTopology{Complete: true, P2P: map[string]safenvml.P2PCapabilities{
		uuidOf(0): {Read: ptr(false), Write: ptr(false)},
	}})
	result := Snapshot([]safenvml.Device{a, b})
	require.Len(t, result, 2)
	require.Equal(t, []GPUPeer{{UUID: uuidOf(1), P2PCapabilities: &P2PCapabilities{Read: ptr(true), Write: ptr(false)}}}, result[0].GPUPeers)
	require.Empty(t, result[1].GPUPeers, "B→A reports no supported capability and B is not attached to A")
}

func TestSnapshotPayloadShape(t *testing.T) {
	d := snapshotOf(t, &safenvml.PeerTopology{
		Complete: true,
		Ports: []safenvml.NVLinkPort{
			{Port: 0, RemoteType: safenvml.NVLinkRemoteGPU, RemotePCIBusID: "0000:19:00.0", PeerUUID: uuidOf(1), Version: "3.1"},
			{Port: 1, RemoteType: safenvml.NVLinkRemoteSwitch, RemotePCIBusID: "0000:80:00.0"},
		},
		P2P: map[string]safenvml.P2PCapabilities{uuidOf(1): {Read: ptr(true), NVLink: ptr(false)}},
	})
	encoded, err := json.Marshal(d)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))
	require.JSONEq(t, fmt.Sprintf(`[{"gpu_uuid":%q,"p2p_capabilities":{"read":true,"nvlink":false},"direct_nvlinks":[{"nvlink_port":0,"remote_pci_bus_id":"0000:19:00.0","version":"3.1"}]}]`, uuidOf(1)), string(fields["gpu_peers"]))
	require.JSONEq(t, `[{"nvlink_port":1,"remote_type":"switch","remote_pci_bus_id":"0000:80:00.0"}]`, string(fields["unresolved_nvlinks"]))
}

func TestSnapshotTopologyStatus(t *testing.T) {
	vgpu := &safenvml.PhysicalDevice{DeviceInfo: safenvml.DeviceInfo{VirtualizationMode: nvml.GPU_VIRTUALIZATION_MODE_VGPU}}
	complete := gpuWithTopology(0, &safenvml.PeerTopology{Complete: true})
	incomplete := gpuWithTopology(1, &safenvml.PeerTopology{Complete: false})
	unscanned := gpuWithTopology(2, nil)

	require.Equal(t, TopologyStatus{Complete: true}, SnapshotTopologyStatus([]safenvml.Device{complete, vgpu, &safenvml.MIGDevice{}}))
	status := SnapshotTopologyStatus([]safenvml.Device{unscanned, complete, incomplete})
	require.False(t, status.Complete)
	require.Equal(t, []string{uuidOf(1), uuidOf(2)}, status.IncompleteGPUs)
}
