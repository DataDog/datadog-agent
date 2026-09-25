// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
)

// PathSubscriptionConfig describes a gNMI path subscription with optional keyed segments.
type PathSubscriptionConfig struct {
	Path string
	Tags map[string]string
}

// DeviceMetadataConfig maps logical device metadata fields to gNMI paths.
type DeviceMetadataConfig struct {
	Keys            map[string]string `yaml:"keys"`
	ComponentType   string            `yaml:"component_type"`
	Hostname        string            `yaml:"hostname"`
	VendorName      string            `yaml:"vendor_name"`
	SerialNumber    string            `yaml:"serial_number"`
	Platform        string            `yaml:"platform"`
	SoftwareVersion string            `yaml:"software_version"`
	HardwareVersion string            `yaml:"hardware_version"`
}

// InterfaceMetadataConfig maps logical interface metadata fields to gNMI paths.
type InterfaceMetadataConfig struct {
	Keys        map[string]string `yaml:"keys"`
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	AdminStatus string            `yaml:"admin_status"`
	OperStatus  string            `yaml:"oper_status"`
	MACAddress  string            `yaml:"mac_address"`
	IfIndex     string            `yaml:"ifindex"`
	Type        string            `yaml:"type"`
}

// IPAddressMetadataConfig maps logical interface IP metadata fields to gNMI paths.
type IPAddressMetadataConfig struct {
	Keys             map[string]string `yaml:"keys"`
	IP               string            `yaml:"ip"`
	PrefixLength     string            `yaml:"prefix_length"`
	IPv6             string            `yaml:"ipv6"`
	IPv6PrefixLength string            `yaml:"ipv6_prefix_length"`
}

// MetadataConfig defines gNMI paths used for NDM device and interface metadata.
type MetadataConfig struct {
	Device    DeviceMetadataConfig    `yaml:"device"`
	Interface InterfaceMetadataConfig `yaml:"interface"`
	IPAddress IPAddressMetadataConfig `yaml:"ip_address"`
}

// DefaultOpenConfigMetadata returns the standard OpenConfig metadata path mappings.
func DefaultOpenConfigMetadata() MetadataConfig {
	return MetadataConfig{
		Device: DeviceMetadataConfig{
			Keys: map[string]string{
				"component": "name",
			},
			ComponentType:   "/openconfig/components/component/state/type",
			Hostname:        "/openconfig/system/state/hostname",
			VendorName:      "/openconfig/components/component/state/mfg-name",
			SerialNumber:    "/openconfig/components/component/state/serial-no",
			Platform:        "/openconfig/components/component/state/model-name",
			SoftwareVersion: "/openconfig/system/state/software-version",
			HardwareVersion: "/openconfig/components/component/state/part-no",
		},
		Interface: InterfaceMetadataConfig{
			Keys: map[string]string{
				"interface": "name",
			},
			Name:        "/openconfig/interfaces/interface/state/name",
			Description: "/openconfig/interfaces/interface/state/description",
			AdminStatus: "/openconfig/interfaces/interface/state/admin-status",
			OperStatus:  "/openconfig/interfaces/interface/state/oper-status",
			MACAddress:  "/openconfig/interfaces/interface/ethernet/state/hw-mac-address",
			IfIndex:     "/openconfig/interfaces/interface/state/ifindex",
			Type:        "/openconfig/interfaces/interface/state/type",
		},
		IPAddress: IPAddressMetadataConfig{
			Keys: map[string]string{
				"interface":    "name",
				"subinterface": "index",
				"address":      "ip",
			},
			IP:               "/openconfig/interfaces/interface/subinterfaces/subinterface/ipv4/addresses/address/state/ip",
			PrefixLength:     "/openconfig/interfaces/interface/subinterfaces/subinterface/ipv4/addresses/address/state/prefix-length",
			IPv6:             "/openconfig/interfaces/interface/subinterfaces/subinterface/ipv6/addresses/address/state/ip",
			IPv6PrefixLength: "/openconfig/interfaces/interface/subinterfaces/subinterface/ipv6/addresses/address/state/prefix-length",
		},
	}
}

// Resolved returns the configured metadata or the OpenConfig defaults when unset.
func (m MetadataConfig) Resolved() MetadataConfig {
	if m.IsZero() {
		return DefaultOpenConfigMetadata()
	}
	return m
}

// IsZero reports whether the metadata config was omitted from the profile.
func (m MetadataConfig) IsZero() bool {
	return deviceMetadataConfigIsZero(m.Device) &&
		m.Interface.Name == "" &&
		m.Interface.Description == "" &&
		m.Interface.AdminStatus == "" &&
		m.Interface.OperStatus == "" &&
		m.Interface.MACAddress == "" &&
		m.Interface.IfIndex == "" &&
		m.Interface.Type == "" &&
		len(m.Interface.Keys) == 0 &&
		m.IPAddress.IP == "" &&
		m.IPAddress.PrefixLength == "" &&
		m.IPAddress.IPv6 == "" &&
		m.IPAddress.IPv6PrefixLength == "" &&
		len(m.IPAddress.Keys) == 0
}

func deviceMetadataConfigIsZero(device DeviceMetadataConfig) bool {
	return len(device.Keys) == 0 &&
		device.ComponentType == "" &&
		device.Hostname == "" &&
		device.VendorName == "" &&
		device.SerialNumber == "" &&
		device.Platform == "" &&
		device.SoftwareVersion == "" &&
		device.HardwareVersion == ""
}

// SubscriptionPaths returns deduplicated metadata paths to subscribe to.
func (m MetadataConfig) SubscriptionPaths() []PathSubscriptionConfig {
	resolved := m.Resolved()
	paths := [...]string{
		resolved.Device.ComponentType,
		resolved.Device.Hostname,
		resolved.Device.VendorName,
		resolved.Device.SerialNumber,
		resolved.Device.Platform,
		resolved.Device.SoftwareVersion,
		resolved.Device.HardwareVersion,
		resolved.Interface.Name,
		resolved.Interface.Description,
		resolved.Interface.AdminStatus,
		resolved.Interface.OperStatus,
		resolved.Interface.MACAddress,
		resolved.Interface.IfIndex,
		resolved.Interface.Type,
		resolved.IPAddress.IP,
		resolved.IPAddress.PrefixLength,
		resolved.IPAddress.IPv6,
		resolved.IPAddress.IPv6PrefixLength,
	}
	seen := make(map[string]struct{})
	out := make([]PathSubscriptionConfig, 0, len(paths))
	for _, path := range paths {
		path = normalizeMetadataPath(path)
		if path == "" {
			continue
		}
		tags := metadataSubscriptionKeys(resolved, path)
		seenKey := path + formatMetadataSubscriptionKey(tags)
		if _, ok := seen[seenKey]; ok {
			continue
		}
		seen[seenKey] = struct{}{}

		spec := PathSubscriptionConfig{Path: path}
		if len(tags) > 0 {
			spec.Tags = copyStringMap(tags)
		}
		out = append(out, spec)
	}
	return out
}

// InterfaceLookupPaths returns interface metadata paths used to discover interface names.
func (m MetadataConfig) InterfaceLookupPaths() []string {
	resolved := m.Resolved()
	paths := []string{
		resolved.Interface.Name,
		resolved.Interface.Description,
		resolved.Interface.AdminStatus,
		resolved.Interface.OperStatus,
		resolved.Interface.MACAddress,
		resolved.Interface.IfIndex,
		resolved.Interface.Type,
	}
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = normalizeMetadataPath(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

// InterfaceKeyValues builds cache lookup keys for a named interface.
func (m MetadataConfig) InterfaceKeyValues(interfaceName string) map[string]string {
	resolved := m.Resolved()
	if interfaceName == "" || len(resolved.Interface.Keys) == 0 {
		return map[string]string{"name": interfaceName}
	}

	keys := make(map[string]string, len(resolved.Interface.Keys))
	for _, keyName := range resolved.Interface.Keys {
		if keyName == "" {
			continue
		}
		keys[keyName] = interfaceName
	}
	if len(keys) == 0 {
		return map[string]string{"name": interfaceName}
	}
	return keys
}

// DeviceKeyValues builds cache lookup keys for a named device component.
func (m MetadataConfig) DeviceKeyValues(componentName string) map[string]string {
	resolved := m.Resolved()
	if componentName == "" || len(resolved.Device.Keys) == 0 {
		return nil
	}

	keys := make(map[string]string, len(resolved.Device.Keys))
	for _, keyName := range resolved.Device.Keys {
		if keyName == "" {
			continue
		}
		keys[keyName] = componentName
	}
	if len(keys) == 0 {
		return map[string]string{"name": componentName}
	}
	return keys
}

func metadataSubscriptionKeys(resolved MetadataConfig, path string) map[string]string {
	normalized := normalizeMetadataPath(path)
	switch {
	case normalized == normalizeMetadataPath(resolved.IPAddress.IP),
		normalized == normalizeMetadataPath(resolved.IPAddress.PrefixLength),
		normalized == normalizeMetadataPath(resolved.IPAddress.IPv6),
		normalized == normalizeMetadataPath(resolved.IPAddress.IPv6PrefixLength):
		return copyStringMap(resolved.IPAddress.Keys)
	case strings.Contains(path, "/interfaces/interface/"):
		return copyStringMap(resolved.Interface.Keys)
	case strings.Contains(path, "/components/component/"):
		return copyStringMap(resolved.Device.Keys)
	default:
		return nil
	}
}

func formatMetadataSubscriptionKey(tags map[string]string) string {
	if len(tags) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tags))
	for segment, keyName := range tags {
		parts = append(parts, segment+":"+keyName)
	}
	sort.Strings(parts)
	return "{" + strings.Join(parts, ",") + "}"
}

func validateMetadataConfig(metadata MetadataConfig) error {
	resolved := metadata.Resolved()
	paths := [...]struct {
		field string
		path  string
	}{
		{"device.component_type", resolved.Device.ComponentType},
		{"device.hostname", resolved.Device.Hostname},
		{"device.vendor_name", resolved.Device.VendorName},
		{"device.serial_number", resolved.Device.SerialNumber},
		{"device.platform", resolved.Device.Platform},
		{"device.software_version", resolved.Device.SoftwareVersion},
		{"device.hardware_version", resolved.Device.HardwareVersion},
		{"interface.name", resolved.Interface.Name},
		{"interface.description", resolved.Interface.Description},
		{"interface.admin_status", resolved.Interface.AdminStatus},
		{"interface.oper_status", resolved.Interface.OperStatus},
		{"interface.mac_address", resolved.Interface.MACAddress},
		{"interface.ifindex", resolved.Interface.IfIndex},
		{"interface.type", resolved.Interface.Type},
		{"ip_address.ip", resolved.IPAddress.IP},
		{"ip_address.prefix_length", resolved.IPAddress.PrefixLength},
		{"ip_address.ipv6", resolved.IPAddress.IPv6},
		{"ip_address.ipv6_prefix_length", resolved.IPAddress.IPv6PrefixLength},
	}
	for _, item := range paths {
		if strings.TrimSpace(item.path) == "" {
			continue
		}
		if err := validateMetadataPath(item.path, item.field); err != nil {
			return err
		}
	}

	if deviceUsesComponentPaths(resolved.Device) {
		if resolved.Device.ComponentType == "" {
			return errors.New("metadata.device.component_type is required for component metadata paths")
		}
		if len(resolved.Device.Keys) == 0 {
			return errors.New("metadata.device.keys must define at least one keyed segment for component metadata paths")
		}
	}

	if resolved.Interface.IfIndex == "" {
		return errors.New("metadata.interface.ifindex is required")
	}
	if len(resolved.Interface.Keys) == 0 {
		return errors.New("metadata.interface.keys must define at least one keyed segment")
	}
	for segment, keyName := range resolved.Interface.Keys {
		if strings.TrimSpace(segment) == "" {
			return errors.New("metadata.interface.keys contains an empty segment name")
		}
		if strings.TrimSpace(keyName) == "" {
			return fmt.Errorf("metadata.interface.keys[%q] must not be empty", segment)
		}
	}
	if err := validateUniquePathKeyNames(resolved.Interface.Keys, "metadata.interface.keys"); err != nil {
		return err
	}
	for segment, keyName := range resolved.IPAddress.Keys {
		if strings.TrimSpace(segment) == "" {
			return errors.New("metadata.ip_address.keys contains an empty segment name")
		}
		if strings.TrimSpace(keyName) == "" {
			return fmt.Errorf("metadata.ip_address.keys[%q] must not be empty", segment)
		}
	}
	if err := validateUniquePathKeyNames(resolved.IPAddress.Keys, "metadata.ip_address.keys"); err != nil {
		return err
	}
	return validateUniquePathKeyNames(resolved.Device.Keys, "metadata.device.keys")
}

func deviceUsesComponentPaths(device DeviceMetadataConfig) bool {
	for _, path := range []string{
		device.Hostname,
		device.VendorName,
		device.SerialNumber,
		device.Platform,
		device.SoftwareVersion,
		device.HardwareVersion,
	} {
		if strings.Contains(normalizeMetadataPath(path), "/components/component/") {
			return true
		}
	}
	return false
}

func validateMetadataPath(path, field string) error {
	trimmed := normalizeMetadataPath(path)
	if trimmed == "" {
		return fmt.Errorf("metadata.%s must not be empty", field)
	}
	if !strings.HasPrefix(trimmed, "/") {
		return fmt.Errorf("metadata.%s must start with '/'", field)
	}
	return nil
}

func normalizeMetadataPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if trimmed[0] != '/' {
		return "/" + trimmed
	}
	return trimmed
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	return maps.Clone(in)
}
