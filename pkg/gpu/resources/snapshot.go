// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package resources

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// Snapshot builds resource fields from cached physical device attributes.
func Snapshot(devices []safenvml.Device) []Device {
	result := make([]Device, 0, len(devices))
	for _, device := range devices {
		physical, isPhysical := device.(*safenvml.PhysicalDevice)
		if !isPhysical {
			continue
		}
		info := device.GetDeviceInfo()
		if info.VirtualizationMode == nvml.GPU_VIRTUALIZATION_MODE_VGPU {
			continue
		}
		d := Device{
			UUID:               strings.ToLower(info.UUID),
			Vendor:             "nvidia",
			Model:              gpuutil.NormalizeGPUDeviceName(info.Name),
			PCIBusID:           info.PCIBusID,
			DriverVersion:      info.DriverVersion,
			Architecture:       gpuutil.ArchToString(info.Architecture),
			ComputeCapability:  fmt.Sprintf("%d.%d", info.SMVersion/10, info.SMVersion%10),
			CoreCount:          info.CoreCount,
			MemoryTotalBytes:   info.Memory,
			MemoryBusWidth:     info.MemoryBusWidth,
			VirtualizationMode: info.VirtualizationModeName,
			BoardSerial:        strings.ToLower(info.BoardSerial),
			ChassisSerial:      strings.ToLower(info.ChassisSerial),
			FabricGUID:         info.FabricGUID,
			FabricClusterUUID:  info.FabricClusterUUID,
		}
		if info.FabricClusterUUID != "" {
			clique := info.FabricCliqueID
			d.FabricCliqueID = &clique
			d.NVLinkDomain = fmt.Sprintf("%s:%d", info.FabricClusterUUID, clique)
		}
		d.GPUPeers, d.UnresolvedNVLinks = peersFromTopology(physical.PeerTopology())
		result = append(result, d)
	}
	return result
}

// TopologyStatus tells whether the NVLink and P2P facts in a snapshot are complete.
type TopologyStatus struct {
	// Complete is true when every physical GPU was scanned and no query failed. Facts in an
	// incomplete snapshot are the last known values and must not be used as evidence that a
	// link or peer is gone.
	Complete bool
	// IncompleteGPUs holds the lowercase UUIDs of the GPUs with a missing or incomplete scan.
	IncompleteGPUs []string
}

// SnapshotTopologyStatus reports the completeness of the topology facts Snapshot builds for
// the same devices, so that lifecycle handling can tell absence from an unknown.
func SnapshotTopologyStatus(devices []safenvml.Device) TopologyStatus {
	var incomplete []string
	for _, device := range devices {
		physical, isPhysical := device.(*safenvml.PhysicalDevice)
		if !isPhysical || physical.VirtualizationMode == nvml.GPU_VIRTUALIZATION_MODE_VGPU {
			continue
		}
		if topology := physical.PeerTopology(); topology == nil || !topology.Complete {
			incomplete = append(incomplete, strings.ToLower(physical.UUID))
		}
	}
	slices.Sort(incomplete)
	return TopologyStatus{Complete: len(incomplete) == 0, IncompleteGPUs: incomplete}
}

// peersFromTopology converts one GPU's scan into the contract's arrays. Every observed port
// lands in exactly one of them: ports that resolve to a host-local GPU become direct links of
// that peer and the rest are unresolved. A peer is emitted only when it is directly attached
// or at least one P2P capability towards it is supported.
func peersFromTopology(topology *safenvml.PeerTopology) ([]GPUPeer, []UnresolvedNVLink) {
	if topology == nil {
		return nil, nil
	}
	peers := make(map[string]*GPUPeer)
	peer := func(uuid string) *GPUPeer {
		if peers[uuid] == nil {
			peers[uuid] = &GPUPeer{UUID: uuid}
		}
		return peers[uuid]
	}

	var unresolved []UnresolvedNVLink
	for _, port := range topology.Ports {
		if port.PeerUUID != "" {
			p := peer(port.PeerUUID)
			p.DirectNVLinks = append(p.DirectNVLinks, DirectNVLink{
				Port: int32(port.Port), RemotePCIBusID: port.RemotePCIBusID, Version: port.Version,
			})
			continue
		}
		unresolved = append(unresolved, UnresolvedNVLink{
			Port: int32(port.Port), RemoteType: string(port.RemoteType),
			RemotePCIBusID: port.RemotePCIBusID, Version: port.Version,
		})
	}
	for uuid, caps := range topology.P2P {
		if peers[uuid] == nil && !caps.AnyTrue() {
			continue
		}
		if !caps.IsZero() {
			peer(uuid).P2PCapabilities = &P2PCapabilities{
				Read: caps.Read, Write: caps.Write, Atomics: caps.Atomics, NVLink: caps.NVLink, PCI: caps.PCI,
			}
		}
	}

	var result []GPUPeer
	for _, uuid := range slices.Sorted(maps.Keys(peers)) {
		result = append(result, *peers[uuid])
	}
	// Ports come ordered from the scan, but the contract promises the order, so enforce it.
	slices.SortStableFunc(unresolved, func(a, b UnresolvedNVLink) int { return int(a.Port) - int(b.Port) })
	return result, unresolved
}
