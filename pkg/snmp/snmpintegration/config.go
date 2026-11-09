// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//nolint:revive // TODO(NDM) Fix revive linter
package snmpintegration

import (
	"encoding/json"
	"fmt"
)

// InterfaceConfig interface related configs (e.g. interface speed override)
type InterfaceConfig struct {
	MatchField string   `mapstructure:"match_field" yaml:"match_field" json:"match_field"` // e.g. name, index
	MatchValue string   `mapstructure:"match_value" yaml:"match_value" json:"match_value"` // e.g. eth0 (name), 10 (index)
	InSpeed    uint64   `mapstructure:"in_speed" yaml:"in_speed" json:"in_speed"`          // inbound speed override in bps
	OutSpeed   uint64   `mapstructure:"out_speed" yaml:"out_speed" json:"out_speed"`       // outbound speed override in bps
	Tags       []string `mapstructure:"tags" yaml:"tags" json:"tags"`                      // interface tags
	Disabled   bool     `mapstructure:"disabled" yaml:"disabled" json:"disabled"`          // disables monitoring
}

// PingConfig encapsulates the configuration for ping
type PingConfig struct {
	Linux    PingLinuxConfig `mapstructure:"linux" yaml:"linux" json:"linux"`
	Enabled  *bool           `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	Interval *int            `mapstructure:"interval" yaml:"interval" json:"interval"`
	Timeout  *int            `mapstructure:"timeout" yaml:"timeout" json:"timeout"`
	Count    *int            `mapstructure:"count" yaml:"count" json:"count"`
}

type PingLinuxConfig struct {
	UseRawSocket *bool `mapstructure:"use_raw_socket" yaml:"use_raw_socket" json:"use_raw_socket"`
}

type PackedPingConfig PingConfig

func (pc *PackedPingConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var pingCfg PingConfig
	err := unmarshal(&pingCfg)
	// Needed for autodiscovery case where the passed config will be a string
	if err != nil {
		var pingCfgJSON string
		if err = unmarshal(&pingCfgJSON); err != nil {
			return fmt.Errorf("cannot unmarshal to string: %s", err)
		}
		if pingCfgJSON == "" {
			return nil
		}
		if err = json.Unmarshal([]byte(pingCfgJSON), &pingCfg); err != nil {
			return fmt.Errorf("cannot unmarshal json to snmpintegration.PingConfig: %s", err)
		}
	}

	*pc = PackedPingConfig(pingCfg)
	return nil
}

// DeviceTagsSource controls where the device tags on SNMP metrics come from.
type DeviceTagsSource string

const (
	// DeviceTagsSourceResource attaches only the `dd.internal.resource:ndm_device` tag to metrics
	// and lets the backend enrich them with the device tags from the metadata payload.
	DeviceTagsSourceResource DeviceTagsSource = "resource"
	// DeviceTagsSourceAgent stamps the device tags on every metric and omits the resource tag,
	// so no backend enrichment happens.
	DeviceTagsSourceAgent DeviceTagsSource = "agent"
	// DeviceTagsSourceBoth stamps the device tags on every metric and also sends the resource tag.
	// The backend enrichment still applies on top, which can surface both the previous and the new
	// value of a tag while the metadata payload catches up with a change.
	DeviceTagsSourceBoth DeviceTagsSource = "both"

	// DefaultDeviceTagsSource is used when the setting is unset or empty.
	DefaultDeviceTagsSource = DeviceTagsSourceResource
)

// IsValid returns true if the source is one of the supported values.
func (s DeviceTagsSource) IsValid() bool {
	switch s {
	case DeviceTagsSourceResource, DeviceTagsSourceAgent, DeviceTagsSourceBoth:
		return true
	}
	return false
}

// SendResourceTag returns true if the `dd.internal.resource:ndm_device` tag must be attached to metrics.
func (s DeviceTagsSource) SendResourceTag() bool {
	return s != DeviceTagsSourceAgent
}

// SendDeviceTags returns true if the Agent must stamp the device tags on every metric.
func (s DeviceTagsSource) SendDeviceTags() bool {
	return s != DeviceTagsSourceResource
}

// ParseDeviceTagsSource resolves a raw config value to a DeviceTagsSource. An empty value yields
// DefaultDeviceTagsSource. An unknown value yields DefaultDeviceTagsSource and an error, so callers
// can warn and keep running with the default.
func ParseDeviceTagsSource(raw string) (DeviceTagsSource, error) {
	if raw == "" {
		return DefaultDeviceTagsSource, nil
	}
	source := DeviceTagsSource(raw)
	if !source.IsValid() {
		return DefaultDeviceTagsSource, fmt.Errorf("invalid device_tags_source %q, must be one of %q, %q or %q, defaulting to %q",
			raw, DeviceTagsSourceResource, DeviceTagsSourceAgent, DeviceTagsSourceBoth, DefaultDeviceTagsSource)
	}
	return source, nil
}
