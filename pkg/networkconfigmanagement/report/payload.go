// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

// Package report contains types and functions for submitting/reporting network device configurations payloads
package report

import (
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"
)

// ConfigType defines the type of network device configuration
type ConfigType string

const (
	// RUNNING represents the running configuration of a network device (the current active configuration)
	RUNNING ConfigType = "running"
	// STARTUP represents the startup configuration of a network device (the configuration that is loaded on boot)
	STARTUP ConfigType = "startup"
)

// NCMPayload contains network devices configuration payload sent to EvP / backend
type NCMPayload struct {
	Namespace        string                   `json:"namespace"`
	Integration      integrations.Integration `json:"integration"`
	Configs          []NetworkDeviceConfig    `json:"configs"`
	CollectTimestamp int64                    `json:"collect_timestamp"`
}

// NetworkDeviceConfig contains network device configuration for a single device
type NetworkDeviceConfig struct {
	DeviceID   string   `json:"device_id"`
	DeviceIP   string   `json:"device_ip"`
	ConfigType string   `json:"config_type"`
	Timestamp  int64    `json:"timestamp"`
	Tags       []string `json:"tags"`
	Content    []byte   `json:"content"`
}

// ToNCMPayload converts the given parameters into a NCMPayload (sent to event platform / backend).
func ToNCMPayload(namespace string, integration integrations.Integration, configs []NetworkDeviceConfig, timestamp int64) NCMPayload {
	return NCMPayload{
		Namespace:        namespace,
		Integration:      integration,
		Configs:          configs,
		CollectTimestamp: timestamp,
	}
}

// ToNetworkDeviceConfig converts the given parameters into a NetworkDeviceConfig, representing a single device's configuration in a point in time.
func ToNetworkDeviceConfig(deviceID, deviceIP string, configType ConfigType, extractedMetadata *profile.ExtractedMetadata, tags []string, content []byte) NetworkDeviceConfig {
	var ts int64
	if extractedMetadata != nil && extractedMetadata.Timestamp != 0 {
		ts = extractedMetadata.Timestamp
	} else {
		ts = 0
	}
	return NetworkDeviceConfig{
		DeviceID:   deviceID,
		DeviceIP:   deviceIP,
		ConfigType: string(configType),
		Timestamp:  ts,
		Tags:       tags,
		Content:    content,
	}
}
