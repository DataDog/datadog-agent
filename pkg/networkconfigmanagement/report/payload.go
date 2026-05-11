// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

// Package report contains types and functions for submitting/reporting network device configurations payloads
package report

import (
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// NCMPayload contains network devices configuration payload sent to EvP / backend
type NCMPayload struct {
	Namespace        string                `json:"namespace"`
	Configs          []NetworkDeviceConfig `json:"configs"`
	CollectTimestamp int64                 `json:"collect_timestamp"`
}

// NetworkDeviceConfig contains network device configuration for a single device
type NetworkDeviceConfig struct {
	DeviceID     string             `json:"device_id"`
	DeviceIP     string             `json:"device_ip"`
	ConfigType   types.ConfigType   `json:"config_type"`
	ConfigSource types.ConfigSource `json:"config_source"`
	Timestamp    int64              `json:"timestamp"`
	Tags         []string           `json:"tags"`
	Content      string             `json:"content"`
}

// ToNCMPayload converts the given parameters into a NCMPayload (sent to event platform / backend).
func ToNCMPayload(namespace string, configs []NetworkDeviceConfig, timestamp int64) NCMPayload {
	for i := range configs {
		// if timestamp could not be extracted from the configurations / commands, use the agent timestamp
		if configs[i].Timestamp == 0 {
			configs[i].Timestamp = timestamp
		}
	}
	return NCMPayload{
		Namespace:        namespace,
		Configs:          configs,
		CollectTimestamp: timestamp,
	}
}

// ToNetworkDeviceConfig converts the given parameters into a NetworkDeviceConfig, representing a single device's configuration in a point in time.
func ToNetworkDeviceConfig(deviceID, deviceIP string, configType types.ConfigType, extractedMetadata *profile.ExtractedMetadata, tags []string, content []byte) NetworkDeviceConfig {
	var ts int64
	if extractedMetadata != nil && extractedMetadata.Timestamp != 0 {
		ts = extractedMetadata.Timestamp
	} else {
		ts = 0
	}
	return NetworkDeviceConfig{
		DeviceID:     deviceID,
		DeviceIP:     deviceIP,
		ConfigType:   configType,
		ConfigSource: types.CLI,
		Timestamp:    ts,
		Tags:         tags,
		Content:      string(content),
	}
}
