// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	ndmutils "github.com/DataDog/datadog-agent/pkg/networkdevice/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	sortutil "github.com/DataDog/datadog-agent/pkg/util/sort"
)

const (
	// DefaultMetadataCollectionInterval is the default cadence for metadata resubmission.
	DefaultMetadataCollectionInterval = 10 * time.Minute

	pathHostname        = "/system/state/hostname"
	pathVendorName      = "/system/state/vendor-name"
	pathSerialNumber    = "/system/state/serial-number"
	pathPlatform        = "/system/state/platform"
	pathSoftwareVersion = "/system/state/software-version"
	pathHardwareVersion = "/system/state/hardware-version"
)

// ShouldReportMetadata reports whether metadata should be submitted based on the last report time.
func ShouldReportMetadata(lastReport time.Time, interval time.Duration, now time.Time) bool {
	if interval <= 0 {
		interval = DefaultMetadataCollectionInterval
	}
	if lastReport.IsZero() {
		return true
	}
	return !now.Before(lastReport.Add(interval))
}

// MetadataCollectionInterval returns the configured metadata collection interval.
func MetadataCollectionInterval(cfg *config.CheckConfig) time.Duration {
	if cfg == nil || cfg.Instance.MetadataCollectionInterval <= 0 {
		return DefaultMetadataCollectionInterval
	}
	return time.Duration(cfg.Instance.MetadataCollectionInterval) * time.Second
}

// ReportMetadata builds and submits device and interface metadata payloads.
func ReportMetadata(s sender.Sender, cfg *config.CheckConfig, snapshot []client.CachedValue, collectTime time.Time) error {
	if s == nil {
		return errors.New("sender is nil")
	}
	if cfg == nil {
		return errors.New("check config is nil")
	}

	deviceID := buildDeviceID(cfg.Instance.Address)
	tags := sortutil.UniqInPlace(ndmutils.CopyStrings(buildBaseTags(cfg)))

	device := buildDeviceMetadata(deviceID, cfg, snapshot, tags)
	interfaces := buildInterfaceMetadata(deviceID, snapshot)

	var topologyLinks []devicemetadata.TopologyLinkMetadata
	if cfg.Instance.CollectTopology {
		topologyLinks = buildTopologyLinks(deviceID, snapshot, interfaces)
	}

	if isEmptyMetadata(device, interfaces, topologyLinks) {
		log.Debugf("skipping gNMI metadata submission for %s: no metadata available", cfg.Instance.Address)
		return nil
	}

	payloads := devicemetadata.BatchPayloads(
		integrations.Gnmi,
		defaultDeviceNamespace,
		"",
		collectTime,
		devicemetadata.PayloadMetadataBatchSize,
		[]devicemetadata.DeviceMetadata{device},
		interfaces,
		nil,
		topologyLinks,
		nil,
		nil,
		nil,
	)

	for _, payload := range payloads {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		s.EventPlatformEvent(payloadBytes, eventplatform.EventTypeNetworkDevicesMetadata)
	}

	return nil
}

func isEmptyMetadata(device devicemetadata.DeviceMetadata, interfaces []devicemetadata.InterfaceMetadata, links []devicemetadata.TopologyLinkMetadata) bool {
	if len(interfaces) > 0 || len(links) > 0 {
		return false
	}
	return device.Name == "" &&
		device.Vendor == "" &&
		device.SerialNumber == "" &&
		device.ProductName == "" &&
		device.OsVersion == "" &&
		device.Version == ""
}

func buildDeviceMetadata(deviceID string, cfg *config.CheckConfig, snapshot []client.CachedValue, tags []string) devicemetadata.DeviceMetadata {
	index := indexSnapshot(snapshot)

	hostname := firstStringValue(index, pathHostname, nil)
	vendor := firstStringValue(index, pathVendorName, nil)
	serialNumber := firstStringValue(index, pathSerialNumber, nil)
	platform := firstStringValue(index, pathPlatform, nil)
	softwareVersion := firstStringValue(index, pathSoftwareVersion, nil)
	hardwareVersion := firstStringValue(index, pathHardwareVersion, nil)

	productName := platform
	if productName == "" {
		productName = hardwareVersion
	}

	return devicemetadata.DeviceMetadata{
		ID:           deviceID,
		IDTags:       []string{"device_ip:" + cfg.Instance.Address},
		Name:         hostname,
		IPAddress:    cfg.Instance.Address,
		Profile:      cfg.Profile.Name,
		Vendor:       vendor,
		SerialNumber: serialNumber,
		ProductName:  productName,
		Version:      hardwareVersion,
		OsVersion:    softwareVersion,
		Tags:         tags,
		Status:       devicemetadata.DeviceStatusReachable,
		Integration:  string(integrations.Gnmi),
	}
}

func buildInterfaceMetadata(deviceID string, snapshot []client.CachedValue) []devicemetadata.InterfaceMetadata {
	index := indexSnapshot(snapshot)
	names := interfaceNames(index)
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)

	interfaces := make([]devicemetadata.InterfaceMetadata, 0, len(names))
	for i, name := range names {
		keys := map[string]string{"name": name}

		ifIndex, ok := firstInt32Value(index, "/interfaces/interface/state/ifindex", keys)
		if !ok || ifIndex == 0 {
			ifIndex = int32(i + 1)
		}

		ifType, _ := firstInt32Value(index, "/interfaces/interface/state/type", keys)
		if ifType == 0 {
			ifType = parseIANAIfType(firstStringValue(index, "/interfaces/interface/state/type", keys))
		}

		isPhysical := physicalInterface(ifType)

		adminStatus := parseAdminStatus(firstStringValue(index, "/interfaces/interface/state/admin-status", keys))
		operStatus := parseOperStatus(firstStringValue(index, "/interfaces/interface/state/oper-status", keys))

		interfaceName := firstStringValue(index, "/interfaces/interface/state/name", keys)
		if interfaceName == "" {
			interfaceName = name
		}

		interfaces = append(interfaces, devicemetadata.InterfaceMetadata{
			DeviceID:    deviceID,
			Index:       ifIndex,
			Name:        interfaceName,
			Description: firstStringValue(index, "/interfaces/interface/state/description", keys),
			MacAddress:  firstStringValue(index, "/interfaces/interface/state/mac-address", keys),
			AdminStatus: adminStatus,
			OperStatus:  operStatus,
			Type:        ifType,
			IsPhysical:  isPhysical,
		})
	}

	return interfaces
}

func parseAdminStatus(value string) devicemetadata.IfAdminStatus {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "UP":
		return devicemetadata.AdminStatusUp
	case "DOWN":
		return devicemetadata.AdminStatusDown
	case "TESTING":
		return devicemetadata.AdminStatusTesting
	default:
		if parsed, ok := int64Value(value); ok {
			return devicemetadata.IfAdminStatus(parsed)
		}
		return 0
	}
}

func parseOperStatus(value string) devicemetadata.IfOperStatus {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "UP":
		return devicemetadata.OperStatusUp
	case "DOWN":
		return devicemetadata.OperStatusDown
	case "TESTING":
		return devicemetadata.OperStatusTesting
	case "UNKNOWN":
		return devicemetadata.OperStatusUnknown
	case "DORMANT":
		return devicemetadata.OperStatusDormant
	case "NOT_PRESENT":
		return devicemetadata.OperStatusNotPresent
	case "LOWER_LAYER_DOWN":
		return devicemetadata.OperStatusLowerLayerDown
	default:
		if parsed, ok := int64Value(value); ok {
			return devicemetadata.IfOperStatus(parsed)
		}
		return 0
	}
}

func parseIANAIfType(value string) int32 {
	if value == "" {
		return 0
	}
	if parsed, ok := int64Value(value); ok {
		return int32(parsed)
	}

	normalized := strings.ToLower(value)
	if idx := strings.LastIndex(normalized, ":"); idx >= 0 {
		normalized = normalized[idx+1:]
	}

	knownTypes := map[string]int32{
		"ethernetcsmacd":   6,
		"fastether":        62,
		"fastetherfx":      69,
		"gigabitethernet":  117,
		"softwareloopback": 24,
	}
	if ifType, ok := knownTypes[normalized]; ok {
		return ifType
	}
	return 0
}

func physicalInterface(ifType int32) *bool {
	if ifType == 0 {
		return nil
	}
	physical := ifType == 6 || ifType == 62 || ifType == 69 || ifType == 117
	return &physical
}
