// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

func TestBuildTopologyLinks(t *testing.T) {
	topology := config.DefaultOpenConfigLLDP()
	deviceID := "default:10.0.0.5"
	keys := map[string]string{"name": "eth0", "id": "1"}
	snapshot := []client.CachedValue{
		{Key: client.CacheKey{Path: topology.LLDP.ChassisIDType, Keys: keys}, Entry: client.CacheEntry{Value: "MAC_ADDRESS"}},
		{Key: client.CacheKey{Path: topology.LLDP.ChassisID, Keys: keys}, Entry: client.CacheEntry{Value: "00:aa:bb:cc:dd:ee"}},
		{Key: client.CacheKey{Path: topology.LLDP.PortIDType, Keys: keys}, Entry: client.CacheEntry{Value: "INTERFACE_NAME"}},
		{Key: client.CacheKey{Path: topology.LLDP.PortID, Keys: keys}, Entry: client.CacheEntry{Value: "Gi0/1"}},
		{Key: client.CacheKey{Path: topology.LLDP.SystemName, Keys: keys}, Entry: client.CacheEntry{Value: "remote-switch"}},
		{Key: client.CacheKey{Path: topology.LLDP.ManagementAddress, Keys: keys}, Entry: client.CacheEntry{Value: "10.0.0.9"}},
	}

	interfaces := []devicemetadata.InterfaceMetadata{
		{
			DeviceID: deviceID,
			Index:    1,
			Name:     "eth0",
		},
	}

	links := buildTopologyLinks(deviceID, topology, snapshot, interfaces)
	require.Len(t, links, 1)
	assert.Equal(t, topologyLinkSourceTypeLLDP, links[0].SourceType)
	assert.Equal(t, "gnmi", links[0].Integration)
	require.NotNil(t, links[0].Remote)
	assert.Equal(t, "remote-switch", links[0].Remote.Device.Name)
	assert.Equal(t, "mac_address", links[0].Remote.Device.IDType)
	assert.Equal(t, "00:aa:bb:cc:dd:ee", links[0].Remote.Device.ID)
	assert.Equal(t, "10.0.0.9", links[0].Remote.Device.IPAddress)
	assert.Equal(t, "interface_name", links[0].Remote.Interface.IDType)
	assert.Equal(t, "Gi0/1", links[0].Remote.Interface.ID)
	require.NotNil(t, links[0].Local)
	assert.Equal(t, deviceID+":1", links[0].Local.Interface.DDID)
	assert.Equal(t, "eth0", links[0].Local.Interface.ID)
}

func TestBuildTopologyLinksEmptyWhenNoNeighbors(t *testing.T) {
	links := buildTopologyLinks("default:10.0.0.5", config.TopologyConfig{}, nil, nil)
	assert.Nil(t, links)
}

func TestNormalizeLLDPIDType(t *testing.T) {
	assert.Equal(t, "mac_address", normalizeLLDPIDType("openconfig-lldp-types:MAC_ADDRESS"))
	assert.Equal(t, "mac_address", normalizeLLDPIDType("4"))
	assert.Equal(t, "interface_name", normalizeLLDPIDType("INTERFACE_NAME"))
}
