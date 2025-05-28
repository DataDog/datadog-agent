package report

import (
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

type vpnTunnelStore struct {
	ByOutsideIPs      map[string]*devicemetadata.VPNTunnelMetadata
	ByRemoteOutsideIP map[string]*devicemetadata.VPNTunnelMetadata
}

type deviceRoute struct {
	Destination string
	PrefixLen   int
	NextHopIP   string
	IfIndex     string
}

func newVPNTunnelStore() vpnTunnelStore {
	return vpnTunnelStore{
		ByOutsideIPs:      make(map[string]*devicemetadata.VPNTunnelMetadata),
		ByRemoteOutsideIP: make(map[string]*devicemetadata.VPNTunnelMetadata),
	}
}

func (vts *vpnTunnelStore) AddTunnel(vpnTunnel devicemetadata.VPNTunnelMetadata) {
	if vts.ByOutsideIPs == nil || vts.ByRemoteOutsideIP == nil {
		return
	}

	vts.ByOutsideIPs[buildOutsideIPsKey(vpnTunnel.LocalOutsideIP, vpnTunnel.RemoteOutsideIP)] = &vpnTunnel
	vts.ByRemoteOutsideIP[vpnTunnel.RemoteOutsideIP] = &vpnTunnel
}

func (vts *vpnTunnelStore) GetTunnelByOutsideIPs(localOutsideIP string, remoteOutsideIP string) (*devicemetadata.VPNTunnelMetadata, bool) {
	vpnTunnel, exists := vts.ByOutsideIPs[buildOutsideIPsKey(localOutsideIP, remoteOutsideIP)]
	return vpnTunnel, exists
}

func (vts *vpnTunnelStore) GetTunnelByRemoteOutsideIP(remoteOutsideIP string) (*devicemetadata.VPNTunnelMetadata, bool) {
	vpnTunnel, exists := vts.ByRemoteOutsideIP[remoteOutsideIP]
	return vpnTunnel, exists
}

func (vts *vpnTunnelStore) ToSlice() []devicemetadata.VPNTunnelMetadata {
	vpnTunnels := make([]devicemetadata.VPNTunnelMetadata, 0, len(vts.ByOutsideIPs))
	for _, vpnTunnel := range vts.ByOutsideIPs {
		vpnTunnels = append(vpnTunnels, *vpnTunnel)
	}
	return vpnTunnels
}

func buildOutsideIPsKey(localOutsideIP string, remoteOutsideIP string) string {
	return localOutsideIP + remoteOutsideIP
}
