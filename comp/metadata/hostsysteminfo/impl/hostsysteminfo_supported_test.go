// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && (windows || darwin)

package hostsysteminfoimpl

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSystemInfoProvider_EndUserDeviceMode(t *testing.T) {

	hh := getTestHostSystemInfo(t, nil)
	// Should be enabled for end_user_device mode
	assert.True(t, hh.InventoryPayload.Enabled, "Should be enabled in end_user_device mode")
	assert.NotNil(t, hh.MetadataProvider, "Provider should not be nil when enabled")

	// Check intervals
	assert.Equal(t, 1*time.Hour, hh.InventoryPayload.MinInterval, "MinInterval should be 1 hour")
	assert.Equal(t, 1*time.Hour, hh.InventoryPayload.MaxInterval, "MaxInterval should be 1 hour")

	// Check flare filename
	assert.Equal(t, "hostsysteminfo.json", hh.FlareFileName, "Flare filename should match")
}

func TestNewSystemInfoProvider_FullMode(t *testing.T) {
	// Test with full mode (default) - should be disabled
	overrides := map[string]any{
		"infrastructure_mode": "full",
	}

	hh := getTestHostSystemInfo(t, overrides)

	// Should be disabled for full mode
	assert.False(t, hh.InventoryPayload.Enabled, "Should be disabled in full mode")
}

func TestNewSystemInfoProvider_BasicMode(t *testing.T) {
	// Test with basic mode - should be disabled
	overrides := map[string]any{
		"infrastructure_mode": "basic",
	}

	hh := getTestHostSystemInfo(t, overrides)

	// Should be disabled for basic mode
	assert.False(t, hh.InventoryPayload.Enabled, "Should be disabled in basic mode")
}

func TestGetPayload(t *testing.T) {
	hh := getTestHostSystemInfo(t, nil)

	// Mock the hostname
	hh.hostname = "test-hostname"

	// Get payload
	startTime := time.Now().UnixNano()
	payload := hh.getPayload()

	// Verify payload is not nil
	require.NotNil(t, payload, "Payload should not be nil")

	// Cast to proper type
	p, ok := payload.(*Payload)
	require.True(t, ok, "Payload should be of type *Payload")

	// Verify payload fields
	assert.Equal(t, "test-hostname", p.Hostname, "Hostname should match")
	assert.True(t, p.Timestamp >= startTime, "Timestamp should be after start time")
	assert.NotEmpty(t, p.UUID, "UUID should not be empty")
	assert.NotNil(t, p.Metadata, "Metadata should not be nil")

	// Verify metadata structure exists (values will depend on the system)
	assert.NotNil(t, p.Metadata, "Host system info metadata should not be nil")
}

func TestPayloadMarshalJSON(t *testing.T) {
	payload := &Payload{
		Hostname:  "test-host",
		Timestamp: time.Now().UnixNano(),
		UUID:      "test-uuid-12345",
		Metadata: &hostSystemInfoMetadata{
			Manufacturer: "Test Manufacturer",
			ModelNumber:  "Test Model",
			SerialNumber: "TEST123",
			ModelName:    "Test Name",
			ChassisType:  "Desktop",
			Identifier:   "ID123",
		},
	}

	// Marshal to JSON
	jsonData, err := payload.MarshalJSON()
	require.NoError(t, err, "MarshalJSON should not error")

	// Unmarshal to verify structure
	var result map[string]interface{}
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err, "Should unmarshal successfully")

	// Verify top-level fields
	assert.Equal(t, "test-host", result["hostname"])
	assert.Equal(t, "test-uuid-12345", result["uuid"])
	assert.NotNil(t, result["timestamp"])
	assert.NotNil(t, result["host_system_info_metadata"])

	// Verify metadata fields
	metadata := result["host_system_info_metadata"].(map[string]interface{})
	assert.Equal(t, "Test Manufacturer", metadata["manufacturer"])
	assert.Equal(t, "Test Model", metadata["model_number"])
	assert.Equal(t, "TEST123", metadata["serial_number"])
	assert.Equal(t, "Test Name", metadata["model_name"])
	assert.Equal(t, "Desktop", metadata["chassis_type"])
	assert.Equal(t, "ID123", metadata["identifier"])
}

func TestFillData(t *testing.T) {
	hh := getTestHostSystemInfo(t, nil)

	// Call fillData
	hh.fillData()

	// Verify data structure exists
	assert.NotNil(t, hh.data, "Data should not be nil after fillData")

	// Note: The actual values depend on the system where tests run
	// We can only verify the structure is populated
	t.Logf("Collected System Info:")
	t.Logf("  Manufacturer: %s", hh.data.Manufacturer)
	t.Logf("  Model Number: %s", hh.data.ModelNumber)
	t.Logf("  Serial Number: %s", hh.data.SerialNumber)
	t.Logf("  Model Name: %s", hh.data.ModelName)
	t.Logf("  Chassis Type: %s", hh.data.ChassisType)
	t.Logf("  Identifier: %s", hh.data.Identifier)
}

func TestPayloadStructure(t *testing.T) {
	// Test that the payload structure matches expected JSON format
	metadata := &hostSystemInfoMetadata{
		Manufacturer: "Test Manufacturer",
		ModelNumber:  "Test Model",
		SerialNumber: "TEST123",
		ModelName:    "Test Name",
		ChassisType:  "Desktop",
		Identifier:   "ID123",
	}

	payload := &Payload{
		Hostname:  "test-host",
		Timestamp: 1234567890,
		Metadata:  metadata,
		UUID:      "uuid-123",
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	// Unmarshal to verify all fields are present
	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err)

	// Verify all expected keys exist
	expectedKeys := []string{"hostname", "timestamp", "host_system_info_metadata", "uuid"}
	for _, key := range expectedKeys {
		assert.Contains(t, result, key, "JSON should contain key: %s", key)
	}

	// Verify metadata keys
	metadataMap := result["host_system_info_metadata"].(map[string]interface{})
	expectedMetadataKeys := []string{
		"manufacturer",
		"model_number",
		"serial_number",
		"model_name",
		"chassis_type",
		"identifier",
	}
	for _, key := range expectedMetadataKeys {
		assert.Contains(t, metadataMap, key, "Metadata should contain key: %s", key)
	}
}
