// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

package report

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkDeviceConfig_Creation(t *testing.T) {
	now := time.Now().Unix()

	deviceID := "default:10.0.0.1"
	deviceIP := "10.0.0.1"
	configType := RUNNING
	configSource := CLI

	metadata := &profile.ExtractedMetadata{
		Timestamp: now,
	}
	tags := []string{"device_type:router", "vendor:cisco"}
	content := []byte("version 15.1\nhostname Router1")

	config := ToNetworkDeviceConfig(deviceID, deviceIP, configType, metadata, tags, content)

	assert.Equal(t, deviceID, config.DeviceID)
	assert.Equal(t, deviceIP, config.DeviceIP)
	assert.Equal(t, string(configType), config.ConfigType)
	assert.Equal(t, string(configSource), config.ConfigSource)
	assert.Equal(t, now, config.Timestamp)
	assert.Equal(t, tags, config.Tags)
	assert.Equal(t, string(content), config.Content)
}

func TestNetworkDeviceConfig_ConfigTypes(t *testing.T) {
	tests := []struct {
		name         string
		configType   ConfigType
		configSource ConfigSource
		expected     string
	}{
		{
			name:         "running config",
			configType:   RUNNING,
			configSource: CLI,
			expected:     "running",
		},
		{
			name:         "startup config",
			configType:   STARTUP,
			configSource: CLI,
			expected:     "startup",
		},
	}

	for _, tt := range tests {
		metadata := &profile.ExtractedMetadata{
			Timestamp: 0,
		}
		t.Run(tt.name, func(t *testing.T) {
			config := ToNetworkDeviceConfig("default:10.0.0.1", "10.0.0.1", tt.configType, metadata, nil, []byte(""))
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
			ConfigType:   string(RUNNING),
			ConfigSource: string(CLI),
			Timestamp:    timestamp,
			Tags:         []string{"device_type:router"},
			Content:      "running config content",
		},
		{
			DeviceID:     "default:10.0.0.1",
			DeviceIP:     "10.0.0.1",
			ConfigType:   string(STARTUP),
			ConfigSource: string(CLI),
			Timestamp:    timestamp,
			Tags:         []string{"device_type:router"},
			Content:      "startup config content",
		},
	}

	payload := ToNCMPayload(namespace, configs, timestamp)

	assert.Equal(t, namespace, payload.Namespace)
	assert.Equal(t, timestamp, payload.CollectTimestamp)
	assert.Len(t, payload.Configs, 2)
	assert.Equal(t, configs, payload.Configs)
}

func TestNetworkDevicesConfigPayload_EmptyConfigs(t *testing.T) {
	payload := ToNCMPayload("test", []NetworkDeviceConfig{}, time.Now().Unix())

	assert.Equal(t, "test", payload.Namespace)
	assert.Empty(t, payload.Configs)

	// Should still be valid JSON
	jsonData, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"configs\":[]")
}

func TestNetworkDevicesConfigPayload_EmptyTimestamps(t *testing.T) {
	agentTs := time.Now().Unix()
	ndc := NetworkDeviceConfig{
		DeviceID:     "default:10.0.0.1",
		DeviceIP:     "10.0.0.1",
		ConfigType:   string(RUNNING),
		ConfigSource: string(CLI),
		Timestamp:    0,
	}
	payload := ToNCMPayload("test", []NetworkDeviceConfig{ndc}, agentTs)

	expected := NetworkDeviceConfig{
		DeviceID:     "default:10.0.0.1",
		DeviceIP:     "10.0.0.1",
		ConfigType:   string(RUNNING),
		ConfigSource: string(CLI),
		Timestamp:    agentTs,
	}

	// check NCM payload
	assert.Equal(t, "test", payload.Namespace)
	assert.Len(t, payload.Configs, 1)
	assert.Equal(t, agentTs, payload.CollectTimestamp)

	// check the config's empty timestamp replaced with agent collection timestamp
	assert.Equal(t, payload.Configs[0], expected)
}
