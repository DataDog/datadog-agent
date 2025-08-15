// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package report

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"
)

func TestNetworkDeviceConfig_Creation(t *testing.T) {
	now := time.Now().Unix()

	deviceID := "default:10.0.0.1"
	deviceIP := "10.0.0.1"
	configType := RUNNING
	timestamp := now
	tags := []string{"device_type:router", "vendor:cisco"}
	content := "version 15.1\nhostname Router1"

	config := ToNetworkDeviceConfig(deviceID, deviceIP, configType, now, tags, content)

	assert.Equal(t, deviceID, config.DeviceID)
	assert.Equal(t, deviceIP, config.DeviceIP)
	assert.Equal(t, string(configType), config.ConfigType)
	assert.Equal(t, timestamp, config.Timestamp)
	assert.Equal(t, tags, config.Tags)
	assert.Equal(t, content, config.Content)
}

func TestNetworkDeviceConfig_ConfigTypes(t *testing.T) {
	tests := []struct {
		name       string
		configType ConfigType
		expected   string
	}{
		{
			name:       "running config",
			configType: RUNNING,
			expected:   "running",
		},
		{
			name:       "startup config",
			configType: STARTUP,
			expected:   "startup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := ToNetworkDeviceConfig("default:10.0.0.1", "10.0.0.1", tt.configType, 0, nil, "")
			assert.Equal(t, tt.expected, config.ConfigType)
		})
	}
}

func TestNetworkDevicesConfigPayload_Creation(t *testing.T) {
	namespace := "production"
	integration := integrations.Integration("ncm")
	timestamp := time.Now().Unix()

	configs := []NetworkDeviceConfig{
		{
			DeviceID:   "default:10.0.0.1",
			DeviceIP:   "10.0.0.1",
			ConfigType: string(RUNNING),
			Timestamp:  timestamp,
			Tags:       []string{"device_type:router"},
			Content:    "running config content",
		},
		{
			DeviceID:   "default:10.0.0.1",
			DeviceIP:   "10.0.0.1",
			ConfigType: string(STARTUP),
			Timestamp:  timestamp,
			Tags:       []string{"device_type:router"},
			Content:    "startup config content",
		},
	}

	payload := ToNCMPayload(namespace, integration, configs, timestamp)

	assert.Equal(t, namespace, payload.Namespace)
	assert.Equal(t, integration, payload.Integration)
	assert.Equal(t, timestamp, payload.CollectTimestamp)
	assert.Len(t, payload.Configs, 2)
	assert.Equal(t, configs, payload.Configs)
}

func TestNetworkDevicesConfigPayload_EmptyConfigs(t *testing.T) {
	payload := ToNCMPayload("test", "", []NetworkDeviceConfig{}, time.Now().Unix())

	assert.Equal(t, "test", payload.Namespace)
	assert.Empty(t, payload.Configs)

	// Should still be valid JSON
	jsonData, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "\"configs\":[]")
}
