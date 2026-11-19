// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"slices"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
)

// NVLinkRemoteType is the kind of device observed at the remote end of a local NVLink port.
type NVLinkRemoteType string

// Remote device kinds. The values match the REDAPL gpu_device contract.
const (
	NVLinkRemoteGPU     NVLinkRemoteType = "gpu"
	NVLinkRemoteSwitch  NVLinkRemoteType = "switch"
	NVLinkRemoteOther   NVLinkRemoteType = "other"
	NVLinkRemoteUnknown NVLinkRemoteType = "unknown"
)

// NVLinkPort is the last observation of one local NVLink port.
type NVLinkPort struct {
	// Port is the local physical NVLink index. Zero is valid.
	Port int
	// RemoteType is the kind of device NVML reports at the other end.
	RemoteType NVLinkRemoteType
	// RemotePCIBusID is the normalized PCI BDF reported for the remote device, or empty when
	// NVML did not report one. It is a locator, never an identity.
	RemotePCIBusID string
	// Version is the NVLink protocol version of the port, or empty when not reported.
	Version string
	// PeerUUID is the lowercase UUID of the host-local GPU the remote BDF resolves to. It is
	// only set for remote GPUs that resolve to another physical GPU of this host.
	PeerUUID string
}

// P2PCapabilities holds the peer-to-peer capabilities of an ordered GPU pair. True means
// supported, false means explicitly unsupported or disabled and nil means unknown.
type P2PCapabilities struct {
	Read    *bool
	Write   *bool
	Atomics *bool
	NVLink  *bool
	PCI     *bool
}

// IsZero reports whether no capability is known.
func (c P2PCapabilities) IsZero() bool {
	return c.Read == nil && c.Write == nil && c.Atomics == nil && c.NVLink == nil && c.PCI == nil
}

// AnyTrue reports whether at least one capability is known to be supported.
func (c P2PCapabilities) AnyTrue() bool {
	for _, v := range []*bool{c.Read, c.Write, c.Atomics, c.NVLink, c.PCI} {
		if v != nil && *v {
			return true
		}
	}
	return false
}

// PeerTopology is the last known NVLink and P2P view of one physical GPU. It is built when
// the device cache is refreshed and is immutable afterwards.
type PeerTopology struct {
	// Ports holds the observed local NVLink ports, ordered by port.
	Ports []NVLinkPort
	// P2P holds the capabilities from this GPU (the source of the query) towards each other
	// physical GPU of the host, keyed by lowercase peer UUID. Peers with no known capability
	// are absent.
	P2P map[string]P2PCapabilities
	// Complete is false when any query for this GPU failed for a reason other than the API
	// being unsupported, or when facts known earlier could not be re-confirmed. The values
	// above are then partially carried over from the previous scan: a failed query means
	// unknown, never removal.
	Complete bool
}

type p2pQuery struct {
	index nvml.GpuP2PCapsIndex
	field func(*P2PCapabilities) **bool
}

var p2pQueries = []p2pQuery{
	{nvml.P2P_CAPS_INDEX_READ, func(c *P2PCapabilities) **bool { return &c.Read }},
	{nvml.P2P_CAPS_INDEX_WRITE, func(c *P2PCapabilities) **bool { return &c.Write }},
	{nvml.P2P_CAPS_INDEX_ATOMICS, func(c *P2PCapabilities) **bool { return &c.Atomics }},
	{nvml.P2P_CAPS_INDEX_NVLINK, func(c *P2PCapabilities) **bool { return &c.NVLink }},
	{nvml.P2P_CAPS_INDEX_PCI, func(c *P2PCapabilities) **bool { return &c.PCI }},
}

// p2pStatusToBool maps NVML's P2P status to the published tri-state: OK is true, the explicit
// unsupported/disabled statuses are false, and anything else (unknown, new values) is unknown.
func p2pStatusToBool(status nvml.GpuP2PStatus) *bool {
	var value bool
	switch status {
	case nvml.P2P_STATUS_OK:
		value = true
	case nvml.P2P_STATUS_CHIPSET_NOT_SUPPORTED,
		nvml.P2P_STATUS_GPU_NOT_SUPPORTED,
		nvml.P2P_STATUS_IOH_TOPOLOGY_NOT_SUPPORTED,
		nvml.P2P_STATUS_DISABLED_BY_REGKEY,
		nvml.P2P_STATUS_NOT_SUPPORTED:
		value = false
	default:
		return nil
	}
	return &value
}

func nvlinkRemoteType(deviceType nvml.IntNvLinkDeviceType) NVLinkRemoteType {
	switch deviceType {
	case nvml.NVLINK_DEVICE_TYPE_GPU:
		return NVLinkRemoteGPU
	case nvml.NVLINK_DEVICE_TYPE_SWITCH:
		return NVLinkRemoteSwitch
	case nvml.NVLINK_DEVICE_TYPE_IBMNPU:
		return NVLinkRemoteOther
	default:
		return NVLinkRemoteUnknown
	}
}

// peerTopologyScanner performs one scan of all the physical GPUs of the host.
type peerTopologyScanner struct {
	devices  []*PhysicalDevice
	previous map[string]*PeerTopology // by lowercase UUID
	byBDF    map[string]string        // normalized BDF -> lowercase UUID
	byUUID   map[string]*PhysicalDevice
}

// scanPeerTopology queries NVLink ports and pairwise P2P capabilities for the given physical
// GPUs. It is O(ports*GPUs + GPUs²) NVML calls and must only run when the device cache is
// refreshed. previous holds the results of the previous scan, used to keep last-known facts
// when a query fails. The result is keyed by lowercase UUID.
func scanPeerTopology(devices []*PhysicalDevice, previous map[string]*PeerTopology) map[string]*PeerTopology {
	s := &peerTopologyScanner{
		previous: previous,
		byBDF:    make(map[string]string, len(devices)),
		byUUID:   make(map[string]*PhysicalDevice, len(devices)),
	}
	for _, device := range devices {
		if device.VirtualizationMode == nvml.GPU_VIRTUALIZATION_MODE_VGPU {
			continue
		}
		s.devices = append(s.devices, device)
		uuid := strings.ToLower(device.UUID)
		s.byUUID[uuid] = device
		if device.PCIBusID != "" {
			s.byBDF[device.PCIBusID] = uuid
		}
	}

	result := make(map[string]*PeerTopology, len(s.devices))
	for _, device := range s.devices {
		result[strings.ToLower(device.UUID)] = s.scanDevice(device)
	}
	return result
}

func (s *peerTopologyScanner) scanDevice(device *PhysicalDevice) *PeerTopology {
	uuid := strings.ToLower(device.UUID)
	previous := s.previous[uuid]
	topology := &PeerTopology{Complete: true}

	topology.Ports = s.scanPorts(device, previous, &topology.Complete)
	topology.P2P = s.scanP2P(device, previous, &topology.Complete)
	return topology
}

func (s *peerTopologyScanner) scanPorts(device *PhysicalDevice, previous *PeerTopology, complete *bool) []NVLinkPort {
	previousPorts := map[int]NVLinkPort{}
	if previous != nil {
		for _, port := range previous.Ports {
			previousPorts[port.Port] = port
		}
	}

	linkCount := device.NVLinkLinkCount
	var ports []NVLinkPort
	seen := make(map[int]struct{}, linkCount)
	for link := range linkCount {
		port, err := s.scanPort(device, link)
		if err != nil {
			if !IsUnsupported(err) {
				*complete = false
			}
			// A failed query is unknown, not removal: keep what was last observed.
			if prev, ok := previousPorts[link]; ok {
				port, err = prev, nil
			}
		}
		if err != nil {
			continue
		}
		seen[link] = struct{}{}
		ports = append(ports, port)
	}

	// The link count itself could not be read this time (it is cached with the device), but
	// ports were known before: keep them rather than reporting that the links vanished.
	if linkCount == 0 && len(previousPorts) > 0 {
		*complete = false
		for link, port := range previousPorts {
			if _, ok := seen[link]; !ok {
				ports = append(ports, port)
			}
		}
	}

	slices.SortFunc(ports, func(a, b NVLinkPort) int { return a.Port - b.Port })
	// Resolution depends on the current GPU set, so redo it for carried-over ports as well.
	for i := range ports {
		ports[i].PeerUUID = ""
		if ports[i].RemoteType != NVLinkRemoteGPU {
			continue
		}
		if peer, ok := s.byBDF[ports[i].RemotePCIBusID]; ok && peer != strings.ToLower(device.UUID) {
			ports[i].PeerUUID = peer
		}
	}
	return ports
}

// scanPort queries one local NVLink port. A returned error means the port could not be
// observed (the remote type or a reported remote PCI info could not be read).
func (s *peerTopologyScanner) scanPort(device *PhysicalDevice, link int) (NVLinkPort, error) {
	deviceType, err := device.GetNvLinkRemoteDeviceType(link)
	if err != nil {
		return NVLinkPort{}, err
	}
	port := NVLinkPort{Port: link, RemoteType: nvlinkRemoteType(deviceType)}

	pciInfo, err := device.GetNvLinkRemotePciInfo(link)
	switch {
	case err == nil:
		port.RemotePCIBusID = gpuutil.PCIInfoToBusID(pciInfo)
	case IsUnsupported(err):
		// The remote BDF is only a locator; a driver that does not report it still
		// tells us what the port is attached to.
	default:
		return NVLinkPort{}, err
	}

	if version, err := device.GetNvLinkVersion(link); err == nil {
		port.Version = nvlinkVersionString(version)
	}
	return port, nil
}

func (s *peerTopologyScanner) scanP2P(device *PhysicalDevice, previous *PeerTopology, complete *bool) map[string]P2PCapabilities {
	self := strings.ToLower(device.UUID)
	result := make(map[string]P2PCapabilities)
	for _, peer := range s.devices {
		peerUUID := strings.ToLower(peer.UUID)
		if peerUUID == self {
			continue
		}
		var caps P2PCapabilities
		var previousCaps P2PCapabilities
		if previous != nil {
			previousCaps = previous.P2P[peerUUID]
		}
		for _, query := range p2pQueries {
			field := query.field(&caps)
			status, err := device.GetP2PStatus(peer, query.index)
			if err != nil {
				if !IsUnsupported(err) {
					*complete = false
				}
				// Unknown, not removal: keep the last known value for this direction.
				*field = *query.field(&previousCaps)
				continue
			}
			*field = p2pStatusToBool(status)
		}
		if !caps.IsZero() {
			result[peerUUID] = caps
		}
	}

	// Facts about GPUs that are no longer enumerated are dropped with them, but the scan
	// can no longer be called complete when that happens.
	if previous != nil {
		for peerUUID := range previous.P2P {
			if _, ok := s.byUUID[peerUUID]; !ok {
				*complete = false
			}
		}
	}
	return result
}
