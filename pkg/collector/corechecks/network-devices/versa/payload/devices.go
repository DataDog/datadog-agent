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

var (
	// TimeNow useful for mocking
	TimeNow = time.Now
)

// DeviceUserTagResourcePrefix contains the REDAPL table to store device user tags
const DeviceUserTagResourcePrefix = "dd.internal.resource:ndm_device_user_tags"

// GetDeviceMetadataFromAppliances process devices API payloads to build metadata
func GetDeviceMetadataFromAppliances(namespace string, devices []client.Appliance) []devicemetadata.DeviceMetadata {
	var devicesMetadata []devicemetadata.DeviceMetadata
	for _, device := range devices {
		devicesMetadata = append(devicesMetadata, buildApplianceDeviceMetadata(namespace, device))
	}
	return devicesMetadata
}

// GetDeviceMetadataFromDirector process devices API payloads to build metadata
func GetDeviceMetadataFromDirector(namespace string, director *client.DirectorStatus) (devicemetadata.DeviceMetadata, error) {
	ipAddress, err := director.IPAddress()
	if err != nil {
		return devicemetadata.DeviceMetadata{}, err
	}

	return devicemetadata.DeviceMetadata{
		ID:          buildDeviceID(namespace, ipAddress),
		IDTags:      []string{"device_namespace:" + namespace, "device_ip:" + ipAddress},
		Tags:        buildDirectorDeviceTags(namespace, ipAddress, director),
		IPAddress:   ipAddress,
		Vendor:      "versa",
		Name:        director.HAConfig.ClusterID,
		Status:      devicemetadata.DeviceStatusReachable, // TODO: seems like there's no real concept of reachable/unreachable for directors
		OsVersion:   director.PkgInfo.Version,
		Integration: "versa",
		DeviceType:  mapNDMDeviceType("director"),
	}, nil
}

// GetApplianceDevicesTags process devices API payloads to build device tags
func GetApplianceDevicesTags(namespace string, devices []client.Appliance) map[string][]string {
	deviceTags := make(map[string][]string)
	for _, device := range devices {
		deviceTags[device.IPAddress] = buildApplianceDeviceTags(namespace, device)
		deviceTags[device.IPAddress] = append(deviceTags[device.IPAddress], fmt.Sprintf("%s:%s", DeviceUserTagResourcePrefix, buildDeviceID(namespace, device.IPAddress)))
	}
	return deviceTags
}

// GetDirectorDeviceTags process devices API payloads to build device tags
func GetDirectorDeviceTags(namespace string, director *client.DirectorStatus) (map[string][]string, error) {
	ipAddress, err := director.IPAddress()
	if err != nil {
		return nil, err
	}

	deviceTags := make(map[string][]string)
	deviceTags[ipAddress] = buildDirectorDeviceTags(namespace, ipAddress, director)
	deviceTags[ipAddress] = append(deviceTags[ipAddress], fmt.Sprintf("%s:%s", DeviceUserTagResourcePrefix, buildDeviceID(namespace, ipAddress)))
	return deviceTags, nil
}

// GetDevicesUptime process devices API payloads to compute uptimes
func GetDevicesUptime(devices []client.Appliance) map[string]float64 {
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
func GetDevicesStatus(devices []client.Appliance) map[string]float64 {
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

func buildApplianceDeviceMetadata(namespace string, device client.Appliance) devicemetadata.DeviceMetadata {
	id := buildDeviceID(namespace, device.IPAddress)

	return devicemetadata.DeviceMetadata{
		ID:           id,
		IPAddress:    device.IPAddress,
		Vendor:       "versa",
		Name:         device.Name,
		Tags:         append(buildApplianceDeviceTags(namespace, device), "source:versa"),
		IDTags:       []string{"device_namespace:" + namespace, "device_ip:" + device.IPAddress},
		Status:       mapNDMBoolStatus(device.Unreachable),
		PingStatus:   mapNDMPingStatus(device.PingStatus),
		Model:        device.Hardware.Model,
		Version:      device.SoftwareVersion,
		SerialNumber: device.Hardware.SerialNo,
		DeviceType:   mapNDMDeviceType(device.Type),
		ProductName:  device.Hardware.Model,
		Location:     device.ApplianceLocation.LocationID, // TODO: is this appropriate?
		Integration:  "versa",
	}
}

// TODO: can we move this to a common package? Cisco SD-WAN and Versa use this
func mapNDMBoolStatus(versaUnreachable bool) devicemetadata.DeviceStatus {
	if versaUnreachable {
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

func mapNDMDeviceType(versaType string) string {
	switch versaType {
	case "branch":
		return "router"
	case "controller", "director":
		return "sd-wan"
	default:
		return "other"
	}
}

func buildApplianceDeviceTags(namespace string, device client.Appliance) []string {
	tags := []string{
		"device_vendor:versa",
		"device_namespace:" + namespace,
		"hostname:" + device.Name,
		"site:" + device.Name,
		"system_ip:" + device.IPAddress,
		"location_id:" + device.ApplianceLocation.LocationID, // TODO: is this appropriate?
		"type:" + device.Type,
		"device_ip:" + device.IPAddress,
		"device_hostname:" + device.Name,
		"device_id:" + buildDeviceID(namespace, device.IPAddress),
	}
	tags = append(tags, device.ApplianceTags...)

	return tags
}

func buildDirectorDeviceTags(namespace string, ipAddress string, directorStatus *client.DirectorStatus) []string {
	tags := []string{
		"device_vendor:versa",
		"device_namespace:" + namespace,
		"hostname:" + directorStatus.HAConfig.ClusterID, // TODO: should we use IP address instead
		"type:director",
		"device_ip:" + ipAddress,
		"device_hostname:" + directorStatus.HAConfig.ClusterID,
		"device_id:" + buildDeviceID(namespace, ipAddress),
		"startup_mode:" + directorStatus.HAConfig.StartupMode,
	}

	for _, ip := range directorStatus.HAConfig.MyVnfManagementIPs {
		tags = append(tags, "management_ip:"+ip)
	}

	return tags
}

func computeUptime(device client.Appliance) (float64, error) {
	now := TimeNow().UnixMilli()

	// TODO: should we be using lastUpdatedTime instead of startTime?
	parsedTime, err := time.Parse("Mon Jan 2 15:04:05 2006", device.StartTime)
	if err != nil {
		return 0, fmt.Errorf("error parsing device uptime: %w", err)
	}
	deviceUptime := parsedTime.UnixMilli()

	return math.Round((float64(now) - float64(deviceUptime)) / 10), nil // In hundredths of a second, to match SNMP
}

func buildDeviceID(namespace string, deviceID string) string {
	return fmt.Sprintf("%s:%s", namespace, deviceID)
}
