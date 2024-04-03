// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package payload implement processing of Cisco SD-WAN api responses
package payload

import (
	"fmt"
	"math"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

// TimeNow useful for mocking
var TimeNow = time.Now

// GetDevicesMetadata process devices API payloads to build metadata
func GetDevicesMetadata(namespace string, devices []client.Device) []devicemetadata.DeviceMetadata {
	var devicesMetadata []devicemetadata.DeviceMetadata
	for _, device := range devices {
		devicesMetadata = append(devicesMetadata, buildDeviceMetadata(namespace, device))
	}
	return devicesMetadata
}

// GetDevicesTags process devices API payloads to build device tags
func GetDevicesTags(namespace string, devices []client.Device) map[string][]string {
	deviceTags := make(map[string][]string)
	for _, device := range devices {
		deviceTags[device.SystemIP] = buildDeviceTags(namespace, device)
	}
	return deviceTags
}

// GetDevicesUptime process devices API payloads to compute uptimes
func GetDevicesUptime(devices []client.Device) map[string]float64 {
	uptimes := make(map[string]float64)
	for _, device := range devices {
		if device.UptimeDate != 0 {
			uptimes[device.SystemIP] = computeUptime(device)
		}
	}
	return uptimes
}

func buildDeviceMetadata(namespace string, device client.Device) devicemetadata.DeviceMetadata {
	id := fmt.Sprintf("%s:%s", namespace, device.SystemIP)

	return devicemetadata.DeviceMetadata{
		ID:           id,
		IPAddress:    device.SystemIP,
		Vendor:       "cisco",
		Name:         device.HostName,
		Tags:         []string{"source:cisco-sdwan", "device_namespace:" + namespace, "site_id:" + device.SiteID},
		IDTags:       []string{"system_ip:" + device.SystemIP},
		Status:       mapNDMStatus(device.Reachability),
		Model:        device.DeviceModel,
		OsName:       device.DeviceOs,
		Version:      device.Version,
		SerialNumber: device.BoardSerial,
		DeviceType:   mapNDMDeviceType(device.DeviceType),
		ProductName:  device.DeviceModel,
		Location:     device.SiteName,
		Integration:  "cisco-sdwan",
	}
}

func mapNDMStatus(ciscoStatus string) devicemetadata.DeviceStatus {
	if ciscoStatus == "reachable" {
		return devicemetadata.DeviceStatusReachable
	}
	return devicemetadata.DeviceStatusUnreachable
}

func mapNDMDeviceType(ciscoType string) string {
	switch ciscoType {
	case "vsmart", "vmanage", "vbond":
		return "sd-wan"
	case "vedge":
		return "router"
	}
	return "other"
}

func buildDeviceTags(namespace string, device client.Device) []string {
	return []string{
		"device_vendor:cisco",
		"device_namespace:" + namespace,
		"hostname:" + device.HostName,
		"system_ip:" + device.SystemIP,
		"site_id:" + device.SiteID,
		"type:" + device.DeviceType,
	}
}

func computeUptime(device client.Device) float64 {
	now := TimeNow().UnixMilli()
	return math.Round((float64(now) - device.UptimeDate) / 10) // In hundredths of a second, to match SNMP
}
