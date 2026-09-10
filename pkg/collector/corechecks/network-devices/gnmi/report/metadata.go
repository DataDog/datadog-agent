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

	interfaceStatusMetric = "snmp.interface.status"

	// gnmiInterfaceRawIDType identifies the RawID scheme (deviceID:interfaceName) used for
	// gNMI interfaces, matching the naming convention used by other integrations
	// (e.g. "versa_interface").
	gnmiInterfaceRawIDType = "gnmi_interface"
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

// InterfaceSnapshotComplete reports whether every discovered interface has a name in the snapshot.
func InterfaceSnapshotComplete(snapshot []client.CachedValue, metadata config.MetadataConfig) bool {
	index := indexSnapshot(snapshot)
	names := interfaceNames(index, metadata, nil)
	if len(names) == 0 {
		return true
	}

	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return false
		}
	}
	return true
}

// ReportMetadata builds and submits device and interface metadata payloads.
// The returned bool is true when at least one metadata payload was sent.
func ReportMetadata(s sender.Sender, cfg *config.CheckConfig, snapshot []client.CachedValue, collectTime time.Time) (bool, error) {
	if s == nil {
		return false, errors.New("sender is nil")
	}
	if cfg == nil {
		return false, errors.New("check config is nil")
	}

	deviceID := buildDeviceID(cfg)
	tags := sortutil.UniqInPlace(ndmutils.CopyStrings(buildBaseTags(cfg, snapshot)))

	device := buildDeviceMetadata(deviceID, cfg, snapshot, tags)
	interfaces := buildInterfaceMetadata(deviceID, cfg.Profile.Metadata, snapshot, interfaceMetricPathsFromProfile(cfg.Profile))
	ipAddresses := buildIPAddressMetadata(deviceID, cfg.Profile.Metadata, interfaces, snapshot)

	var topologyLinks []devicemetadata.TopologyLinkMetadata
	if cfg.Instance.CollectTopology {
		topologyLinks = buildTopologyLinks(deviceID, cfg.Profile.Topology, snapshot, interfaces)
	}

	if isEmptyMetadata(device, interfaces, topologyLinks) {
		log.Debugf("skipping gNMI metadata submission for %s: no metadata available", cfg.Instance.Address)
		return false, nil
	}

	payloads := devicemetadata.BatchPayloads(
		integrations.Gnmi,
		defaultDeviceNamespace,
		"",
		collectTime,
		devicemetadata.PayloadMetadataBatchSize,
		[]devicemetadata.DeviceMetadata{device},
		interfaces,
		ipAddresses,
		topologyLinks,
		nil,
		nil,
		nil,
	)

	for _, payload := range payloads {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return false, err
		}
		s.EventPlatformEvent(payloadBytes, eventplatform.EventTypeNetworkDevicesMetadata)
	}

	return true, nil
}

// ReportInterfaceStatus emits snmp.interface.status for each interface in the snapshot.
func ReportInterfaceStatus(s sender.Sender, cfg *config.CheckConfig, snapshot []client.CachedValue) error {
	if s == nil {
		return errors.New("sender is nil")
	}
	if cfg == nil {
		return errors.New("check config is nil")
	}

	deviceID := buildDeviceID(cfg)
	interfaces := buildInterfaceMetadata(deviceID, cfg.Profile.Metadata, snapshot, interfaceMetricPathsFromProfile(cfg.Profile))
	if len(interfaces) == 0 {
		return nil
	}

	baseTags := buildBaseTags(cfg, snapshot)
	for _, iface := range interfaces {
		status := string(computeInterfaceStatus(iface.AdminStatus, iface.OperStatus))
		tags := []string{
			"status:" + status,
			"admin_status:" + iface.AdminStatus.AsString(),
			"oper_status:" + iface.OperStatus.AsString(),
		}
		if iface.Name != "" {
			tags = append(tags, "interface:"+iface.Name)
		}
		if iface.Description != "" {
			tags = append(tags, "interface_alias:"+iface.Description)
		}
		tags = append(tags, baseTags...)
		if iface.Name != "" {
			tags = append(tags, internalInterfaceResourceTag(deviceID, iface.Name))
		}

		s.Gauge(interfaceStatusMetric, 1, "", tags)
	}

	return nil
}

func computeInterfaceStatus(adminStatus devicemetadata.IfAdminStatus, operStatus devicemetadata.IfOperStatus) devicemetadata.InterfaceStatus {
	if adminStatus == devicemetadata.AdminStatusUp {
		switch {
		case operStatus == devicemetadata.OperStatusUp:
			return devicemetadata.InterfaceStatusUp
		case operStatus == devicemetadata.OperStatusDown:
			return devicemetadata.InterfaceStatusDown
		}
		return devicemetadata.InterfaceStatusWarning
	}
	if adminStatus == devicemetadata.AdminStatusDown {
		switch {
		case operStatus == devicemetadata.OperStatusUp:
			return devicemetadata.InterfaceStatusDown
		case operStatus == devicemetadata.OperStatusDown:
			return devicemetadata.InterfaceStatusOff
		}
		return devicemetadata.InterfaceStatusWarning
	}
	if adminStatus == devicemetadata.AdminStatusTesting {
		switch {
		case operStatus != devicemetadata.OperStatusDown:
			return devicemetadata.InterfaceStatusWarning
		}
	}
	return devicemetadata.InterfaceStatusDown
}

func isEmptyMetadata(device devicemetadata.DeviceMetadata, interfaces []devicemetadata.InterfaceMetadata, links []devicemetadata.TopologyLinkMetadata) bool {
	if len(interfaces) > 0 || len(links) > 0 {
		return false
	}
	return device.OsHostname == "" &&
		device.Vendor == "" &&
		device.SerialNumber == "" &&
		device.ProductName == "" &&
		device.OsVersion == "" &&
		device.Version == ""
}

func buildDeviceMetadata(deviceID string, cfg *config.CheckConfig, snapshot []client.CachedValue, tags []string) devicemetadata.DeviceMetadata {
	index := indexSnapshot(snapshot)
	metadata := cfg.Profile.Metadata.Resolved()

	hostname := resolveDeviceHostname(index, metadata)
	componentName := resolveDeviceComponentName(index, metadata)
	deviceKeys := metadata.DeviceKeyValues(componentName)

	vendor := firstStringValue(index, metadata.Device.VendorName, deviceKeys)
	serialNumber := firstStringValue(index, metadata.Device.SerialNumber, deviceKeys)
	platform := firstStringValue(index, metadata.Device.Platform, deviceKeys)
	softwareVersion := firstStringValue(index, metadata.Device.SoftwareVersion, nil)
	hardwareVersion := firstStringValue(index, metadata.Device.HardwareVersion, deviceKeys)

	productName := platform
	if productName == "" {
		productName = hardwareVersion
	}

	deviceName := hostname
	if deviceName == "" {
		deviceName = cfg.Instance.Address
	}

	return devicemetadata.DeviceMetadata{
		ID:           deviceID,
		IDTags:       buildDeviceIDTags(cfg),
		Name:         deviceName,
		OsHostname:   hostname,
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

func buildInterfaceMetadata(deviceID string, metadata config.MetadataConfig, snapshot []client.CachedValue, metricPaths []string) []devicemetadata.InterfaceMetadata {
	resolved := metadata.Resolved()
	index := indexSnapshot(snapshot)
	names := interfaceNames(index, metadata, metricPaths)
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)

	interfaces := make([]devicemetadata.InterfaceMetadata, 0, len(names))
	for _, name := range names {
		keys := metadata.InterfaceKeyValues(name)

		interfaceName := firstStringValue(index, resolved.Interface.Name, keys)
		if interfaceName == "" {
			interfaceName = name
		}
		if interfaceName == "" {
			log.Debugf("skipping interface metadata for %s: missing interface name", deviceID)
			continue
		}

		ifIndex, _ := firstInt32Value(index, resolved.Interface.IfIndex, keys)

		ifType, _ := firstInt32Value(index, resolved.Interface.Type, keys)
		if ifType == 0 {
			ifType = parseIANAIfType(firstStringValue(index, resolved.Interface.Type, keys))
		}

		isPhysical := physicalInterface(ifType)

		adminStatus := parseAdminStatus(firstStringValue(index, resolved.Interface.AdminStatus, keys))
		operStatus := parseOperStatus(firstStringValue(index, resolved.Interface.OperStatus, keys))

		interfaces = append(interfaces, devicemetadata.InterfaceMetadata{
			DeviceID:    deviceID,
			IDTags:      []string{"interface:" + interfaceName},
			Index:       ifIndex,
			RawID:       buildInterfaceID(deviceID, interfaceName),
			RawIDType:   gnmiInterfaceRawIDType,
			Name:        interfaceName,
			Description: firstStringValue(index, resolved.Interface.Description, keys),
			MacAddress:  firstStringValue(index, resolved.Interface.MACAddress, keys),
			AdminStatus: adminStatus,
			OperStatus:  operStatus,
			Type:        ifType,
			IsPhysical:  isPhysical,
		})
	}

	return interfaces
}

func buildIPAddressMetadata(
	deviceID string,
	metadata config.MetadataConfig,
	interfaces []devicemetadata.InterfaceMetadata,
	snapshot []client.CachedValue,
) []devicemetadata.IPAddressMetadata {
	resolved := metadata.Resolved()
	interfaceKey := interfaceNameKeyFromMetadataKeys(resolved.IPAddress.Keys)

	interfaceByName := make(map[string]devicemetadata.InterfaceMetadata, len(interfaces))
	for _, iface := range interfaces {
		if iface.Name == "" {
			continue
		}
		interfaceByName[iface.Name] = iface
	}

	var ipAddresses []devicemetadata.IPAddressMetadata
	ipAddresses = append(ipAddresses, buildIPAddressMetadataForFamily(
		deviceID, resolved.IPAddress.IP, resolved.IPAddress.PrefixLength, interfaceKey, interfaceByName, snapshot)...)
	ipAddresses = append(ipAddresses, buildIPAddressMetadataForFamily(
		deviceID, resolved.IPAddress.IPv6, resolved.IPAddress.IPv6PrefixLength, interfaceKey, interfaceByName, snapshot)...)

	sort.Slice(ipAddresses, func(i, j int) bool {
		if ipAddresses[i].InterfaceID == ipAddresses[j].InterfaceID {
			return ipAddresses[i].IPAddress < ipAddresses[j].IPAddress
		}
		return ipAddresses[i].InterfaceID < ipAddresses[j].InterfaceID
	})

	return ipAddresses
}

// buildIPAddressMetadataForFamily correlates cached IP and prefix-length values for a single
// address family (IPv4 or IPv6) into IPAddressMetadata entries, keyed by interface/subinterface/address.
func buildIPAddressMetadataForFamily(
	deviceID string,
	ipPathConfig string,
	prefixPathConfig string,
	interfaceKey string,
	interfaceByName map[string]devicemetadata.InterfaceMetadata,
	snapshot []client.CachedValue,
) []devicemetadata.IPAddressMetadata {
	ipPath := normalizeProfilePath(ipPathConfig)
	if ipPath == "" {
		return nil
	}
	prefixPath := normalizeProfilePath(prefixPathConfig)

	type addressData struct {
		ip        string
		prefixLen int32
		hasPrefix bool
	}

	addresses := make(map[string]*addressData)
	addressKey := func(keys map[string]string) string {
		interfaceName := keys[interfaceKey]
		if interfaceName == "" {
			return ""
		}
		parts := []string{interfaceName}
		if subinterfaceIndex := keys["index"]; subinterfaceIndex != "" {
			parts = append(parts, subinterfaceIndex)
		}
		if ip := keys["ip"]; ip != "" {
			parts = append(parts, ip)
		}
		return strings.Join(parts, "\x00")
	}

	for _, cached := range snapshot {
		key := addressKey(cached.Key.Keys)
		if key == "" {
			continue
		}
		entry := addresses[key]
		if entry == nil {
			entry = &addressData{}
			addresses[key] = entry
		}

		switch cached.Key.Path {
		case ipPath:
			if value, ok := stringValue(cached.Entry.Value); ok {
				entry.ip = value
			}
			if entry.ip == "" {
				entry.ip = cached.Key.Keys["ip"]
			}
		case prefixPath:
			if value, ok := int64Value(cached.Entry.Value); ok {
				entry.prefixLen = int32(value)
				entry.hasPrefix = true
			}
		}
	}

	ipAddresses := make([]devicemetadata.IPAddressMetadata, 0, len(addresses))
	for key, entry := range addresses {
		if entry.ip == "" {
			continue
		}
		interfaceName := strings.Split(key, "\x00")[0]
		iface, ok := interfaceByName[interfaceName]
		if !ok || iface.Name == "" {
			log.Debugf("skipping interface IP %s for %s: missing interface metadata for %q", entry.ip, deviceID, interfaceName)
			continue
		}
		if iface.Index == 0 {
			log.Debugf("skipping interface IP %s for %s: interface %q has no ifIndex", entry.ip, deviceID, interfaceName)
			continue
		}

		ipMetadata := devicemetadata.IPAddressMetadata{
			InterfaceID: buildInterfaceIDFromIndex(deviceID, iface.Index),
			IPAddress:   entry.ip,
		}
		if entry.hasPrefix {
			ipMetadata.Prefixlen = entry.prefixLen
		}
		ipAddresses = append(ipAddresses, ipMetadata)
	}

	return ipAddresses
}

func interfaceNameKeyFromMetadataKeys(keys map[string]string) string {
	if keyName, ok := keys["interface"]; ok && keyName != "" {
		return keyName
	}
	for _, keyName := range keys {
		if keyName != "" {
			return keyName
		}
	}
	return "name"
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
