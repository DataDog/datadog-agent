// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package report

import (
	"sort"

	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

// VPNTunnelStore stores VPN tunnel metadata indexed by outside IPs
type VPNTunnelStore struct {
	ByOutsideIPs      map[string]*devicemetadata.VPNTunnelMetadata
	ByRemoteOutsideIP map[string][]*devicemetadata.VPNTunnelMetadata
}

// DeviceRoute represents a route on a network device
type DeviceRoute struct {
	Destination string
	PrefixLen   int
	NextHopIP   string
	IfIndex     string
}

// RoutesByIfIndex stores routes indexed by interface index
type RoutesByIfIndex map[string][]DeviceRoute

// DeviceTunnel represents a tunnel on a network device
type DeviceTunnel struct {
	LocalIP  string
	RemoteIP string
	IfIndex  string
}

// NewVPNTunnelStore creates a new VPNTunnelStore
func NewVPNTunnelStore() VPNTunnelStore {
	return VPNTunnelStore{
		ByOutsideIPs:      make(map[string]*devicemetadata.VPNTunnelMetadata),
		ByRemoteOutsideIP: make(map[string][]*devicemetadata.VPNTunnelMetadata),
	}
}

// AddTunnel adds a VPN tunnel to the VPNTunnelStore
func (vts *VPNTunnelStore) AddTunnel(vpnTunnel devicemetadata.VPNTunnelMetadata) {
	if vts.ByOutsideIPs == nil || vts.ByRemoteOutsideIP == nil {
		return
	}

	vts.ByOutsideIPs[buildOutsideIPsKey(vpnTunnel.LocalOutsideIP, vpnTunnel.RemoteOutsideIP)] = &vpnTunnel
	vts.ByRemoteOutsideIP[vpnTunnel.RemoteOutsideIP] = append(vts.ByRemoteOutsideIP[vpnTunnel.RemoteOutsideIP], &vpnTunnel)
}

// GetTunnelByOutsideIPs retrieves a VPN tunnel by its local and remote outside IPs
func (vts *VPNTunnelStore) GetTunnelByOutsideIPs(localOutsideIP string, remoteOutsideIP string) (*devicemetadata.VPNTunnelMetadata, bool) {
	vpnTunnel, exists := vts.ByOutsideIPs[buildOutsideIPsKey(localOutsideIP, remoteOutsideIP)]
	return vpnTunnel, exists
}

// GetTunnelsByRemoteOutsideIP retrieves VPN tunnels by their remote outside IP
func (vts *VPNTunnelStore) GetTunnelsByRemoteOutsideIP(remoteOutsideIP string) ([]*devicemetadata.VPNTunnelMetadata, bool) {
	vpnTunnels, exists := vts.ByRemoteOutsideIP[remoteOutsideIP]
	return vpnTunnels, exists
}

// ToNormalizedSortedSlice converts the VPNTunnelStore to a sorted slice (by outside IPs) of normalized VPNTunnelMetadata
func (vts *VPNTunnelStore) ToNormalizedSortedSlice() []devicemetadata.VPNTunnelMetadata {
	vpnTunnels := make([]devicemetadata.VPNTunnelMetadata, 0, len(vts.ByOutsideIPs))
	for _, vpnTunnel := range vts.ByOutsideIPs {
		normalizedVPNTunnel := normalizeVPNTunnel(*vpnTunnel)
		vpnTunnels = append(vpnTunnels, normalizedVPNTunnel)
	}
	sort.Slice(vpnTunnels, func(i1, i2 int) bool {
		v1 := buildOutsideIPsKey(vpnTunnels[i1].LocalOutsideIP, vpnTunnels[i1].RemoteOutsideIP)
		v2 := buildOutsideIPsKey(vpnTunnels[i2].LocalOutsideIP, vpnTunnels[i2].RemoteOutsideIP)
		return v1 < v2
	})
	return vpnTunnels
}

func buildOutsideIPsKey(localOutsideIP string, remoteOutsideIP string) string {
	return localOutsideIP + remoteOutsideIP
}

func normalizeVPNTunnel(vpnTunnel devicemetadata.VPNTunnelMetadata) devicemetadata.VPNTunnelMetadata {
	sort.Strings(vpnTunnel.RouteAddresses)

	return vpnTunnel
}
