// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows || darwin

package networkv2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	inventorychecksmock "github.com/DataDog/datadog-agent/comp/metadata/inventorychecks/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

// TestSubmitIfaceMetrics tests the gauges and tags of interfaces in each link state.
func TestSubmitIfaceMetrics(t *testing.T) {
	mockSender := mocksender.NewMockSender(t, "network")
	mockSender.SetupAcceptAll()

	submitIfaceMetrics(mockSender, ifaceInfo{name: "eth", ifaceType: "ethernet", mtu: 9000, link: linkStateUp, speedMbps: 2500, duplex: "full", vlanID: 42})
	submitIfaceMetrics(mockSender, ifaceInfo{name: "unplugged", ifaceType: "ethernet", mtu: 1500, link: linkStateDown, speedMbps: 1000})
	submitIfaceMetrics(mockSender, ifaceInfo{name: "unknown", ifaceType: "wifi", link: linkStateUnknown})
	submitIfaceMetrics(mockSender, ifaceInfo{name: "wifi", ifaceType: "wifi", mtu: 1500, link: linkStateUp, speedMbps: 866.7})

	ethTags := []string{"iface:eth", "iface_type:ethernet", "duplex:full", "vlan_id:42"}
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.mtu", float64(9000), "", ethTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.up", float64(1), "", ethTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.speed", float64(2500), "", ethTags)

	unpluggedTags := []string{"iface:unplugged", "iface_type:ethernet"}
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.up", float64(0), "", unpluggedTags)
	mockSender.AssertNotCalled(t, "Gauge", "system.net.iface.speed", mock.Anything, "", unpluggedTags)

	unknownTags := []string{"iface:unknown", "iface_type:wifi"}
	mockSender.AssertNotCalled(t, "Gauge", "system.net.iface.mtu", mock.Anything, "", unknownTags)
	mockSender.AssertNotCalled(t, "Gauge", "system.net.iface.up", mock.Anything, "", unknownTags)

	wifiTags := []string{"iface:wifi", "iface_type:wifi"}
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.up", float64(1), "", wifiTags)
	mockSender.AssertNotCalled(t, "Gauge", "system.net.iface.speed", mock.Anything, "", wifiTags)
}

// TestSetInterfacesMetadata tests the JSON of the "interfaces" check metadata.
func TestSetInterfacesMetadata(t *testing.T) {
	networkCheck := &NetworkCheck{CheckBase: core.NewCheckBase(CheckName)}
	ifaces := []ifaceInfo{
		{name: "wifi", ifaceType: "wifi", macAddress: "9a:62:9c:bd:37:0a", permanentMacAddress: "bc:d0:74:1e:bd:b7", mtu: 1500, link: linkStateUp, speedMbps: 866.7},
		{name: "eth", description: "USB Ethernet", ifaceType: "ethernet", macAddress: "42:52:1c:e5:c2:ec", permanentMacAddress: "42:52:1c:e5:c2:ec", mtu: 1500, duplex: "full", vlanID: 10, driverProvider: "Realtek", driverVersion: "1.0"},
	}

	// Without an inventory context, the metadata is dropped
	networkCheck.setInterfacesMetadata(ifaces)

	invChecks := inventorychecksmock.NewMock().Comp
	check.InitializeInventoryChecksContext(invChecks)
	defer check.ReleaseContext()
	networkCheck.setInterfacesMetadata(ifaces)

	metadata := invChecks.GetInstanceMetadata(string(networkCheck.ID()))
	require.Contains(t, metadata, "interfaces")
	assert.JSONEq(t, `[
		{"name":"eth","description":"USB Ethernet","type":"ethernet","macaddress":"42:52:1c:e5:c2:ec","mtu":1500,"duplex":"full","vlan_id":10,"driver_provider":"Realtek","driver_version":"1.0"},
		{"name":"wifi","type":"wifi","macaddress":"9a:62:9c:bd:37:0a","permanent_macaddress":"bc:d0:74:1e:bd:b7","mtu":1500}
	]`, metadata["interfaces"].(string))
}
