// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package report

import (
	"github.com/stretchr/testify/assert"
	"testing"

	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

func TestVPNTunnelStore(t *testing.T) {
	vts := NewVPNTunnelStore()
	assert.NotNil(t, vts)
	assert.NotNil(t, vts.ByOutsideIPs)
	assert.NotNil(t, vts.ByRemoteOutsideIP)

	vpnTunnels := []devicemetadata.VPNTunnelMetadata{
		{
			DeviceID:        "device1",
			InterfaceID:     "device1:1",
			LocalOutsideIP:  "1.2.3.4",
			RemoteOutsideIP: "4.3.2.1",
			Protocol:        "ipsec",
			RouteAddresses:  []string{"10.0.0.0/24", "10.0.0.1/24"},
		},
		{
			DeviceID:        "device1",
			InterfaceID:     "device1:2",
			LocalOutsideIP:  "5.6.7.8",
			RemoteOutsideIP: "8.7.6.5",
			Protocol:        "ipsec",
			RouteAddresses:  []string{"10.0.0.2/24", "10.0.0.3/24"},
		},
		{
			DeviceID:        "device1",
			InterfaceID:     "device1:3",
			LocalOutsideIP:  "9.10.11.12",
			RemoteOutsideIP: "12.11.10.9",
			Protocol:        "ipsec",
			RouteAddresses:  []string{"10.0.0.4/24", "10.0.0.5/24"},
		},
	}

	vpnTunnel, exists := vts.GetTunnelByOutsideIPs("1.2.3.4", "4.3.2.1")
	assert.False(t, exists)

	for _, vpnTunnel := range vpnTunnels {
		vts.AddTunnel(vpnTunnel)
	}
	assert.Len(t, vts.ByOutsideIPs, len(vpnTunnels))
	assert.Len(t, vts.ByRemoteOutsideIP, len(vpnTunnels))

	vpnTunnel, exists = vts.GetTunnelByOutsideIPs("1.2.3.4", "4.3.2.1")
	assert.True(t, exists)
	assert.Equal(t, vpnTunnels[0], *vpnTunnel)
	vpnTunnel, exists = vts.GetTunnelByOutsideIPs("9.10.11.12", "12.11.10.9")
	assert.True(t, exists)
	assert.Equal(t, vpnTunnels[2], *vpnTunnel)

	_, exists = vts.GetTunnelByOutsideIPs("", "")
	assert.False(t, exists)
	_, exists = vts.GetTunnelByOutsideIPs("4.3.2.1", "1.2.3.4")
	assert.False(t, exists)

	vpnTunnel, exists = vts.GetTunnelByRemoteOutsideIP("4.3.2.1")
	assert.True(t, exists)
	assert.Equal(t, vpnTunnels[0], *vpnTunnel)
	vpnTunnel, exists = vts.GetTunnelByRemoteOutsideIP("8.7.6.5")
	assert.True(t, exists)
	assert.Equal(t, vpnTunnels[1], *vpnTunnel)

	_, exists = vts.GetTunnelByRemoteOutsideIP("1.2.3.4")
	assert.False(t, exists)
	_, exists = vts.GetTunnelByRemoteOutsideIP("")
	assert.False(t, exists)

	vtsSlice := vts.ToSlice()
	assert.Len(t, vtsSlice, len(vpnTunnels))
	for _, vpnTunnel := range vpnTunnels {
		assert.Contains(t, vtsSlice, vpnTunnel)
	}
}
