// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

// Package iisconfig manages iis configuration
package wlanapi

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetWlanInterface(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping test on non-windows platform")
	}

	h, err := OpenWLANHandle()
	assert.Nil(t, err)
	assert.NotNil(t, h)

	defer h.Close()

	ifaces, err := h.EnumNetworks()
	assert.Nil(t, err)
	assert.NotEmpty(t, ifaces)
	for _, iface := range ifaces {
		t.Logf("Found interface: %s", iface.InterfaceDescription)
		for _, network := range iface.Networks {
			t.Log("- - - - - - - - - - - - - - - - -")
			t.Logf("Found network SSID: %s", network.SSID)
			t.Logf("Found network ID  : %s", network.NetworkID)
			t.Logf("Found Signal Quality: %d", network.SignalStrength)
			t.Logf("connected %v connectable %v profile %v reason %v", network.Connected, network.Connectable, network.HasProfile, network.NotConnectableReason)
			
		}

	}
}	