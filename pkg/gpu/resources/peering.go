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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Peering is the NVLink and P2P peering of one GPU, in the shape published in gpu_device.
type Peering struct {
	GPUPeers          []GPUPeer
	UnresolvedNVLinks []UnresolvedNVLink
}

// nvlinkPort is what NVML reports for one local NVLink port.
type nvlinkPort struct {
	link       int
	remoteType string // gpu, switch, other or unknown, as in the contract
	remoteBDF  string // normalized; a locator, never an identity
	version    string
}

// PeeringScanner scans NVLink ports and pairwise P2P capabilities of the host's physical GPUs.
// Peering is static, so the scan is O(ports*GPUs + GPUs²) NVML calls that run at most once per
// interval, or when the set of GPUs changes, never on every collection.
type PeeringScanner struct {
	interval time.Duration
	lastScan time.Time
	scanned  map[string]Peering // by lowercase UUID; one entry per GPU of the last scan
}

// NewPeeringScanner creates a scanner that rescans at most once per interval.
func NewPeeringScanner(interval time.Duration) *PeeringScanner {
	return &PeeringScanner{interval: interval}
}

// Scan returns the peering of each physical GPU, keyed by lowercase UUID. It returns the
// previous scan unless the interval elapsed or the GPU set changed.
func (s *PeeringScanner) Scan(now time.Time, devices []safenvml.Device) map[string]Peering {
	physical := physicalDevices(devices)
	byBDF := make(map[string]string, len(physical))
	sameGPUs := s.scanned != nil && len(s.scanned) == len(physical)
	for _, device := range physical {
		uuid := strings.ToLower(device.UUID)
		if device.PCIBusID != "" {
			byBDF[device.PCIBusID] = uuid
		}
		if _, ok := s.scanned[uuid]; !ok {
			sameGPUs = false
		}
	}

	if !sameGPUs || now.Sub(s.lastScan) >= s.interval {
		scanned := make(map[string]Peering, len(physical))
		var failed []error
		for _, device := range physical {
			uuid := strings.ToLower(device.UUID)
			ports, portErrs := scanPorts(device)
			p2p, p2pErrs := scanP2P(device, physical)
			scanned[uuid] = buildPeering(uuid, ports, p2p, byBDF)
			failed = append(append(failed, portErrs...), p2pErrs...)
		}
		s.scanned, s.lastScan = scanned, now
		if len(failed) > 0 && peeringLogLimit.ShouldLog() {
			log.Warnf("GPU NVLink/P2P peering scan: %d NVML queries failed, their fields are not published until the next scan; first error: %v", len(failed), failed[0])
		}
	}
	return maps.Clone(s.scanned)
}

var peeringLogLimit = log.NewLogLimit(10, 10*time.Minute)

func physicalDevices(devices []safenvml.Device) []*safenvml.PhysicalDevice {
	var physical []*safenvml.PhysicalDevice
	for _, device := range devices {
		if p, ok := device.(*safenvml.PhysicalDevice); ok && p.VirtualizationMode != nvml.GPU_VIRTUALIZATION_MODE_VGPU {
			physical = append(physical, p)
		}
	}
	return physical
}

// failure keeps errors other than "not supported", which is a valid answer meaning unknown.
func failure(errs []error, err error) []error {
	if err != nil && !safenvml.IsUnsupported(err) {
		return append(errs, err)
	}
	return errs
}

// scanPorts queries each local NVLink port. A port whose remote type cannot be read is not
// reported; a remote address or version that cannot be read is left empty.
func scanPorts(device *safenvml.PhysicalDevice) ([]nvlinkPort, []error) {
	var ports []nvlinkPort
	var errs []error
	for link := range device.NVLinkLinkCount {
		deviceType, err := device.GetNvLinkRemoteDeviceType(link)
		if err != nil {
			errs = failure(errs, err)
			continue
		}
		port := nvlinkPort{link: link, remoteType: nvlinkRemoteType(deviceType)}
		if info, err := device.GetNvLinkRemotePciInfo(link); err == nil {
			port.remoteBDF = gpuutil.PCIInfoToBusID(info)
		} else {
			errs = failure(errs, err)
		}
		if version, err := device.GetNvLinkVersion(link); err == nil {
			port.version = safenvml.NVLinkVersionString(version)
		} else {
			errs = failure(errs, err)
		}
		ports = append(ports, port)
	}
	return ports, errs
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

// scanP2P queries the capabilities from device towards every other physical GPU, keyed by
// lowercase peer UUID. A capability that cannot be read or interpreted is left unknown.
func scanP2P(device *safenvml.PhysicalDevice, all []*safenvml.PhysicalDevice) (map[string]P2PCapabilities, []error) {
	self := strings.ToLower(device.UUID)
	result := make(map[string]P2PCapabilities)
	var errs []error
	for _, peer := range all {
		peerUUID := strings.ToLower(peer.UUID)
		if peerUUID == self {
			continue
		}
		var caps P2PCapabilities
		for _, query := range p2pQueries {
			status, err := device.GetP2PStatus(peer, query.index)
			if err != nil {
				errs = failure(errs, err)
				continue
			}
			*query.field(&caps) = p2pStatusToBool(status)
		}
		if !isZero(caps) {
			result[peerUUID] = caps
		}
	}
	return result, errs
}

// buildPeering places every observed port in exactly one array: ports whose remote GPU
// resolves by BDF to another GPU of this host become direct links of that peer, the rest are
// unresolved. A peer is published only when directly attached or when at least one P2P
// capability towards it is supported.
func buildPeering(self string, ports []nvlinkPort, p2p map[string]P2PCapabilities, byBDF map[string]string) Peering {
	peers := make(map[string]*GPUPeer)
	peer := func(uuid string) *GPUPeer {
		if peers[uuid] == nil {
			peers[uuid] = &GPUPeer{UUID: uuid}
		}
		return peers[uuid]
	}

	var result Peering
	for _, port := range ports {
		if peerUUID, ok := byBDF[port.remoteBDF]; ok && port.remoteType == nvlinkRemoteGPU && peerUUID != self {
			p := peer(peerUUID)
			p.DirectNVLinks = append(p.DirectNVLinks, DirectNVLink{
				Port: int32(port.link), RemotePCIBusID: port.remoteBDF, Version: port.version,
			})
			continue
		}
		result.UnresolvedNVLinks = append(result.UnresolvedNVLinks, UnresolvedNVLink{
			Port: int32(port.link), RemoteType: port.remoteType, RemotePCIBusID: port.remoteBDF, Version: port.version,
		})
	}
	for uuid, caps := range p2p {
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
