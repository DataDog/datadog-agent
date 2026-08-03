// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package report contains types and functions for submitting/reporting network device configurations payloads
package report

import (
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// NCMPayload contains network devices configuration payload sent to EvP / backend
type NCMPayload struct {
	Namespace        string                `json:"namespace"`
	Configs          []NetworkDeviceConfig `json:"configs,omitempty"`
	Inventories      []InventoryEntry      `json:"inventories,omitempty"`
	CollectTimestamp int64                 `json:"collect_timestamp"`
	AgentHostname    string                `json:"agent_hostname"`
}

// NetworkDeviceConfig contains network device configuration for a single device.
// ID and ConfigHash are populated when the agent was able to persist the
// config in the local store; both are omitted from the payload otherwise.
type NetworkDeviceConfig struct {
	DeviceID      string             `json:"device_id"`
	DeviceIP      string             `json:"device_ip"`
	ConfigType    types.ConfigType   `json:"config_type"`
	ConfigSource  types.ConfigSource `json:"config_source"`
	ConfigProfile string             `json:"config_profile,omitempty"`
	Timestamp     int64              `json:"timestamp"`
	Tags          []string           `json:"tags"`
	Content       string             `json:"content"`
	ID            string             `json:"id,omitempty"`
	ConfigHash    string             `json:"config_hash,omitempty"`
}

// InventoryEntry contains the metadata about the configs stored locally on the agent
type InventoryEntry struct {
	Namespace  string `json:"namespace"`
	ConfigID   string `json:"config_id"`
	DeviceID   string `json:"device_id"`
	ReportedAt int64  `json:"reported_at"`
}

// ToNCMPayload converts the given parameters into a NCMPayload (sent to event platform / backend).
func ToNCMPayload(namespace string, agentHostname string, configs []NetworkDeviceConfig, inventories []InventoryEntry, timestamp int64) NCMPayload {
	for i := range configs {
		// if timestamp could not be extracted from the configurations / commands, use the agent timestamp
		if configs[i].Timestamp == 0 {
			configs[i].Timestamp = timestamp
		}
	}
	return NCMPayload{
		Namespace:        namespace,
		AgentHostname:    agentHostname,
		Configs:          configs,
		Inventories:      inventories,
		CollectTimestamp: timestamp,
	}
}

// ToNetworkDeviceConfig converts the given parameters into a NetworkDeviceConfig, representing a single device's configuration in a point in time.
// id and configHash are optional — pass empty strings when the config could not be persisted in the local store.
func ToNetworkDeviceConfig(deviceID, deviceIP string, configType types.ConfigType, configProfile string, extractedMetadata *profile.ExtractedMetadata, tags []string, content []byte, uuid string, configHash string) NetworkDeviceConfig {
	var ts int64
	if extractedMetadata != nil && extractedMetadata.Timestamp != 0 {
		ts = extractedMetadata.Timestamp
	} else {
		ts = 0
	}
	return NetworkDeviceConfig{
		DeviceID:      deviceID,
		DeviceIP:      deviceIP,
		ConfigType:    configType,
		ConfigSource:  types.CLI,
		ConfigProfile: configProfile,
		Timestamp:     ts,
		Tags:          tags,
		Content:       string(content),
		ID:            uuid,
		ConfigHash:    configHash,
	}
}
