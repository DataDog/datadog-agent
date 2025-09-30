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

	allVPNTunnels := []devicemetadata.VPNTunnelMetadata{
		{
			DeviceID:        "device1",
			InterfaceID:     "device1:4",
			LocalOutsideIP:  "13.14.15.16",
			RemoteOutsideIP: "12.11.10.9",
			Protocol:        "ipsec",
			RouteAddresses:  []string{"10.0.0.6/24"},
		},
		{
			DeviceID:        "device1",
			InterfaceID:     "device1:1",
			LocalOutsideIP:  "1.2.3.4",
			RemoteOutsideIP: "4.3.2.1",
			Protocol:        "ipsec",
			RouteAddresses:  []string{"20.0.0.0/16", "10.0.0.0/24", "10.0.0.1/24"},
		},
		{
			DeviceID:        "device1",
			InterfaceID:     "device1:3",
			LocalOutsideIP:  "9.10.11.12",
			RemoteOutsideIP: "12.11.10.9",
			Protocol:        "ipsec",
			RouteAddresses:  []string{"10.0.0.4/24", "10.0.0.5/24"},
		},
		{
			DeviceID:        "device1",
			InterfaceID:     "device1:2",
			LocalOutsideIP:  "5.6.7.8",
			RemoteOutsideIP: "8.7.6.5",
			Protocol:        "ipsec",
			RouteAddresses:  []string{"10.0.0.2/24", "30.0.0.0/24", "10.0.0.3/24"},
		},
	}

	_, exists := vts.GetTunnelByOutsideIPs("1.2.3.4", "4.3.2.1")
	assert.False(t, exists)

	for _, vpnTunnel := range allVPNTunnels {
		vts.AddTunnel(vpnTunnel)
	}
	assert.Len(t, vts.ByOutsideIPs, len(allVPNTunnels))
	assert.Len(t, vts.ByRemoteOutsideIP, 3)

	vpnTunnel, exists := vts.GetTunnelByOutsideIPs("1.2.3.4", "4.3.2.1")
	assert.True(t, exists)
	assert.Equal(t, allVPNTunnels[1], *vpnTunnel)
	vpnTunnel, exists = vts.GetTunnelByOutsideIPs("5.6.7.8", "8.7.6.5")
	assert.True(t, exists)
	assert.Equal(t, allVPNTunnels[3], *vpnTunnel)

	_, exists = vts.GetTunnelByOutsideIPs("4.3.2.1", "1.2.3.4")
	assert.False(t, exists)
	_, exists = vts.GetTunnelByOutsideIPs("", "")
	assert.False(t, exists)

	vpnTunnels, exists := vts.GetTunnelsByRemoteOutsideIP("4.3.2.1")
	assert.True(t, exists)
	assert.Len(t, vpnTunnels, 1)
	assert.Equal(t, *vpnTunnels[0], allVPNTunnels[1])
	vpnTunnels, exists = vts.GetTunnelsByRemoteOutsideIP("12.11.10.9")
	assert.True(t, exists)
	assert.Len(t, vpnTunnels, 2)
	assert.Equal(t, *vpnTunnels[0], allVPNTunnels[0])
	assert.Equal(t, *vpnTunnels[1], allVPNTunnels[2])

	_, exists = vts.GetTunnelsByRemoteOutsideIP("1.2.3.4")
	assert.False(t, exists)
	_, exists = vts.GetTunnelsByRemoteOutsideIP("")
	assert.False(t, exists)

	vtsSlice := vts.ToNormalizedSortedSlice()
	assert.Equal(t, []devicemetadata.VPNTunnelMetadata{
		{
			DeviceID:        "device1",
			InterfaceID:     "device1:1",
			LocalOutsideIP:  "1.2.3.4",
			RemoteOutsideIP: "4.3.2.1",
			Protocol:        "ipsec",
			RouteAddresses:  []string{"10.0.0.0/24", "10.0.0.1/24", "20.0.0.0/16"},
		},
		{
			DeviceID:        "device1",
			InterfaceID:     "device1:4",
			LocalOutsideIP:  "13.14.15.16",
			RemoteOutsideIP: "12.11.10.9",
			Protocol:        "ipsec",
			RouteAddresses:  []string{"10.0.0.6/24"},
		},
		{
			DeviceID:        "device1",
			InterfaceID:     "device1:2",
			LocalOutsideIP:  "5.6.7.8",
			RemoteOutsideIP: "8.7.6.5",
			Protocol:        "ipsec",
			RouteAddresses:  []string{"10.0.0.2/24", "10.0.0.3/24", "30.0.0.0/24"},
		},
		{
			DeviceID:        "device1",
			InterfaceID:     "device1:3",
			LocalOutsideIP:  "9.10.11.12",
			RemoteOutsideIP: "12.11.10.9",
			Protocol:        "ipsec",
			RouteAddresses:  []string{"10.0.0.4/24", "10.0.0.5/24"},
		},
	}, vtsSlice)
}
