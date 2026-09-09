// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"fmt"
	"strings"
)

// LLDPNeighborConfig maps logical LLDP neighbor fields to gNMI paths.
type LLDPNeighborConfig struct {
	Keys               map[string]string `yaml:"keys"`
	ChassisID          string            `yaml:"chassis_id"`
	ChassisIDType      string            `yaml:"chassis_id_type"`
	PortID             string            `yaml:"port_id"`
	PortIDType         string            `yaml:"port_id_type"`
	SystemName         string            `yaml:"system_name"`
	SystemDescription  string            `yaml:"system_description"`
	PortDescription    string            `yaml:"port_description"`
	ManagementAddress  string            `yaml:"management_address"`
}

// TopologyConfig defines gNMI paths used for NDM topology link metadata.
type TopologyConfig struct {
	LLDP LLDPNeighborConfig `yaml:"lldp"`
}

// DefaultOpenConfigLLDP returns the standard OpenConfig LLDP path mappings.
func DefaultOpenConfigLLDP() TopologyConfig {
	return TopologyConfig{
		LLDP: LLDPNeighborConfig{
			Keys: map[string]string{
				"interface": "name",
				"neighbor":  "id",
			},
			ChassisID:         "/openconfig/lldp/interfaces/interface/neighbors/neighbor/state/chassis-id",
			ChassisIDType:     "/openconfig/lldp/interfaces/interface/neighbors/neighbor/state/chassis-id-type",
			PortID:            "/openconfig/lldp/interfaces/interface/neighbors/neighbor/state/port-id",
			PortIDType:        "/openconfig/lldp/interfaces/interface/neighbors/neighbor/state/port-id-type",
			SystemName:        "/openconfig/lldp/interfaces/interface/neighbors/neighbor/state/system-name",
			SystemDescription: "/openconfig/lldp/interfaces/interface/neighbors/neighbor/state/system-description",
			PortDescription:   "/openconfig/lldp/interfaces/interface/neighbors/neighbor/state/port-description",
			ManagementAddress: "/openconfig/lldp/interfaces/interface/neighbors/neighbor/state/management-address",
		},
	}
}

// IsZero reports whether topology collection paths were omitted from the profile.
func (t TopologyConfig) IsZero() bool {
	return t.LLDP.ChassisID == "" &&
		t.LLDP.ChassisIDType == "" &&
		t.LLDP.PortID == "" &&
		t.LLDP.PortIDType == "" &&
		t.LLDP.SystemName == "" &&
		t.LLDP.SystemDescription == "" &&
		t.LLDP.PortDescription == "" &&
		t.LLDP.ManagementAddress == "" &&
		len(t.LLDP.Keys) == 0
}

// SubscriptionPaths returns deduplicated topology paths to subscribe to.
func (t TopologyConfig) SubscriptionPaths() []PathSubscriptionConfig {
	if t.IsZero() {
		return nil
	}

	seen := make(map[string]struct{})
	out := make([]PathSubscriptionConfig, 0, 8)

	add := func(path string, keyed bool) {
		path = normalizeMetadataPath(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}

		spec := PathSubscriptionConfig{Path: path}
		if keyed && len(t.LLDP.Keys) > 0 {
			spec.Tags = copyStringMap(t.LLDP.Keys)
		}
		out = append(out, spec)
	}

	add(t.LLDP.ChassisID, true)
	add(t.LLDP.ChassisIDType, true)
	add(t.LLDP.PortID, true)
	add(t.LLDP.PortIDType, true)
	add(t.LLDP.SystemName, true)
	add(t.LLDP.SystemDescription, true)
	add(t.LLDP.PortDescription, true)
	add(t.LLDP.ManagementAddress, true)

	return out
}

// LookupPaths returns LLDP neighbor paths used to discover neighbors in the cache.
func (t TopologyConfig) LookupPaths() []string {
	if t.IsZero() {
		return nil
	}

	paths := []string{
		t.LLDP.ChassisID,
		t.LLDP.ChassisIDType,
		t.LLDP.PortID,
		t.LLDP.PortIDType,
		t.LLDP.SystemName,
		t.LLDP.SystemDescription,
		t.LLDP.PortDescription,
		t.LLDP.ManagementAddress,
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

// NeighborInterfaceKey returns the cache key name for the local interface segment.
func (t TopologyConfig) NeighborInterfaceKey() string {
	return neighborKeyName(t.LLDP.Keys, "interface", "name")
}

// NeighborIDKey returns the cache key name for the remote neighbor segment.
func (t TopologyConfig) NeighborIDKey() string {
	return neighborKeyName(t.LLDP.Keys, "neighbor", "id")
}

func neighborKeyName(keys map[string]string, segment, fallback string) string {
	if keyName, ok := keys[segment]; ok && keyName != "" {
		return keyName
	}
	return fallback
}

func validateTopologyConfig(topology TopologyConfig) error {
	if topology.IsZero() {
		return nil
	}

	for field, path := range map[string]string{
		"lldp.chassis_id":          topology.LLDP.ChassisID,
		"lldp.chassis_id_type":     topology.LLDP.ChassisIDType,
		"lldp.port_id":             topology.LLDP.PortID,
		"lldp.port_id_type":        topology.LLDP.PortIDType,
		"lldp.system_name":         topology.LLDP.SystemName,
		"lldp.system_description":  topology.LLDP.SystemDescription,
		"lldp.port_description":    topology.LLDP.PortDescription,
		"lldp.management_address": topology.LLDP.ManagementAddress,
	} {
		if err := validateMetadataPath(path, field); err != nil {
			return err
		}
	}

	if len(topology.LLDP.Keys) == 0 {
		return fmt.Errorf("topology.lldp.keys must define at least one keyed segment")
	}
	for segment, keyName := range topology.LLDP.Keys {
		if strings.TrimSpace(segment) == "" {
			return fmt.Errorf("topology.lldp.keys contains an empty segment name")
		}
		if strings.TrimSpace(keyName) == "" {
			return fmt.Errorf("topology.lldp.keys[%q] must not be empty", segment)
		}
	}
	return nil
}

func mergeTopologyConfig(base, override TopologyConfig) TopologyConfig {
	merged := base
	merged.LLDP = mergeLLDPNeighborConfig(base.LLDP, override.LLDP)
	return merged
}

func mergeLLDPNeighborConfig(base, override LLDPNeighborConfig) LLDPNeighborConfig {
	if len(override.Keys) > 0 {
		base.Keys = copyStringMap(override.Keys)
	}
	if override.ChassisID != "" {
		base.ChassisID = override.ChassisID
	}
	if override.ChassisIDType != "" {
		base.ChassisIDType = override.ChassisIDType
	}
	if override.PortID != "" {
		base.PortID = override.PortID
	}
	if override.PortIDType != "" {
		base.PortIDType = override.PortIDType
	}
	if override.SystemName != "" {
		base.SystemName = override.SystemName
	}
	if override.SystemDescription != "" {
		base.SystemDescription = override.SystemDescription
	}
	if override.PortDescription != "" {
		base.PortDescription = override.PortDescription
	}
	if override.ManagementAddress != "" {
		base.ManagementAddress = override.ManagementAddress
	}
	return base
}
