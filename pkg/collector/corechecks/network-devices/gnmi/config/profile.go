// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v2"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const profilesFolder = "profiles"

// MetricType is the Datadog metric submission type for a gNMI metric mapping.
type MetricType string

const (
	// MetricTypeGauge maps to Datadog gauge metrics.
	MetricTypeGauge MetricType = "gauge"
	// MetricTypeMonotonicCount maps to Datadog monotonic count metrics.
	MetricTypeMonotonicCount MetricType = "monotonic_count"
)

var supportedMetricTypes = map[MetricType]struct{}{
	MetricTypeGauge:          {},
	MetricTypeMonotonicCount: {},
}

// MetricConfig maps a gNMI path to a Datadog metric.
type MetricConfig struct {
	Path     string            `yaml:"path"`
	Metric   string            `yaml:"metric"`
	Type     MetricType        `yaml:"type"`
	Keys     map[string]string `yaml:"keys"`
	Tags     map[string]string `yaml:"tags"`
	ValueMap map[string]int    `yaml:"value_map"`
}

// SubscriptionKeys returns keyed path segments to subscribe with wildcards.
// When keys is omitted, tags are used (interface metrics where the tag segment matches the list node).
func (m MetricConfig) SubscriptionKeys() map[string]string {
	source := m.Keys
	if len(source) == 0 {
		source = m.Tags
	}
	if len(source) == 0 {
		return nil
	}
	keys := make(map[string]string, len(source))
	for segment, keyName := range source {
		if keyName == "" {
			continue
		}
		keys[segment] = keyName
	}
	if len(keys) == 0 {
		return nil
	}
	return keys
}

// ProfileDefinition holds the metric mappings loaded from a profile YAML file.
type ProfileDefinition struct {
	Name     string
	Path     string
	Metrics  []MetricConfig `yaml:"metrics"`
	Metadata MetadataConfig `yaml:"metadata"`
	Topology TopologyConfig `yaml:"topology"`
}

type rawProfileDefinition struct {
	Extends  []string       `yaml:"extends"`
	Metrics  []MetricConfig `yaml:"metrics"`
	Metadata MetadataConfig `yaml:"metadata"`
	Topology TopologyConfig `yaml:"topology"`
}

// LoadProfile loads a profile by name or path from the gNMI profiles directory.
func LoadProfile(profileRef string) (*ProfileDefinition, error) {
	profilePath, err := resolveProfilePath(profileRef)
	if err != nil {
		return nil, err
	}

	buf, err := os.ReadFile(profilePath)
	if err != nil {
		return nil, fmt.Errorf("read profile %q: %w", profileRef, err)
	}

	raw := rawProfileDefinition{}
	if err := yaml.Unmarshal(buf, &raw); err != nil {
		return nil, fmt.Errorf("parse profile %q: %w", profileRef, err)
	}

	extensions, err := resolveProfileExtensions(profileRef, raw, nil)
	if err != nil {
		return nil, err
	}

	profile := &ProfileDefinition{
		Name:     profileNameFromPath(profileRef, profilePath),
		Path:     profilePath,
		Metrics:  raw.Metrics,
		Metadata: extensions.Metadata.Resolved(),
		Topology: extensions.Topology,
	}
	if err := validateProfileDefinition(profile); err != nil {
		return nil, fmt.Errorf("validate profile %q: %w", profileRef, err)
	}

	return profile, nil
}

func validateProfileDefinition(profile *ProfileDefinition) error {
	if len(profile.Metrics) == 0 {
		return errors.New("profile must define at least one metric")
	}

	for i, metric := range profile.Metrics {
		if err := validateMetricConfig(metric, i); err != nil {
			return err
		}
	}
	if err := validateMetadataConfig(profile.Metadata); err != nil {
		return err
	}
	return validateTopologyConfig(profile.Topology)
}

type profileExtensionConfig struct {
	Metadata MetadataConfig
	Topology TopologyConfig
}

func resolveProfileExtensions(profileRef string, raw rawProfileDefinition, visited map[string]bool) (profileExtensionConfig, error) {
	if visited == nil {
		visited = make(map[string]bool)
	}
	if visited[profileRef] {
		return profileExtensionConfig{}, fmt.Errorf("cyclic profile extend detected for %q", profileRef)
	}
	visited[profileRef] = true

	extensions := profileExtensionConfig{}
	for _, extendRef := range raw.Extends {
		extendPath, err := resolveProfilePath(extendRef)
		if err != nil {
			return profileExtensionConfig{}, fmt.Errorf("resolve extended profile %q for %q: %w", extendRef, profileRef, err)
		}

		extendBuf, err := os.ReadFile(extendPath)
		if err != nil {
			return profileExtensionConfig{}, fmt.Errorf("read extended profile %q for %q: %w", extendRef, profileRef, err)
		}

		extendRaw := rawProfileDefinition{}
		if err := yaml.Unmarshal(extendBuf, &extendRaw); err != nil {
			return profileExtensionConfig{}, fmt.Errorf("parse extended profile %q for %q: %w", extendRef, profileRef, err)
		}

		extendConfig, err := resolveProfileExtensions(extendRef, extendRaw, visited)
		if err != nil {
			return profileExtensionConfig{}, err
		}
		extensions.Metadata = mergeMetadataConfig(extensions.Metadata, extendConfig.Metadata)
		extensions.Topology = mergeTopologyConfig(extensions.Topology, extendConfig.Topology)
	}

	extensions.Metadata = mergeMetadataConfig(extensions.Metadata, raw.Metadata)
	extensions.Topology = mergeTopologyConfig(extensions.Topology, raw.Topology)
	return extensions, nil
}

func mergeMetadataConfig(base, override MetadataConfig) MetadataConfig {
	merged := base
	merged.Device = mergeDeviceMetadataConfig(base.Device, override.Device)
	merged.Interface = mergeInterfaceMetadataConfig(base.Interface, override.Interface)
	merged.IPAddress = mergeIPAddressMetadataConfig(base.IPAddress, override.IPAddress)
	return merged
}

func mergeDeviceMetadataConfig(base, override DeviceMetadataConfig) DeviceMetadataConfig {
	if len(override.Keys) > 0 {
		base.Keys = copyStringMap(override.Keys)
	}
	if override.Hostname != "" {
		base.Hostname = override.Hostname
	}
	if override.VendorName != "" {
		base.VendorName = override.VendorName
	}
	if override.SerialNumber != "" {
		base.SerialNumber = override.SerialNumber
	}
	if override.Platform != "" {
		base.Platform = override.Platform
	}
	if override.SoftwareVersion != "" {
		base.SoftwareVersion = override.SoftwareVersion
	}
	if override.HardwareVersion != "" {
		base.HardwareVersion = override.HardwareVersion
	}
	return base
}

func mergeInterfaceMetadataConfig(base, override InterfaceMetadataConfig) InterfaceMetadataConfig {
	if len(override.Keys) > 0 {
		base.Keys = copyStringMap(override.Keys)
	}
	if override.Name != "" {
		base.Name = override.Name
	}
	if override.Description != "" {
		base.Description = override.Description
	}
	if override.AdminStatus != "" {
		base.AdminStatus = override.AdminStatus
	}
	if override.OperStatus != "" {
		base.OperStatus = override.OperStatus
	}
	if override.MACAddress != "" {
		base.MACAddress = override.MACAddress
	}
	if override.IfIndex != "" {
		base.IfIndex = override.IfIndex
	}
	if override.Type != "" {
		base.Type = override.Type
	}
	return base
}

func mergeIPAddressMetadataConfig(base, override IPAddressMetadataConfig) IPAddressMetadataConfig {
	if len(override.Keys) > 0 {
		base.Keys = copyStringMap(override.Keys)
	}
	if override.IP != "" {
		base.IP = override.IP
	}
	if override.PrefixLength != "" {
		base.PrefixLength = override.PrefixLength
	}
	if override.IPv6 != "" {
		base.IPv6 = override.IPv6
	}
	if override.IPv6PrefixLength != "" {
		base.IPv6PrefixLength = override.IPv6PrefixLength
	}
	return base
}

func validateMetricConfig(metric MetricConfig, index int) error {
	metricPrefix := fmt.Sprintf("metrics[%d]", index)

	if strings.TrimSpace(metric.Path) == "" {
		return fmt.Errorf("%s: `path` must not be empty", metricPrefix)
	}
	if strings.TrimSpace(metric.Metric) == "" {
		return fmt.Errorf("%s: `metric` must not be empty", metricPrefix)
	}
	if _, ok := supportedMetricTypes[metric.Type]; !ok {
		return fmt.Errorf("%s: unsupported metric `type` %q (supported: gauge, monotonic_count)", metricPrefix, metric.Type)
	}
	return nil
}

func resolveProfilePath(profileRef string) (string, error) {
	if profileRef == "" {
		return "", errors.New("profile reference is empty")
	}

	if filepath.IsAbs(profileRef) {
		return profileRef, nil
	}

	profilesRoot := getProfilesRoot()
	candidate := filepath.Join(profilesRoot, profileRef)
	if fileExists(candidate) {
		return candidate, nil
	}

	withYAMLExt := ensureYAMLExtension(profileRef)
	candidate = filepath.Join(profilesRoot, withYAMLExt)
	if fileExists(candidate) {
		return candidate, nil
	}

	return "", fmt.Errorf("profile %q not found under %q", profileRef, profilesRoot)
}

func getProfilesRoot() string {
	confdPath := pkgconfigsetup.Datadog().GetString("confd_path")
	return filepath.Join(confdPath, "gnmi.d", profilesFolder)
}

func ensureYAMLExtension(profileRef string) string {
	if strings.HasSuffix(profileRef, ".yaml") || strings.HasSuffix(profileRef, ".yml") {
		return profileRef
	}
	return profileRef + ".yaml"
}

func profileNameFromPath(profileRef, profilePath string) string {
	base := filepath.Base(profilePath)
	if strings.HasSuffix(base, ".yaml") {
		return strings.TrimSuffix(base, ".yaml")
	}
	if strings.HasSuffix(base, ".yml") {
		return strings.TrimSuffix(base, ".yml")
	}
	return profileRef
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
