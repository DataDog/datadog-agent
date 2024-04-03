// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package payload

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

// mockTimeNow mocks time.Now
var mockTimeNow = func() time.Time {
	layout := "2006-01-02 15:04:05"
	str := "2000-01-01 00:00:00"
	t, _ := time.Parse(layout, str)
	return t
}

var testDevices = []client.Device{
	{
		SystemIP:     "10.0.0.1",
		HostName:     "test-1",
		SiteID:       "100",
		Reachability: "reachable",
		DeviceModel:  "vmanage",
		DeviceOs:     "vmanage-os",
		Version:      "20.12",
		BoardSerial:  "test-serial",
		DeviceType:   "vmanage",
		SiteName:     "test-site",
		UptimeDate:   float64(mockTimeNow().Add(-time.Hour).UnixMilli()),
	},
	{
		SystemIP:     "10.0.0.2",
		HostName:     "test-2",
		SiteID:       "102",
		Reachability: "reachable",
		DeviceModel:  "vbond",
		DeviceOs:     "vbond-os",
		Version:      "20.12",
		BoardSerial:  "test-serial-2",
		DeviceType:   "vbond",
		SiteName:     "test-site-2",
		UptimeDate:   float64(mockTimeNow().Add(-2 * time.Hour).UnixMilli()),
	},
}

func TestProcessDevicesMetadata(t *testing.T) {
	metadata := GetDevicesMetadata("test-ns", testDevices)
	require.Len(t, metadata, 2)
	require.Equal(t, []devicemetadata.DeviceMetadata{
		{
			ID:           "test-ns:10.0.0.1",
			IPAddress:    "10.0.0.1",
			Vendor:       "cisco",
			Name:         "test-1",
			Tags:         []string{"source:cisco-sdwan", "device_namespace:test-ns", "site_id:100"},
			IDTags:       []string{"system_ip:10.0.0.1"},
			Status:       devicemetadata.DeviceStatusReachable,
			Model:        "vmanage",
			OsName:       "vmanage-os",
			Version:      "20.12",
			SerialNumber: "test-serial",
			DeviceType:   "sd-wan",
			ProductName:  "vmanage",
			Location:     "test-site",
			Integration:  "cisco-sdwan",
		},
		{
			ID:           "test-ns:10.0.0.2",
			IPAddress:    "10.0.0.2",
			Vendor:       "cisco",
			Name:         "test-2",
			Tags:         []string{"source:cisco-sdwan", "device_namespace:test-ns", "site_id:102"},
			IDTags:       []string{"system_ip:10.0.0.2"},
			Status:       devicemetadata.DeviceStatusReachable,
			Model:        "vbond",
			OsName:       "vbond-os",
			Version:      "20.12",
			SerialNumber: "test-serial-2",
			DeviceType:   "sd-wan",
			ProductName:  "vbond",
			Location:     "test-site-2",
			Integration:  "cisco-sdwan",
		},
	}, metadata)
}

func TestProcessDevicesTags(t *testing.T) {
	tags := GetDevicesTags("test-ns", testDevices)
	require.Len(t, tags, 2)
	require.Equal(t, map[string][]string{
		"10.0.0.1": {
			"device_vendor:cisco",
			"device_namespace:test-ns",
			"hostname:test-1",
			"system_ip:10.0.0.1",
			"site_id:100",
			"type:vmanage",
		},
		"10.0.0.2": {
			"device_vendor:cisco",
			"device_namespace:test-ns",
			"hostname:test-2",
			"system_ip:10.0.0.2",
			"site_id:102",
			"type:vbond",
		},
	}, tags)
}

func TestProcessDevicesUptime(t *testing.T) {
	TimeNow = mockTimeNow

	uptimes := GetDevicesUptime(testDevices)
	require.Len(t, uptimes, 2)
	require.Equal(t, map[string]float64{
		"10.0.0.1": 360000,
		"10.0.0.2": 720000,
	}, uptimes)
}

func TestBuildDeviceMetadata(t *testing.T) {
	TimeNow = mockTimeNow

	tests := []struct {
		name             string
		namespace        string
		device           client.Device
		expectedMetadata devicemetadata.DeviceMetadata
	}{
		{
			name:      "All fields",
			namespace: "test-ns",
			device: client.Device{
				SystemIP:     "10.0.0.1",
				HostName:     "test-1",
				SiteID:       "100",
				Reachability: "reachable",
				DeviceModel:  "vmanage",
				DeviceOs:     "vmanage-os",
				Version:      "20.12",
				BoardSerial:  "test-serial",
				DeviceType:   "vmanage",
				SiteName:     "test-site",
			},
			expectedMetadata: devicemetadata.DeviceMetadata{
				ID:           "test-ns:10.0.0.1",
				IPAddress:    "10.0.0.1",
				Vendor:       "cisco",
				Name:         "test-1",
				Tags:         []string{"source:cisco-sdwan", "device_namespace:test-ns", "site_id:100"},
				IDTags:       []string{"system_ip:10.0.0.1"},
				Status:       devicemetadata.DeviceStatusReachable,
				Model:        "vmanage",
				OsName:       "vmanage-os",
				Version:      "20.12",
				SerialNumber: "test-serial",
				DeviceType:   "sd-wan",
				ProductName:  "vmanage",
				Location:     "test-site",
				Integration:  "cisco-sdwan",
			},
		},
		{
			name:      "Missing reachability",
			namespace: "test-ns",
			device: client.Device{
				SystemIP:    "10.0.0.1",
				HostName:    "test-1",
				SiteID:      "100",
				DeviceModel: "vmanage",
				DeviceOs:    "vmanage-os",
				Version:     "20.12",
				BoardSerial: "test-serial",
				DeviceType:  "vmanage",
				SiteName:    "test-site",
			},
			expectedMetadata: devicemetadata.DeviceMetadata{
				ID:           "test-ns:10.0.0.1",
				IPAddress:    "10.0.0.1",
				Vendor:       "cisco",
				Name:         "test-1",
				Tags:         []string{"source:cisco-sdwan", "device_namespace:test-ns", "site_id:100"},
				IDTags:       []string{"system_ip:10.0.0.1"},
				Status:       devicemetadata.DeviceStatusUnreachable,
				Model:        "vmanage",
				OsName:       "vmanage-os",
				Version:      "20.12",
				SerialNumber: "test-serial",
				DeviceType:   "sd-wan",
				ProductName:  "vmanage",
				Location:     "test-site",
				Integration:  "cisco-sdwan",
			},
		},
		{
			name:      "Missing device type",
			namespace: "test-ns",
			device: client.Device{
				SystemIP:     "10.0.0.1",
				HostName:     "test-1",
				SiteID:       "100",
				Reachability: "reachable",
				DeviceModel:  "vmanage",
				DeviceOs:     "vmanage-os",
				Version:      "20.12",
				BoardSerial:  "test-serial",
				SiteName:     "test-site",
			},
			expectedMetadata: devicemetadata.DeviceMetadata{
				ID:           "test-ns:10.0.0.1",
				IPAddress:    "10.0.0.1",
				Vendor:       "cisco",
				Name:         "test-1",
				Tags:         []string{"source:cisco-sdwan", "device_namespace:test-ns", "site_id:100"},
				IDTags:       []string{"system_ip:10.0.0.1"},
				Status:       devicemetadata.DeviceStatusReachable,
				Model:        "vmanage",
				OsName:       "vmanage-os",
				Version:      "20.12",
				SerialNumber: "test-serial",
				DeviceType:   "other",
				ProductName:  "vmanage",
				Location:     "test-site",
				Integration:  "cisco-sdwan",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := buildDeviceMetadata(tt.namespace, tt.device)
			require.Equal(t, tt.expectedMetadata, metadata)
		})
	}
}

func TestMapNDMStatus(t *testing.T) {
	tests := []struct {
		ciscoStatus    string
		expectedStatus devicemetadata.DeviceStatus
	}{
		{
			ciscoStatus:    "reachable",
			expectedStatus: devicemetadata.DeviceStatusReachable,
		},
		{
			ciscoStatus:    "unreachable",
			expectedStatus: devicemetadata.DeviceStatusUnreachable,
		},
		{
			ciscoStatus:    "invalid",
			expectedStatus: devicemetadata.DeviceStatusUnreachable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.ciscoStatus, func(t *testing.T) {
			require.Equal(t, tt.expectedStatus, mapNDMStatus(tt.ciscoStatus))
		})
	}
}

func TestMapNDMDeviceType(t *testing.T) {
	tests := []struct {
		ciscoType    string
		expectedType string
	}{
		{
			ciscoType:    "vmanage",
			expectedType: "sd-wan",
		},
		{
			ciscoType:    "vbond",
			expectedType: "sd-wan",
		},
		{
			ciscoType:    "vsmart",
			expectedType: "sd-wan",
		},
		{
			ciscoType:    "vedge",
			expectedType: "router",
		},
		{
			ciscoType:    "anything",
			expectedType: "other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.ciscoType, func(t *testing.T) {
			require.Equal(t, tt.expectedType, mapNDMDeviceType(tt.ciscoType))
		})
	}
}

func TestBuildDeviceTags(t *testing.T) {
	tests := []struct {
		name         string
		namespace    string
		device       client.Device
		expectedTags []string
	}{
		{
			name:      "all tags",
			namespace: "test-ns",
			device: client.Device{
				SystemIP:   "10.0.0.1",
				HostName:   "test-1",
				SiteID:     "1000",
				DeviceType: "vmanage",
			},
			expectedTags: []string{
				"device_vendor:cisco",
				"device_namespace:test-ns",
				"hostname:test-1",
				"system_ip:10.0.0.1",
				"site_id:1000",
				"type:vmanage",
			},
		},
		{
			name:      "missing hostname",
			namespace: "test-ns-2",
			device: client.Device{
				SystemIP:   "10.0.0.1",
				SiteID:     "1000",
				DeviceType: "vmanage",
			},
			expectedTags: []string{
				"device_vendor:cisco",
				"device_namespace:test-ns-2",
				"hostname:",
				"system_ip:10.0.0.1",
				"site_id:1000",
				"type:vmanage",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expectedTags, buildDeviceTags(tt.namespace, tt.device))
		})
	}
}

func TestComputeUptime(t *testing.T) {
	TimeNow = mockTimeNow

	tests := []struct {
		name           string
		devices        client.Device
		expectedUptime float64
	}{
		{
			name: "One hour",
			devices: client.Device{

				SystemIP:   "10.0.0.1",
				UptimeDate: float64(TimeNow().Add(-time.Hour).UnixMilli()),
			},
			expectedUptime: 360000, // One hour
		},
		{
			name: "One day",
			devices: client.Device{

				SystemIP:   "10.0.0.1",
				UptimeDate: float64(TimeNow().Add(-24 * time.Hour).UnixMilli()),
			},
			expectedUptime: 24 * 360000, // One day
		},
		{
			name: "One year",
			devices: client.Device{

				SystemIP:   "10.0.0.1",
				UptimeDate: float64(TimeNow().Add(-365 * 24 * time.Hour).UnixMilli()),
			},
			expectedUptime: 365 * 24 * 360000, // One year
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uptimes := computeUptime(tt.devices)
			require.Equal(t, tt.expectedUptime, uptimes)
		})
	}
}
