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
		{Key: client.CacheKey{Path: defaultMetadata.Device.VendorName}, Entry: client.CacheEntry{Value: "Cisco"}},
		{Key: client.CacheKey{Path: defaultMetadata.Device.SerialNumber}, Entry: client.CacheEntry{Value: "ABC123"}},
		{Key: client.CacheKey{Path: defaultMetadata.Device.Platform}, Entry: client.CacheEntry{Value: "ASR9000"}},
		{Key: client.CacheKey{Path: defaultMetadata.Device.SoftwareVersion}, Entry: client.CacheEntry{Value: "7.3.2"}},
	}

	device := buildDeviceMetadata("default:10.0.0.5", cfg, snapshot, []string{"device_ip:10.0.0.5"})
	assert.Equal(t, "default:10.0.0.5", device.ID)
	assert.Equal(t, "router-1", device.Name)
	assert.Equal(t, "Cisco", device.Vendor)
	assert.Equal(t, "ABC123", device.SerialNumber)
	assert.Equal(t, "ASR9000", device.ProductName)
	assert.Equal(t, "7.3.2", device.OsVersion)
	assert.Equal(t, "gnmi", device.Integration)
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

	interfaces := buildInterfaceMetadata("default:10.0.0.5", config.DefaultOpenConfigMetadata(), snapshot)
	require.Len(t, interfaces, 1)
	assert.Equal(t, int32(42), interfaces[0].Index)
	assert.Equal(t, "eth0", interfaces[0].Name)
	assert.Equal(t, []string{"interface:eth0"}, interfaces[0].IDTags)
	assert.Equal(t, devicemetadata.AdminStatusUp, interfaces[0].AdminStatus)
	assert.Equal(t, devicemetadata.OperStatusUp, interfaces[0].OperStatus)
	require.NotNil(t, interfaces[0].IsPhysical)
	assert.True(t, *interfaces[0].IsPhysical)
}

func TestBuildInterfaceMetadataSkipsMissingIfIndex(t *testing.T) {
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

	interfaces := buildInterfaceMetadata("default:10.0.0.5", config.DefaultOpenConfigMetadata(), snapshot)
	require.Len(t, interfaces, 1)
	assert.Equal(t, "eth1", interfaces[0].Name)
	assert.Equal(t, int32(7), interfaces[0].Index)
	assert.Equal(t, []string{"interface:eth1"}, interfaces[0].IDTags)
}

func TestInterfaceSnapshotComplete(t *testing.T) {
	metadata := config.DefaultOpenConfigMetadata()
	assert.True(t, InterfaceSnapshotComplete(nil, metadata))

	incomplete := []client.CachedValue{
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/name", Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: "eth0"},
		},
	}
	assert.False(t, InterfaceSnapshotComplete(incomplete, metadata))

	complete := append(incomplete, client.CachedValue{
		Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/ifindex", Keys: map[string]string{"name": "eth0"}},
		Entry: client.CacheEntry{Value: int32(42)},
	})
	assert.True(t, InterfaceSnapshotComplete(complete, metadata))
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
		"interface_index:42",
		"interface:Ethernet1",
		"interface_alias:uplink",
		"device_ip:10.0.0.5",
		"device_id:default:10.0.0.5",
		"dd.internal.resource:ndm_interface:default:10.0.0.5:42",
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
