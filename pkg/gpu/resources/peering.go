// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package resources

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
)

// Peering is the NVLink and P2P peering of one GPU, in the shape published in gpu_device.
type Peering struct {
	GPUPeers          []GPUPeer
	UnresolvedNVLinks []UnresolvedNVLink
	// Complete is false when a query failed or returned something that could not be
	// interpreted, or when facts known from an earlier scan could not be re-confirmed. The
	// arrays then hold the last known values: a failed query is unknown, not removal.
	Complete bool
}

// nvlinkPort is what was last observed on one local NVLink port.
type nvlinkPort struct {
	remoteType string // gpu, switch, other or unknown, as in the contract
	remoteBDF  string // normalized; a locator, never an identity
	version    string
}

// gpuPeering holds one GPU's last scan: its ports, the P2P capabilities towards every peer
// (including peers with only negative capabilities, which are not published but must be
// remembered), and the result built from them.
type gpuPeering struct {
	ports  map[int]nvlinkPort
	p2p    map[string]P2PCapabilities // by lowercase peer UUID, in query direction
	result Peering
}

// PeeringScanner scans NVLink ports and pairwise P2P capabilities of the host's physical GPUs
// and remembers the last scan, so that a failed query keeps the last known value. Peering is
// static, so the scan is O(ports*GPUs + GPUs²) NVML calls that run at most once per interval,
// or when the set of GPUs changes, never on every collection.
type PeeringScanner struct {
	interval time.Duration
	lastScan time.Time
	gpus     []string // lowercase UUIDs of the last scan, sorted
	scanned  map[string]*gpuPeering
}

// NewPeeringScanner creates a scanner that rescans at most once per interval.
func NewPeeringScanner(interval time.Duration) *PeeringScanner {
	return &PeeringScanner{interval: interval}
}

// Scan returns the peering of each physical GPU, keyed by lowercase UUID. It returns the
// previous scan unless the interval elapsed or the GPU set changed.
func (s *PeeringScanner) Scan(now time.Time, devices []safenvml.Device) map[string]Peering {
	physical := physicalDevices(devices)
	uuids := make([]string, 0, len(physical))
	byBDF := make(map[string]string, len(physical))
	for _, device := range physical {
		uuid := strings.ToLower(device.UUID)
		uuids = append(uuids, uuid)
		if device.PCIBusID != "" {
			byBDF[device.PCIBusID] = uuid
		}
	}
	slices.Sort(uuids)

	if s.scanned == nil || !slices.Equal(uuids, s.gpus) || now.Sub(s.lastScan) >= s.interval {
		scanned := make(map[string]*gpuPeering, len(physical))
		for _, device := range physical {
			uuid := strings.ToLower(device.UUID)
			scanned[uuid] = scanGPU(device, physical, byBDF, s.scanned[uuid])
		}
		s.scanned, s.gpus, s.lastScan = scanned, uuids, now
	}

	result := make(map[string]Peering, len(s.scanned))
	for uuid, gpu := range s.scanned {
		result[uuid] = gpu.result
	}
	return result
}

// IncompletePeering returns the sorted UUIDs whose peering is incomplete or missing, for
// lifecycle handling that must tell absence from unknown.
func IncompletePeering(devices []safenvml.Device, peering map[string]Peering) []string {
	var incomplete []string
	for _, device := range physicalDevices(devices) {
		uuid := strings.ToLower(device.UUID)
		if p, ok := peering[uuid]; !ok || !p.Complete {
			incomplete = append(incomplete, uuid)
		}
	}
	slices.Sort(incomplete)
	return incomplete
}

func physicalDevices(devices []safenvml.Device) []*safenvml.PhysicalDevice {
	var physical []*safenvml.PhysicalDevice
	for _, device := range devices {
		if p, ok := device.(*safenvml.PhysicalDevice); ok && p.VirtualizationMode != nvml.GPU_VIRTUALIZATION_MODE_VGPU {
			physical = append(physical, p)
		}
	}
	return physical
}

// scanner state for one GPU scan
type gpuScan struct {
	previous *gpuPeering
	complete bool
}

// unknown records a query that gave no usable answer. A value known from the previous scan is
// kept, and the scan is incomplete because that value could not be re-confirmed. Without a
// previous value, an API that is simply not supported is a complete answer; any other failure
// is not.
func (g *gpuScan) unknown(err error, haveKnown bool) {
	if haveKnown || !safenvml.IsUnsupported(err) {
		g.complete = false
	}
}

func scanGPU(device *safenvml.PhysicalDevice, all []*safenvml.PhysicalDevice, byBDF map[string]string, previous *gpuPeering) *gpuPeering {
	if previous == nil {
		previous = &gpuPeering{}
	}
	g := &gpuScan{previous: previous, complete: true}
	gpu := &gpuPeering{ports: g.scanPorts(device), p2p: g.scanP2P(device, all)}
	gpu.result = buildPeering(strings.ToLower(device.UUID), gpu, byBDF)
	gpu.result.Complete = g.complete
	return gpu
}

func (g *gpuScan) scanPorts(device *safenvml.PhysicalDevice) map[int]nvlinkPort {
	ports := make(map[int]nvlinkPort)
	for link := range device.NVLinkLinkCount {
		prev, havePrev := g.previous.ports[link]
		deviceType, err := device.GetNvLinkRemoteDeviceType(link)
		if err != nil {
			g.unknown(err, havePrev)
			if havePrev {
				ports[link] = prev
			}
			continue
		}
		port := nvlinkPort{remoteType: nvlinkRemoteType(deviceType)}

		if info, err := device.GetNvLinkRemotePciInfo(link); err == nil {
			port.remoteBDF = gpuutil.PCIInfoToBusID(info)
		} else {
			// Keep a previously observed address only if the port still reports the same
			// kind of device; otherwise it is a different attachment.
			keep := havePrev && prev.remoteBDF != "" && prev.remoteType == port.remoteType
			g.unknown(err, keep)
			if keep {
				port.remoteBDF = prev.remoteBDF
			}
		}

		if version, err := device.GetNvLinkVersion(link); err == nil {
			port.version = safenvml.NVLinkVersionString(version)
		} else {
			keep := havePrev && prev.version != ""
			g.unknown(err, keep)
			if keep {
				port.version = prev.version
			}
		}
		ports[link] = port
	}

	// The cached link count could not be read this time, but ports were known before: keep
	// them rather than reporting that the links vanished.
	if device.NVLinkLinkCount == 0 && len(g.previous.ports) > 0 {
		g.complete = false
		maps.Copy(ports, g.previous.ports)
	}
	return ports
}

var p2pQueries = []struct {
	index nvml.GpuP2PCapsIndex
	field func(*P2PCapabilities) **bool
}{
	{nvml.P2P_CAPS_INDEX_READ, func(c *P2PCapabilities) **bool { return &c.Read }},
	{nvml.P2P_CAPS_INDEX_WRITE, func(c *P2PCapabilities) **bool { return &c.Write }},
	{nvml.P2P_CAPS_INDEX_ATOMICS, func(c *P2PCapabilities) **bool { return &c.Atomics }},
	{nvml.P2P_CAPS_INDEX_NVLINK, func(c *P2PCapabilities) **bool { return &c.NVLink }},
	{nvml.P2P_CAPS_INDEX_PCI, func(c *P2PCapabilities) **bool { return &c.PCI }},
}

func (g *gpuScan) scanP2P(device *safenvml.PhysicalDevice, all []*safenvml.PhysicalDevice) map[string]P2PCapabilities {
	self := strings.ToLower(device.UUID)
	result := make(map[string]P2PCapabilities)
	current := make(map[string]struct{}, len(all))
	for _, peer := range all {
		peerUUID := strings.ToLower(peer.UUID)
		current[peerUUID] = struct{}{}
		if peerUUID == self {
			continue
		}
		var caps P2PCapabilities
		prev := g.previous.p2p[peerUUID]
		for _, query := range p2pQueries {
			known := *query.field(&prev)
			status, err := device.GetP2PStatus(peer, query.index)
			value := p2pStatusToBool(status)
			if err == nil && value == nil {
				// NVML answered but the status says nothing (unknown or a value newer than
				// this Agent): not a negative.
				err = errUninterpretableP2PStatus
			}
			if err != nil {
				g.unknown(err, known != nil)
				value = known
			}
			*query.field(&caps) = value
		}
		if !isZero(caps) {
			result[peerUUID] = caps
		}
	}

	// Facts about GPUs that are no longer enumerated are dropped with them, but the scan can
	// no longer be called complete.
	for peerUUID := range g.previous.p2p {
		if _, ok := current[peerUUID]; !ok {
			g.complete = false
		}
	}
	return result
}

// buildPeering places every observed port in exactly one array: ports whose remote GPU
// resolves by BDF to another GPU of this host become direct links of that peer, the rest are
// unresolved. A peer is published only when directly attached or when at least one P2P
// capability towards it is supported.
func buildPeering(self string, gpu *gpuPeering, byBDF map[string]string) Peering {
	peers := make(map[string]*GPUPeer)
	peer := func(uuid string) *GPUPeer {
		if peers[uuid] == nil {
			peers[uuid] = &GPUPeer{UUID: uuid}
		}
		return peers[uuid]
	}

	var result Peering
	for _, link := range slices.Sorted(maps.Keys(gpu.ports)) {
		port := gpu.ports[link]
		if peerUUID, ok := byBDF[port.remoteBDF]; ok && port.remoteType == nvlinkRemoteGPU && peerUUID != self {
			p := peer(peerUUID)
			p.DirectNVLinks = append(p.DirectNVLinks, DirectNVLink{
				Port: int32(link), RemotePCIBusID: port.remoteBDF, Version: port.version,
			})
			continue
		}
		result.UnresolvedNVLinks = append(result.UnresolvedNVLinks, UnresolvedNVLink{
			Port: int32(link), RemoteType: port.remoteType, RemotePCIBusID: port.remoteBDF, Version: port.version,
		})
	}
	for uuid, caps := range gpu.p2p {
		if peers[uuid] == nil && !anyTrue(caps) {
			continue
		}
		c := caps
		peer(uuid).P2PCapabilities = &c
	}
	for _, uuid := range slices.Sorted(maps.Keys(peers)) {
		result.GPUPeers = append(result.GPUPeers, *peers[uuid])
	}
	return result
}

const (
	nvlinkRemoteGPU     = "gpu"
	nvlinkRemoteSwitch  = "switch"
	nvlinkRemoteOther   = "other"
	nvlinkRemoteUnknown = "unknown"
)

func nvlinkRemoteType(deviceType nvml.IntNvLinkDeviceType) string {
	switch deviceType {
	case nvml.NVLINK_DEVICE_TYPE_GPU:
		return nvlinkRemoteGPU
	case nvml.NVLINK_DEVICE_TYPE_SWITCH:
		return nvlinkRemoteSwitch
	case nvml.NVLINK_DEVICE_TYPE_IBMNPU:
		return nvlinkRemoteOther
	default:
		return nvlinkRemoteUnknown
	}
}

// p2pStatusToBool maps NVML's P2P status to the published tri-state: OK is true, the explicit
// unsupported/disabled statuses are false, and anything else is nil (unknown).
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

var errUninterpretableP2PStatus = safenvml.NewNvmlAPIErrorOrNil("GetP2PStatus", nvml.ERROR_UNKNOWN)

func isZero(c P2PCapabilities) bool {
	return c.Read == nil && c.Write == nil && c.Atomics == nil && c.NVLink == nil && c.PCI == nil
}

func anyTrue(c P2PCapabilities) bool {
	for _, v := range []*bool{c.Read, c.Write, c.Atomics, c.NVLink, c.PCI} {
		if v != nil && *v {
			return true
		}
	}
	return false
}
