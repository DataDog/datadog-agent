// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package tracer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchVPNPattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"cisco anyconnect", "cisco anyconnect virtual miniport adapter", "Cisco AnyConnect"},
		{"globalprotect pangp", "pangp virtual ethernet adapter", "GlobalProtect"},
		{"wireguard wintun", "wintun userspace tunnel", "WireGuard"},
		{"openvpn tap", "tap-windows adapter v9", "OpenVPN"},
		{"fortinet", "fortinet virtual ethernet adapter", "FortiClient"},
		{"zscaler", "zscaler network adapter", "Zscaler"},
		{"no match", "intel(r) wi-fi 6 ax201 160mhz", ""},
		{"ethernet adapter", "realtek pcie gbe family controller", ""},
		{"hyper-v", "hyper-v virtual ethernet adapter", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchVPNPattern(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIfTypeName(t *testing.T) {
	assert.Equal(t, "ethernet", ifTypeName(6))
	assert.Equal(t, "ppp", ifTypeName(23))
	assert.Equal(t, "loopback", ifTypeName(24))
	assert.Equal(t, "prop_virtual", ifTypeName(53))
	assert.Equal(t, "wifi", ifTypeName(71))
	assert.Equal(t, "tunnel", ifTypeName(131))
	assert.Equal(t, "other_99", ifTypeName(99))
}

func TestClassify_PPP(t *testing.T) {
	c := &VPNClassifier{
		ifCache: map[uint32]cachedInterface{
			10: {ifType: ifTypePPP, name: "WAN Miniport (L2TP)", descr: "WAN Miniport (L2TP)", physAddrLen: 0},
		},
		done: make(chan struct{}),
	}
	defer c.Close()

	result := c.Classify(10)
	// Interface tags
	assert.Equal(t, "WAN Miniport (L2TP)", result.InterfaceName)
	assert.Equal(t, "ppp", result.InterfaceType)
	assert.False(t, result.IsPhysical)
	// VPN tags
	assert.True(t, result.IsVPN)
	assert.Equal(t, "Windows VPN", result.VPNName)
	assert.Equal(t, "ppp", result.VPNType)
}

func TestClassify_PropVirtual_VPN(t *testing.T) {
	c := &VPNClassifier{
		ifCache: map[uint32]cachedInterface{
			20: {ifType: ifTypePropVirtual, name: "PANGP Virtual Ethernet Adapter", descr: "Palo Alto Networks GlobalProtect", physAddrLen: 0},
		},
		done: make(chan struct{}),
	}
	defer c.Close()

	result := c.Classify(20)
	assert.Equal(t, "Palo Alto Networks GlobalProtect", result.InterfaceName)
	assert.Equal(t, "prop_virtual", result.InterfaceType)
	assert.False(t, result.IsPhysical)
	assert.True(t, result.IsVPN)
	assert.Equal(t, "GlobalProtect", result.VPNName)
	assert.Equal(t, "prop_virtual", result.VPNType)
}

func TestClassify_PropVirtual_NotVPN(t *testing.T) {
	c := &VPNClassifier{
		ifCache: map[uint32]cachedInterface{
			30: {ifType: ifTypePropVirtual, name: "Hyper-V Virtual Ethernet Adapter", descr: "Hyper-V Virtual Ethernet Adapter", physAddrLen: 6},
		},
		done: make(chan struct{}),
	}
	defer c.Close()

	result := c.Classify(30)
	// Interface tags should still be populated
	assert.Equal(t, "Hyper-V Virtual Ethernet Adapter", result.InterfaceName)
	assert.Equal(t, "prop_virtual", result.InterfaceType)
	assert.True(t, result.IsPhysical) // Hyper-V adapters have MAC addresses
	// VPN should not be set
	assert.False(t, result.IsVPN)
}

func TestClassify_Ethernet_Physical(t *testing.T) {
	c := &VPNClassifier{
		ifCache: map[uint32]cachedInterface{
			40: {ifType: ifTypeEthernetCSMACD, name: "Intel(R) Ethernet Connection", descr: "Intel Ethernet", physAddrLen: 6},
		},
		done: make(chan struct{}),
	}
	defer c.Close()

	result := c.Classify(40)
	assert.Equal(t, "Intel Ethernet", result.InterfaceName)
	assert.Equal(t, "ethernet", result.InterfaceType)
	assert.True(t, result.IsPhysical)
	assert.False(t, result.IsVPN)
}

func TestClassify_Wifi(t *testing.T) {
	c := &VPNClassifier{
		ifCache: map[uint32]cachedInterface{
			45: {ifType: ifTypeWifi, name: "Intel(R) Wi-Fi 6 AX201", descr: "Intel Wi-Fi 6 AX201", physAddrLen: 6},
		},
		done: make(chan struct{}),
	}
	defer c.Close()

	result := c.Classify(45)
	assert.Equal(t, "Intel Wi-Fi 6 AX201", result.InterfaceName)
	assert.Equal(t, "wifi", result.InterfaceType)
	assert.True(t, result.IsPhysical)
	assert.False(t, result.IsVPN)
}

func TestClassify_Ethernet_TapVPN(t *testing.T) {
	c := &VPNClassifier{
		ifCache: map[uint32]cachedInterface{
			50: {ifType: ifTypeEthernetCSMACD, name: "TAP-Windows Adapter V9", descr: "TAP-Windows Adapter V9", physAddrLen: 6},
		},
		done: make(chan struct{}),
	}
	defer c.Close()

	result := c.Classify(50)
	assert.Equal(t, "TAP-Windows Adapter V9", result.InterfaceName)
	assert.Equal(t, "ethernet", result.InterfaceType)
	assert.True(t, result.IsPhysical) // TAP adapters have MAC addresses
	assert.True(t, result.IsVPN)
	assert.Equal(t, "OpenVPN", result.VPNName)
	assert.Equal(t, "ethernet_tap", result.VPNType)
}

func TestClassify_UnknownIndex(t *testing.T) {
	c := &VPNClassifier{
		ifCache: map[uint32]cachedInterface{},
		done:    make(chan struct{}),
	}
	defer c.Close()

	result := c.Classify(999)
	assert.Equal(t, "", result.InterfaceName)
	assert.False(t, result.IsVPN)
}

func TestClassify_ZeroIndex(t *testing.T) {
	c := &VPNClassifier{
		ifCache: map[uint32]cachedInterface{
			0: {ifType: ifTypePPP, name: "WAN Miniport", descr: "", physAddrLen: 0},
		},
		done: make(chan struct{}),
	}
	defer c.Close()

	// Zero index should still classify if present in cache
	// (the caller in addInterfaceInfo guards against zero)
	result := c.Classify(0)
	assert.True(t, result.IsVPN)
}

func TestClassify_Wintun(t *testing.T) {
	c := &VPNClassifier{
		ifCache: map[uint32]cachedInterface{
			60: {ifType: ifTypePropVirtual, name: "Wintun Userspace Tunnel", descr: "Wintun", physAddrLen: 0},
		},
		done: make(chan struct{}),
	}
	defer c.Close()

	result := c.Classify(60)
	assert.Equal(t, "Wintun", result.InterfaceName)
	assert.Equal(t, "prop_virtual", result.InterfaceType)
	assert.False(t, result.IsPhysical)
	assert.True(t, result.IsVPN)
	assert.Equal(t, "WireGuard", result.VPNName)
	assert.Equal(t, "prop_virtual", result.VPNType)
}

func TestClassify_CiscoAnyConnect(t *testing.T) {
	c := &VPNClassifier{
		ifCache: map[uint32]cachedInterface{
			70: {ifType: ifTypePropVirtual, name: "Cisco AnyConnect Virtual Miniport Adapter", descr: "Cisco AnyConnect", physAddrLen: 0},
		},
		done: make(chan struct{}),
	}
	defer c.Close()

	result := c.Classify(70)
	assert.True(t, result.IsVPN)
	assert.Equal(t, "Cisco AnyConnect", result.VPNName)
	assert.Equal(t, "prop_virtual", result.VPNType)
}

func TestClassify_AppgateSDP(t *testing.T) {
	// Real-world adapter: Appgate SDP uses Wintun driver under the hood
	c := &VPNClassifier{
		ifCache: map[uint32]cachedInterface{
			17: {ifType: ifTypePropVirtual, name: "Appgate Tunnel", descr: "Wintun Userspace Tunnel", physAddrLen: 0},
		},
		done: make(chan struct{}),
	}
	defer c.Close()

	result := c.Classify(17)
	assert.True(t, result.IsVPN)
	assert.Equal(t, "Appgate SDP", result.VPNName, "should match 'appgate' before falling through to 'wintun'")
	assert.Equal(t, "prop_virtual", result.VPNType)
	assert.Equal(t, "Appgate Tunnel", result.InterfaceName)
}

func TestClassify_GenericTunIndicator(t *testing.T) {
	// Ethernet adapter with "tun" in name but no known VPN pattern
	c := &VPNClassifier{
		ifCache: map[uint32]cachedInterface{
			80: {ifType: ifTypeEthernetCSMACD, name: "Some tun adapter", descr: "Generic tun", physAddrLen: 0},
		},
		done: make(chan struct{}),
	}
	defer c.Close()

	result := c.Classify(80)
	assert.True(t, result.IsVPN)
	assert.Equal(t, "Unknown VPN", result.VPNName)
	assert.Equal(t, "ethernet_tap", result.VPNType)
}
