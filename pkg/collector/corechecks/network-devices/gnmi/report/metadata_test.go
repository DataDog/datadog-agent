// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

func TestShouldReportMetadata(t *testing.T) {
	interval := 10 * time.Minute
	now := time.Unix(1000, 0)

	assert.True(t, ShouldReportMetadata(time.Time{}, interval, now))
	assert.True(t, ShouldReportMetadata(now.Add(-11*time.Minute), interval, now))
	assert.False(t, ShouldReportMetadata(now.Add(-5*time.Minute), interval, now))
}

func TestBuildDeviceMetadata(t *testing.T) {
	defaultMetadata := config.DefaultOpenConfigMetadata()
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "10.0.0.5"},
		Profile:  config.ProfileDefinition{Name: "interface-stats"},
	}
	snapshot := []client.CachedValue{
		{Key: client.CacheKey{Path: defaultMetadata.Device.Hostname}, Entry: client.CacheEntry{Value: "router-1"}},
		{
			Key:   client.CacheKey{Path: defaultMetadata.Device.VendorName, Keys: map[string]string{"name": "Chassis"}},
			Entry: client.CacheEntry{Value: "Cisco"},
		},
		{
			Key:   client.CacheKey{Path: defaultMetadata.Device.VendorName, Keys: map[string]string{"name": "FanTray3"}},
			Entry: client.CacheEntry{Value: "Nokia"},
		},
		{
			Key:   client.CacheKey{Path: defaultMetadata.Device.SerialNumber, Keys: map[string]string{"name": "Chassis"}},
			Entry: client.CacheEntry{Value: "ABC123"},
		},
		{
			Key:   client.CacheKey{Path: defaultMetadata.Device.Platform, Keys: map[string]string{"name": "Chassis"}},
			Entry: client.CacheEntry{Value: "ASR9000"},
		},
		{
			Key:   client.CacheKey{Path: defaultMetadata.Device.Platform, Keys: map[string]string{"name": "FanTray3"}},
			Entry: client.CacheEntry{Value: "imm48-25g-sfp28"},
		},
		{Key: client.CacheKey{Path: defaultMetadata.Device.SoftwareVersion}, Entry: client.CacheEntry{Value: "7.3.2"}},
		{
			Key:   client.CacheKey{Path: defaultMetadata.Device.HardwareVersion, Keys: map[string]string{"name": "Chassis"}},
			Entry: client.CacheEntry{Value: "Sim Part No."},
		},
		{
			Key:   client.CacheKey{Path: defaultMetadata.Device.HardwareVersion, Keys: map[string]string{"name": "FanTray3"}},
			Entry: client.CacheEntry{Value: "Unknown"},
		},
	}

	device := buildDeviceMetadata("default:10.0.0.5", cfg, snapshot, []string{"device_ip:10.0.0.5"})
	assert.Equal(t, "default:10.0.0.5", device.ID)
	assert.Equal(t, []string{"device_namespace:default", "snmp_device:10.0.0.5"}, device.IDTags)
	assert.Equal(t, "router-1", device.Name)
	assert.Equal(t, "router-1", device.OsHostname)
	assert.Equal(t, "Cisco", device.Vendor)
	assert.Equal(t, "ABC123", device.SerialNumber)
	assert.Equal(t, "ASR9000", device.ProductName)
	assert.Equal(t, "Sim Part No.", device.Version)
	assert.Equal(t, "7.3.2", device.OsVersion)
	assert.Equal(t, "gnmi", device.Integration)
}

func TestBuildDeviceMetadataUsesIPAddressWhenHostnameMissing(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "10.0.0.9"},
		Profile:  config.ProfileDefinition{Name: "interface-stats"},
	}

	device := buildDeviceMetadata("default:10.0.0.9", cfg, nil, nil)
	assert.Equal(t, "10.0.0.9", device.Name)
	assert.Empty(t, device.OsHostname)
}

func TestBuildBaseTagsIncludesSNMPHost(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "10.0.0.5"},
		Profile:  config.ProfileDefinition{Name: "interface-stats"},
	}
	snapshot := []client.CachedValue{
		{
			Key:   client.CacheKey{Path: "/openconfig/system/state/hostname"},
			Entry: client.CacheEntry{Value: "router-1"},
		},
	}

	tags := buildBaseTags(cfg, snapshot)
	assert.Contains(t, tags, "snmp_host:router-1")
	assert.Contains(t, tags, "snmp_profile:interface-stats")
	assert.Contains(t, tags, "snmp_device:10.0.0.5")
	assert.Contains(t, tags, deviceNamespaceTag)
	assert.Contains(t, tags, integrationSourceGNMITag)
	assert.Contains(t, tags, internalDeviceResourceTag("default:10.0.0.5"))
}

func TestBuildDeviceIDTags(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "10.0.0.5"},
	}
	assert.Equal(t, []string{"device_namespace:default", "snmp_device:10.0.0.5"}, buildDeviceIDTags(cfg))
}

func TestReportMetadataUsesConfigAddressForDeviceID(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "srl1"},
		Profile:  config.ProfileDefinition{Name: "interface-stats"},
	}
	snapshot := []client.CachedValue{
		{Key: client.CacheKey{Path: config.DefaultOpenConfigMetadata().Device.Hostname}, Entry: client.CacheEntry{Value: "router-1"}},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/name", Keys: map[string]string{"name": "Ethernet1"}},
			Entry: client.CacheEntry{Value: "Ethernet1"},
		},
	}

	mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
	mockSender.SetupAcceptAll()

	sent, err := ReportMetadata(mockSender, cfg, snapshot, time.Unix(123, 0))
	require.NoError(t, err)
	require.True(t, sent)

	var rawEvent []byte
	for _, call := range mockSender.Calls {
		if call.Method == "EventPlatformEvent" {
			rawEvent = call.Arguments[0].([]byte)
			break
		}
	}
	require.NotEmpty(t, rawEvent)

	var metadata devicemetadata.NetworkDevicesMetadata
	require.NoError(t, json.Unmarshal(rawEvent, &metadata))
	require.Len(t, metadata.Devices, 1)
	assert.Equal(t, "default:srl1", metadata.Devices[0].ID)
	assert.Equal(t, "srl1", metadata.Devices[0].IPAddress)
	assert.Equal(t, "router-1", metadata.Devices[0].Name)
}

func TestReportMetadataUsesConfigAddressForLoopbackDeviceID(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "127.0.0.1"},
		Profile:  config.ProfileDefinition{Name: "interface-stats"},
	}
	snapshot := []client.CachedValue{
		{Key: client.CacheKey{Path: config.DefaultOpenConfigMetadata().Device.Hostname}, Entry: client.CacheEntry{Value: "srl2"}},
	}

	mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
	mockSender.SetupAcceptAll()

	sent, err := ReportMetadata(mockSender, cfg, snapshot, time.Unix(123, 0))
	require.NoError(t, err)
	require.True(t, sent)

	var rawEvent []byte
	for _, call := range mockSender.Calls {
		if call.Method == "EventPlatformEvent" {
			rawEvent = call.Arguments[0].([]byte)
			break
		}
	}
	require.NotEmpty(t, rawEvent)

	var metadata devicemetadata.NetworkDevicesMetadata
	require.NoError(t, json.Unmarshal(rawEvent, &metadata))
	require.Len(t, metadata.Devices, 1)
	assert.Equal(t, "default:127.0.0.1", metadata.Devices[0].ID)
	assert.Equal(t, "127.0.0.1", metadata.Devices[0].IPAddress)
	assert.Equal(t, "srl2", metadata.Devices[0].Name)
}

func TestResolveDeviceHostnameFallback(t *testing.T) {
	snapshot := []client.CachedValue{
		{
			Key:   client.CacheKey{Path: "/openconfig/system/config/hostname"},
			Entry: client.CacheEntry{Value: "srl2"},
		},
	}

	hostname := ResolveDeviceHostname(snapshot, config.MetadataConfig{})
	assert.Equal(t, "srl2", hostname)
}

func TestBuildInterfaceMetadata(t *testing.T) {
	snapshot := []client.CachedValue{
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/name", Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: "eth0"},
		},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/admin-status", Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: "UP"},
		},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/oper-status", Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: "UP"},
		},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/ethernet/state/hw-mac-address", Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: "00:11:22:33:44:55"},
		},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/ifindex", Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: int32(42)},
		},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/type", Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: "iana-if-type:ethernetCsmacd"},
		},
	}

	interfaces := buildInterfaceMetadata("default:10.0.0.5", config.DefaultOpenConfigMetadata(), snapshot, nil)
	require.Len(t, interfaces, 1)
	assert.Equal(t, int32(42), interfaces[0].Index)
	assert.Equal(t, "eth0", interfaces[0].Name)
	assert.Equal(t, []string{"interface:eth0"}, interfaces[0].IDTags)
	assert.Equal(t, devicemetadata.AdminStatusUp, interfaces[0].AdminStatus)
	assert.Equal(t, devicemetadata.OperStatusUp, interfaces[0].OperStatus)
	require.NotNil(t, interfaces[0].IsPhysical)
	assert.True(t, *interfaces[0].IsPhysical)
	assert.Equal(t, "default:10.0.0.5:eth0", interfaces[0].RawID)
	assert.Equal(t, "gnmi_interface", interfaces[0].RawIDType)
}

func TestBuildInterfaceMetadataIncludesInterfacesWithoutIfIndex(t *testing.T) {
	snapshot := []client.CachedValue{
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/name", Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: "eth0"},
		},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/name", Keys: map[string]string{"name": "eth1"}},
			Entry: client.CacheEntry{Value: "eth1"},
		},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/ifindex", Keys: map[string]string{"name": "eth1"}},
			Entry: client.CacheEntry{Value: int32(7)},
		},
	}

	interfaces := buildInterfaceMetadata("default:10.0.0.5", config.DefaultOpenConfigMetadata(), snapshot, nil)
	require.Len(t, interfaces, 2)
	assert.Equal(t, "eth0", interfaces[0].Name)
	assert.Equal(t, int32(0), interfaces[0].Index)
	assert.Equal(t, []string{"interface:eth0"}, interfaces[0].IDTags)
	assert.Equal(t, "eth1", interfaces[1].Name)
	assert.Equal(t, int32(7), interfaces[1].Index)
	assert.Equal(t, []string{"interface:eth1"}, interfaces[1].IDTags)
}

func TestInterfaceSnapshotComplete(t *testing.T) {
	metadata := config.DefaultOpenConfigMetadata()
	assert.True(t, InterfaceSnapshotComplete(nil, metadata))

	assert.True(t, InterfaceSnapshotComplete([]client.CachedValue{
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/name", Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: "eth0"},
		},
	}, metadata))
}

func TestReportInterfaceStatus(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "10.0.0.5"},
		Profile:  config.ProfileDefinition{Name: "interface-stats"},
	}
	snapshot := []client.CachedValue{
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/name", Keys: map[string]string{"name": "Ethernet1"}},
			Entry: client.CacheEntry{Value: "Ethernet1"},
		},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/description", Keys: map[string]string{"name": "Ethernet1"}},
			Entry: client.CacheEntry{Value: "uplink"},
		},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/admin-status", Keys: map[string]string{"name": "Ethernet1"}},
			Entry: client.CacheEntry{Value: "UP"},
		},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/oper-status", Keys: map[string]string{"name": "Ethernet1"}},
			Entry: client.CacheEntry{Value: "UP"},
		},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/ifindex", Keys: map[string]string{"name": "Ethernet1"}},
			Entry: client.CacheEntry{Value: int32(42)},
		},
	}

	mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
	mockSender.SetupAcceptAll()

	require.NoError(t, ReportInterfaceStatus(mockSender, cfg, snapshot))
	mockSender.AssertMetric(t, "Gauge", interfaceStatusMetric, 1, "", []string{
		"status:up",
		"admin_status:up",
		"oper_status:up",
		"interface:Ethernet1",
		"interface_alias:uplink",
		deviceNamespaceTag,
		"device_ip:10.0.0.5",
		"device_id:default:10.0.0.5",
		"snmp_device:10.0.0.5",
		"integration_source:gnmi",
		"dd.internal.resource:ndm_interface:default:10.0.0.5:Ethernet1",
	})
}

func TestReportMetadataSubmitsEvent(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "10.0.0.5"},
		Profile:  config.ProfileDefinition{Name: "interface-stats"},
	}
	snapshot := []client.CachedValue{
		{Key: client.CacheKey{Path: config.DefaultOpenConfigMetadata().Device.Hostname}, Entry: client.CacheEntry{Value: "router-1"}},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/name", Keys: map[string]string{"name": "Ethernet1"}},
			Entry: client.CacheEntry{Value: "Ethernet1"},
		},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/description", Keys: map[string]string{"name": "Ethernet1"}},
			Entry: client.CacheEntry{Value: "uplink"},
		},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/admin-status", Keys: map[string]string{"name": "Ethernet1"}},
			Entry: client.CacheEntry{Value: "UP"},
		},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/oper-status", Keys: map[string]string{"name": "Ethernet1"}},
			Entry: client.CacheEntry{Value: "UP"},
		},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/ifindex", Keys: map[string]string{"name": "Ethernet1"}},
			Entry: client.CacheEntry{Value: int32(42)},
		},
	}

	mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
	mockSender.SetupAcceptAll()

	sent, err := ReportMetadata(mockSender, cfg, snapshot, time.Unix(123, 0))
	require.NoError(t, err)
	require.True(t, sent)

	mockSender.AssertCalled(t, "EventPlatformEvent", mock.AnythingOfType("[]uint8"), "network-devices-metadata")

	var rawEvent []byte
	for _, call := range mockSender.Calls {
		if call.Method == "EventPlatformEvent" {
			rawEvent = call.Arguments[0].([]byte)
			break
		}
	}
	require.NotEmpty(t, rawEvent)

	var metadata devicemetadata.NetworkDevicesMetadata
	require.NoError(t, json.Unmarshal(rawEvent, &metadata))
	assert.Equal(t, "default", metadata.Namespace)
	assert.Equal(t, "gnmi", string(metadata.Integration))
	assert.Equal(t, int64(123), metadata.CollectTimestamp)
	require.Len(t, metadata.Devices, 1)
	assert.Equal(t, "default:10.0.0.5", metadata.Devices[0].ID)
	assert.Equal(t, "10.0.0.5", metadata.Devices[0].IPAddress)
	assert.Equal(t, devicemetadata.DeviceStatusReachable, metadata.Devices[0].Status)
	assert.Equal(t, "router-1", metadata.Devices[0].Name)
	assert.Equal(t, "gnmi", metadata.Devices[0].Integration)
	require.Len(t, metadata.Interfaces, 1)
	assert.Equal(t, "default:10.0.0.5", metadata.Interfaces[0].DeviceID)
	assert.Equal(t, []string{"interface:Ethernet1"}, metadata.Interfaces[0].IDTags)
	assert.Equal(t, int32(42), metadata.Interfaces[0].Index)
	assert.Equal(t, "Ethernet1", metadata.Interfaces[0].Name)
	assert.Equal(t, "uplink", metadata.Interfaces[0].Description)
	assert.Equal(t, devicemetadata.AdminStatusUp, metadata.Interfaces[0].AdminStatus)
	assert.Equal(t, devicemetadata.OperStatusUp, metadata.Interfaces[0].OperStatus)
}

func TestBuildIPAddressMetadata(t *testing.T) {
	metadata := config.DefaultOpenConfigMetadata()
	deviceID := "default:router-1"
	snapshot := []client.CachedValue{
		{
			Key: client.CacheKey{
				Path: metadata.Interface.IfIndex,
				Keys: map[string]string{"name": "Ethernet1"},
			},
			Entry: client.CacheEntry{Value: int32(42)},
		},
		{
			Key: client.CacheKey{
				Path: metadata.IPAddress.IP,
				Keys: map[string]string{"name": "Ethernet1", "index": "0", "ip": "10.1.1.1"},
			},
			Entry: client.CacheEntry{Value: "10.1.1.1"},
		},
		{
			Key: client.CacheKey{
				Path: metadata.IPAddress.PrefixLength,
				Keys: map[string]string{"name": "Ethernet1", "index": "0", "ip": "10.1.1.1"},
			},
			Entry: client.CacheEntry{Value: int32(24)},
		},
	}
	interfaces := buildInterfaceMetadata(deviceID, metadata, snapshot, nil)

	ipAddresses := buildIPAddressMetadata(deviceID, metadata, interfaces, snapshot)
	require.Len(t, ipAddresses, 1)
	assert.Equal(t, "default:router-1:42", ipAddresses[0].InterfaceID)
	assert.Equal(t, "10.1.1.1", ipAddresses[0].IPAddress)
	assert.Equal(t, int32(24), ipAddresses[0].Prefixlen)
}

func TestBuildIPAddressMetadataIncludesIPv6(t *testing.T) {
	metadata := config.DefaultOpenConfigMetadata()
	deviceID := "default:router-1"
	snapshot := []client.CachedValue{
		{
			Key: client.CacheKey{
				Path: metadata.Interface.IfIndex,
				Keys: map[string]string{"name": "Ethernet1"},
			},
			Entry: client.CacheEntry{Value: int32(42)},
		},
		{
			Key: client.CacheKey{
				Path: metadata.IPAddress.IP,
				Keys: map[string]string{"name": "Ethernet1", "index": "0", "ip": "10.1.1.1"},
			},
			Entry: client.CacheEntry{Value: "10.1.1.1"},
		},
		{
			Key: client.CacheKey{
				Path: metadata.IPAddress.PrefixLength,
				Keys: map[string]string{"name": "Ethernet1", "index": "0", "ip": "10.1.1.1"},
			},
			Entry: client.CacheEntry{Value: int32(24)},
		},
		{
			Key: client.CacheKey{
				Path: metadata.IPAddress.IPv6,
				Keys: map[string]string{"name": "Ethernet1", "index": "0", "ip": "2001:db8::1"},
			},
			Entry: client.CacheEntry{Value: "2001:db8::1"},
		},
		{
			Key: client.CacheKey{
				Path: metadata.IPAddress.IPv6PrefixLength,
				Keys: map[string]string{"name": "Ethernet1", "index": "0", "ip": "2001:db8::1"},
			},
			Entry: client.CacheEntry{Value: int32(64)},
		},
	}
	interfaces := buildInterfaceMetadata(deviceID, metadata, snapshot, nil)

	ipAddresses := buildIPAddressMetadata(deviceID, metadata, interfaces, snapshot)
	require.Len(t, ipAddresses, 2)
	assert.Equal(t, "default:router-1:42", ipAddresses[0].InterfaceID)
	assert.Equal(t, "10.1.1.1", ipAddresses[0].IPAddress)
	assert.Equal(t, int32(24), ipAddresses[0].Prefixlen)
	assert.Equal(t, "default:router-1:42", ipAddresses[1].InterfaceID)
	assert.Equal(t, "2001:db8::1", ipAddresses[1].IPAddress)
	assert.Equal(t, int32(64), ipAddresses[1].Prefixlen)
}

func TestBuildIPAddressMetadataIPv6Only(t *testing.T) {
	metadata := config.DefaultOpenConfigMetadata()
	deviceID := "default:router-1"
	snapshot := []client.CachedValue{
		{
			Key: client.CacheKey{
				Path: metadata.Interface.IfIndex,
				Keys: map[string]string{"name": "Ethernet1"},
			},
			Entry: client.CacheEntry{Value: int32(42)},
		},
		{
			Key: client.CacheKey{
				Path: metadata.IPAddress.IPv6,
				Keys: map[string]string{"name": "Ethernet1", "index": "0", "ip": "2001:db8::1"},
			},
			Entry: client.CacheEntry{Value: "2001:db8::1"},
		},
		{
			Key: client.CacheKey{
				Path: metadata.IPAddress.IPv6PrefixLength,
				Keys: map[string]string{"name": "Ethernet1", "index": "0", "ip": "2001:db8::1"},
			},
			Entry: client.CacheEntry{Value: int32(64)},
		},
	}
	interfaces := buildInterfaceMetadata(deviceID, metadata, snapshot, nil)

	ipAddresses := buildIPAddressMetadata(deviceID, metadata, interfaces, snapshot)
	require.Len(t, ipAddresses, 1)
	assert.Equal(t, "default:router-1:42", ipAddresses[0].InterfaceID)
	assert.Equal(t, "2001:db8::1", ipAddresses[0].IPAddress)
	assert.Equal(t, int32(64), ipAddresses[0].Prefixlen)
}

func TestReportMetadataIncludesIPAddresses(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "10.0.0.5"},
		Profile:  config.ProfileDefinition{Name: "interface-stats"},
	}
	metadata := config.DefaultOpenConfigMetadata()
	snapshot := []client.CachedValue{
		{Key: client.CacheKey{Path: metadata.Device.Hostname}, Entry: client.CacheEntry{Value: "router-1"}},
		{
			Key:   client.CacheKey{Path: metadata.Interface.Name, Keys: map[string]string{"name": "Ethernet1"}},
			Entry: client.CacheEntry{Value: "Ethernet1"},
		},
		{
			Key:   client.CacheKey{Path: metadata.Interface.IfIndex, Keys: map[string]string{"name": "Ethernet1"}},
			Entry: client.CacheEntry{Value: int32(42)},
		},
		{
			Key: client.CacheKey{
				Path: metadata.IPAddress.IP,
				Keys: map[string]string{"name": "Ethernet1", "index": "0", "ip": "10.1.1.1"},
			},
			Entry: client.CacheEntry{Value: "10.1.1.1"},
		},
		{
			Key: client.CacheKey{
				Path: metadata.IPAddress.PrefixLength,
				Keys: map[string]string{"name": "Ethernet1", "index": "0", "ip": "10.1.1.1"},
			},
			Entry: client.CacheEntry{Value: int32(24)},
		},
	}

	mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
	mockSender.SetupAcceptAll()

	sent, err := ReportMetadata(mockSender, cfg, snapshot, time.Unix(123, 0))
	require.NoError(t, err)
	require.True(t, sent)

	var rawEvent []byte
	for _, call := range mockSender.Calls {
		if call.Method == "EventPlatformEvent" {
			rawEvent = call.Arguments[0].([]byte)
		}
	}
	require.NotEmpty(t, rawEvent)

	var payload devicemetadata.NetworkDevicesMetadata
	require.NoError(t, json.Unmarshal(rawEvent, &payload))
	require.Len(t, payload.IPAddresses, 1)
	assert.Equal(t, "default:10.0.0.5:42", payload.IPAddresses[0].InterfaceID)
	assert.Equal(t, "10.1.1.1", payload.IPAddresses[0].IPAddress)
	assert.Equal(t, int32(24), payload.IPAddresses[0].Prefixlen)
}

func TestReportMetadataSkipsEmptyPayload(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "10.0.0.5"},
		Profile:  config.ProfileDefinition{Name: "interface-stats"},
	}

	mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
	mockSender.SetupAcceptAll()

	sent, err := ReportMetadata(mockSender, cfg, nil, time.Unix(123, 0))
	require.NoError(t, err)
	require.False(t, sent)
	mockSender.AssertNotCalled(t, "EventPlatformEvent", mock.Anything, mock.Anything)
}
