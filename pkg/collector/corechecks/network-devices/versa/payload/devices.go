// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package payload implement processing of Versa api responses
package payload

import (
	"fmt"
	"math"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TimeNow useful for mocking
var TimeNow = time.Now

// DeviceUserTagResourcePrefix contains the REDAPL table to store device user tags
const DeviceUserTagResourcePrefix = "dd.internal.resource:ndm_device_user_tags"

// GetDevicesMetadata process devices API payloads to build metadata
func GetDevicesMetadata(namespace string, devices []client.ApplianceLite) []devicemetadata.DeviceMetadata {
	var devicesMetadata []devicemetadata.DeviceMetadata
	for _, device := range devices {
		devicesMetadata = append(devicesMetadata, buildDeviceMetadata(namespace, device))
	}
	return devicesMetadata
}

// GetControllersMetadata process devices API payloads to build metadata
func GetControllersMetadata(namespace string, controllers []client.ControllerStatus) []devicemetadata.DeviceMetadata {
	var controllersMetadata []devicemetadata.DeviceMetadata
	for _, controller := range controllers {
		controllersMetadata = append(controllersMetadata, buildControllerMetadata(namespace, controller))
	}
	return controllersMetadata
}

// GetDevicesTags process devices API payloads to build device tags
func GetDevicesTags(namespace string, devices []client.ApplianceLite) map[string][]string {
	deviceTags := make(map[string][]string)
	for _, device := range devices {
		deviceTags[device.IPAddress] = buildApplianceDeviceTags(namespace, device)
		deviceTags[device.IPAddress] = append(deviceTags[device.IPAddress], fmt.Sprintf("%s:%s", DeviceUserTagResourcePrefix, buildDeviceID(namespace, device)))
	}
	return deviceTags
}

// GetDevicesUptime process devices API payloads to compute uptimes
func GetDevicesUptime(devices []client.ApplianceLite) map[string]float64 {
	uptimes := make(map[string]float64, len(devices))
	for _, device := range devices {
		uptime, err := computeUptime(device)
		if err != nil {
			log.Warnf("Error computing device uptime: %v", err)
			continue
		}
		uptimes[device.IPAddress] = uptime
	}
	return uptimes
}

// GetDevicesStatus process devices API payloads to get status
func GetDevicesStatus(devices []client.ApplianceLite) map[string]float64 {
	states := make(map[string]float64)
	for _, device := range devices {
		status := 1.0
		if device.Unreachable {
			status = 0.0
		}
		states[device.IPAddress] = status
	}
	return states
}

func buildDeviceMetadata(namespace string, device client.ApplianceLite) devicemetadata.DeviceMetadata {
	id := buildDeviceID(namespace, device)

	return devicemetadata.DeviceMetadata{
		ID:           id,
		IPAddress:    device.IPAddress,
		Vendor:       "versa",
		Name:         device.Name,
		Tags:         append(buildApplianceDeviceTags(namespace, device), "source:versa"),
		IDTags:       []string{"device_namespace:" + namespace, "system_ip:" + device.IPAddress},
		Status:       mapNDMBoolStatus(device.Unreachable),
		PingStatus:   mapNDMPingStatus(device.PingStatus),
		Model:        device.Hardware.Model,
		OsName:       "Versa PLACEHOLDER",
		Version:      device.SoftwareVersion,
		SerialNumber: device.Hardware.SerialNo,
		DeviceType:   mapNDMDeviceType(device.Type),
		ProductName:  device.Hardware.Model,
		Location:     device.Location,
		Integration:  "versa",
	}
}

func buildControllerMetadata(namespace string, controller client.ControllerStatus) devicemetadata.DeviceMetadata {
	id := fmt.Sprintf("%s:%s", namespace, controller.Name)

	return devicemetadata.DeviceMetadata{
		ID:        id,
		IPAddress: controller.IPAddress,
		Vendor:    "versa",
		Name:      controller.Name,
		Tags:      append(buildControllerDeviceTags(namespace, controller), "source:versa"),
		IDTags:    []string{"device_namespace:" + namespace, "system_ip:" + controller.IPAddress},
		Status:    mapNDMStringStatus(controller.Status),
	}
}

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use this
func mapNDMBoolStatus(versaUnreachable bool) devicemetadata.DeviceStatus {
	if versaUnreachable {
		return devicemetadata.DeviceStatusUnreachable
	}
	return devicemetadata.DeviceStatusReachable
}

func mapNDMStringStatus(versaStatus string) devicemetadata.DeviceStatus {
	if versaStatus == "UNREACHABLE" {
		return devicemetadata.DeviceStatusUnreachable
	}
	return devicemetadata.DeviceStatusReachable
}

func mapNDMPingStatus(versaPingStatus string) devicemetadata.DeviceStatus {
	if versaPingStatus == "UNREACHABLE" {
		return devicemetadata.DeviceStatusUnreachable
	}
	return devicemetadata.DeviceStatusReachable
}

func mapNDMDeviceType(_ string) string {
	return "Versa PLACEHOLDER"
}

func buildApplianceDeviceTags(namespace string, device client.ApplianceLite) []string {
	return []string{
		"device_vendor:versa",
		"device_namespace:" + namespace,
		"hostname:" + device.Name,
		"system_ip:" + device.IPAddress,
		"site_id:" + device.Location,
		"type:" + device.Type,
		"device_ip:" + device.IPAddress,
		"device_hostname:" + device.Name,
		"device_id:" + buildDeviceID(namespace, device),
	}
}

func buildControllerDeviceTags(namespace string, controller client.ControllerStatus) []string {
	return []string{
		"device_vendor:versa",
		"device_namespace:" + namespace,
		"hostname:" + controller.Name,
		"system_ip:" + controller.IPAddress,
		"lock_status:" + controller.LockStatus,
		"sync_status:" + controller.SyncStatus,
		"device_ip:" + controller.IPAddress,
		"device_hostname:" + controller.Name,
		"device_id:" + fmt.Sprintf("%s:%s", namespace, controller.Name),
	}
}

func computeUptime(device client.ApplianceLite) (float64, error) {
	now := TimeNow().UnixMilli()

	// TODO: should we be using lastUpdatedTime instead of startTime?
	parsedTime, err := time.Parse("Mon Jan 2 15:04:05 2006", device.StartTime)
	if err != nil {
		return 0, fmt.Errorf("Error parsing device uptime: %w", err)
	}
	deviceUptime := parsedTime.UnixMilli()

	return math.Round((float64(now) - float64(deviceUptime)) / 10), nil // In hundredths of a second, to match SNMP
}

func buildDeviceID(namespace string, device client.ApplianceLite) string {
	return fmt.Sprintf("%s:%s", namespace, device.IPAddress)
}
