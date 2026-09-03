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
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "10.0.0.5"},
		Profile:  config.ProfileDefinition{Name: "basic-interfaces"},
	}
	snapshot := []client.CachedValue{
		{Key: client.CacheKey{Path: pathHostname}, Entry: client.CacheEntry{Value: "router-1"}},
		{Key: client.CacheKey{Path: pathVendorName}, Entry: client.CacheEntry{Value: "Cisco"}},
		{Key: client.CacheKey{Path: pathSerialNumber}, Entry: client.CacheEntry{Value: "ABC123"}},
		{Key: client.CacheKey{Path: pathPlatform}, Entry: client.CacheEntry{Value: "ASR9000"}},
		{Key: client.CacheKey{Path: pathSoftwareVersion}, Entry: client.CacheEntry{Value: "7.3.2"}},
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
			Key:   client.CacheKey{Path: "/interfaces/interface/state/name", Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: "eth0"},
		},
		{
			Key:   client.CacheKey{Path: "/interfaces/interface/state/admin-status", Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: "UP"},
		},
		{
			Key:   client.CacheKey{Path: "/interfaces/interface/state/oper-status", Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: "UP"},
		},
		{
			Key:   client.CacheKey{Path: "/interfaces/interface/state/mac-address", Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: "00:11:22:33:44:55"},
		},
		{
			Key:   client.CacheKey{Path: "/interfaces/interface/state/ifindex", Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: int32(42)},
		},
		{
			Key:   client.CacheKey{Path: "/interfaces/interface/state/type", Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: "iana-if-type:ethernetCsmacd"},
		},
	}

	interfaces := buildInterfaceMetadata("default:10.0.0.5", snapshot)
	require.Len(t, interfaces, 1)
	assert.Equal(t, int32(42), interfaces[0].Index)
	assert.Equal(t, "eth0", interfaces[0].Name)
	assert.Equal(t, devicemetadata.AdminStatusUp, interfaces[0].AdminStatus)
	assert.Equal(t, devicemetadata.OperStatusUp, interfaces[0].OperStatus)
	require.NotNil(t, interfaces[0].IsPhysical)
	assert.True(t, *interfaces[0].IsPhysical)
}

func TestReportMetadataSubmitsEvent(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "10.0.0.5"},
		Profile:  config.ProfileDefinition{Name: "basic-interfaces"},
	}
	snapshot := []client.CachedValue{
		{Key: client.CacheKey{Path: pathHostname}, Entry: client.CacheEntry{Value: "router-1"}},
		{
			Key:   client.CacheKey{Path: "/interfaces/interface/state/name", Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: "eth0"},
		},
	}

	mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
	mockSender.SetupAcceptAll()

	err := ReportMetadata(mockSender, cfg, snapshot, time.Unix(123, 0))
	require.NoError(t, err)

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
	require.Len(t, metadata.Devices, 1)
	assert.Equal(t, "router-1", metadata.Devices[0].Name)
	assert.Equal(t, "gnmi", string(metadata.Integration))
	require.Len(t, metadata.Interfaces, 1)
	assert.Equal(t, "eth0", metadata.Interfaces[0].Name)
}

func TestReportMetadataSkipsEmptyPayload(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "10.0.0.5"},
		Profile:  config.ProfileDefinition{Name: "basic-interfaces"},
	}

	mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
	mockSender.SetupAcceptAll()

	err := ReportMetadata(mockSender, cfg, nil, time.Unix(123, 0))
	require.NoError(t, err)
	mockSender.AssertNotCalled(t, "EventPlatformEvent", mock.Anything, mock.Anything)
}
