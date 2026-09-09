// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package report

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

func formatInt(v int64) string {
	return strconv.FormatInt(v, 10)
}

func TestNetworkDeviceConfig_Creation(t *testing.T) {
	now := time.Now().Unix()

	deviceID := "default:10.0.0.1"
	deviceIP := "10.0.0.1"
	configType := types.RUNNING
	configSource := types.CLI
	configProfile := "cisco_ios"

	metadata := &profile.ExtractedMetadata{
		Timestamp: now,
	}
	tags := []string{"device_type:router", "vendor:cisco"}
	content := []byte("version 15.1\nhostname Router1")

	configUUID := "test_uuid"
	configHash := "test_hash"

	config := ToNetworkDeviceConfig(deviceID, deviceIP, configType, configProfile, metadata, tags, content, configUUID, configHash)

	assert.Equal(t, deviceID, config.DeviceID)
	assert.Equal(t, deviceIP, config.DeviceIP)
	assert.Equal(t, configType, config.ConfigType)
	assert.Equal(t, configSource, config.ConfigSource)
	assert.Equal(t, configProfile, config.ConfigProfile)
	assert.Equal(t, now, config.Timestamp)
	assert.Equal(t, tags, config.Tags)
	assert.Equal(t, string(content), config.Content)
	assert.Equal(t, configUUID, config.ID)
	assert.Equal(t, configHash, config.ConfigHash)
}

func TestNetworkDeviceConfig_OmitsEmptyStoreFields(t *testing.T) {
	metadata := &profile.ExtractedMetadata{Timestamp: time.Now().Unix()}
	config := ToNetworkDeviceConfig("default:10.0.0.1", "10.0.0.1", types.RUNNING, "", metadata, nil, []byte("content"), "", "")

	assert.Empty(t, config.ID)
	assert.Empty(t, config.ConfigHash)
	assert.Empty(t, config.ConfigProfile)

	jsonData, err := json.Marshal(config)
	require.NoError(t, err)
	assert.NotContains(t, string(jsonData), "\"id\"")
	assert.NotContains(t, string(jsonData), "config_hash")
	assert.NotContains(t, string(jsonData), "config_profile")
}

func TestNetworkDeviceConfig_ConfigTypes(t *testing.T) {
	tests := []struct {
		name         string
		configType   types.ConfigType
		configSource types.ConfigSource
		expected     types.ConfigType
	}{
		{
			name:         "running config",
			configType:   types.RUNNING,
			configSource: types.CLI,
			expected:     "running",
		},
		{
			name:         "startup config",
			configType:   types.STARTUP,
			configSource: types.CLI,
			expected:     "startup",
		},
	}

	for _, tt := range tests {
		metadata := &profile.ExtractedMetadata{
			Timestamp: 0,
		}
		t.Run(tt.name, func(t *testing.T) {
			config := ToNetworkDeviceConfig("default:10.0.0.1", "10.0.0.1", tt.configType, "", metadata, nil, []byte(""), "", "")
			assert.Equal(t, tt.expected, config.ConfigType)
		})
	}
}

func TestNetworkDevicesConfigPayload_Creation(t *testing.T) {
	namespace := "production"
	timestamp := time.Now().Unix()

	configs := []NetworkDeviceConfig{
		{
			DeviceID:     "default:10.0.0.1",
			DeviceIP:     "10.0.0.1",
			ConfigType:   types.RUNNING,
			ConfigSource: types.CLI,
			Timestamp:    timestamp,
			Tags:         []string{"device_type:router"},
			Content:      "running config content",
		},
		{
			DeviceID:     "default:10.0.0.1",
			DeviceIP:     "10.0.0.1",
			ConfigType:   types.STARTUP,
			ConfigSource: types.CLI,
			Timestamp:    timestamp,
			Tags:         []string{"device_type:router"},
			Content:      "startup config content",
		},
	}

	payload := ToNCMPayload(namespace, "test-agent-host", configs, []InventoryEntry{}, timestamp)

	assert.Equal(t, namespace, payload.Namespace)
	assert.Equal(t, "test-agent-host", payload.AgentHostname)
	assert.Equal(t, timestamp, payload.CollectTimestamp)
	assert.Len(t, payload.Configs, 2)
	assert.Equal(t, configs, payload.Configs)
}

func TestNCMPayload_JSONFormat(t *testing.T) {
	timestamp := time.Now().Unix()

	tests := []struct {
		name     string
		payload  NCMPayload
		expected string
	}{
		{
			name: "full payload with all fields",
			payload: NCMPayload{
				Namespace: "production",
				Configs: []NetworkDeviceConfig{
					{
						DeviceID:      "default:10.0.0.1",
						DeviceIP:      "10.0.0.1",
						ConfigType:    types.RUNNING,
						ConfigSource:  types.CLI,
						ConfigProfile: "cisco_ios",
						Timestamp:     timestamp,
						Tags:          []string{"device_type:router"},
						Content:       "running config content",
						ID:            "test_uuid",
						ConfigHash:    "test_hash",
					},
				},
				CollectTimestamp: timestamp,
			},
			expected: `{
				"namespace": "production",
				"configs": [
					{
						"device_id": "default:10.0.0.1",
						"device_ip": "10.0.0.1",
						"config_type": "running",
						"config_source": "cli",
						"config_profile": "cisco_ios",
						"timestamp": ` + formatInt(timestamp) + `,
						"tags": ["device_type:router"],
						"content": "running config content",
						"id": "test_uuid",
						"config_hash": "test_hash"
					}
				],
				"collect_timestamp": ` + formatInt(timestamp) + `,
				"agent_hostname": ""
			}`,
		},
		{
			name: "omitempty fields absent when empty",
			payload: NCMPayload{
				Namespace: "production",
				Configs: []NetworkDeviceConfig{
					{
						DeviceID:     "default:10.0.0.1",
						DeviceIP:     "10.0.0.1",
						ConfigType:   types.RUNNING,
						ConfigSource: types.CLI,
						Timestamp:    timestamp,
						Tags:         []string{"device_type:router"},
						Content:      "running config content",
					},
				},
				CollectTimestamp: timestamp,
			},
			expected: `{
				"namespace": "production",
				"configs": [
					{
						"device_id": "default:10.0.0.1",
						"device_ip": "10.0.0.1",
						"config_type": "running",
						"config_source": "cli",
						"timestamp": ` + formatInt(timestamp) + `,
						"tags": ["device_type:router"],
						"content": "running config content"
					}
				],
				"collect_timestamp": ` + formatInt(timestamp) + `,
				"agent_hostname": ""
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualJSON, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			var expectedCompact, actualParsed interface{}
			require.NoError(t, json.Unmarshal([]byte(tt.expected), &expectedCompact))
			require.NoError(t, json.Unmarshal(actualJSON, &actualParsed))
			assert.Equal(t, expectedCompact, actualParsed)
		})
	}
}

func TestNetworkDevicesConfigPayload_EmptyConfigs(t *testing.T) {
	payload := ToNCMPayload("test", "test-agent-host", []NetworkDeviceConfig{}, []InventoryEntry{}, time.Now().Unix())

	assert.Equal(t, "test", payload.Namespace)
	assert.Empty(t, payload.Configs)

	jsonData, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.NotContains(t, string(jsonData), "\"configs\"")
}

func TestNetworkDevicesConfigPayload_EmptyTimestamps(t *testing.T) {
	agentTs := time.Now().Unix()
	ndc := NetworkDeviceConfig{
		DeviceID:     "default:10.0.0.1",
		DeviceIP:     "10.0.0.1",
		ConfigType:   types.RUNNING,
		ConfigSource: types.CLI,
		Timestamp:    0,
	}
	payload := ToNCMPayload("test", "test-agent-host", []NetworkDeviceConfig{ndc}, []InventoryEntry{}, agentTs)

	expected := NetworkDeviceConfig{
		DeviceID:     "default:10.0.0.1",
		DeviceIP:     "10.0.0.1",
		ConfigType:   types.RUNNING,
		ConfigSource: types.CLI,
		Timestamp:    agentTs,
	}

	// check NCM payload
	assert.Equal(t, "test", payload.Namespace)
	assert.Len(t, payload.Configs, 1)
	assert.Equal(t, agentTs, payload.CollectTimestamp)

	// check the config's empty timestamp replaced with agent collection timestamp
	assert.Equal(t, payload.Configs[0], expected)
}
