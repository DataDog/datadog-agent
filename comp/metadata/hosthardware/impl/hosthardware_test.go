// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package hosthardwareimpl

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

// getTestHostHardware creates a test hosthardware instance with mocked dependencies
func getTestHostHardware(t *testing.T, overrides map[string]any) *hostHardware {

	if overrides == nil {
		overrides = map[string]any{
			"infrastructure_mode": "end_user_device",
		}
	}

	p := NewHardwareHostProvider(Requires{
		Log:        logmock.New(t),
		Config:     config.NewMockWithOverrides(t, overrides),
		Serializer: serializermock.NewMetricSerializer(t),
		Hostname:   hostnameimpl.NewHostnameService(),
		IPCClient:  ipcmock.New(t).GetClient(),
	})

	return p.Comp.(*hostHardware)
}

func TestNewHardwareHostProvider_EndUserDeviceMode(t *testing.T) {

	hh := getTestHostHardware(t, nil)
	// Should be enabled for end_user_device mode
	assert.True(t, hh.InventoryPayload.Enabled, "Should be enabled in end_user_device mode")
	assert.NotNil(t, hh.MetadataProvider, "Provider should not be nil when enabled")

	// Check intervals
	assert.Equal(t, 10*time.Minute, hh.InventoryPayload.MinInterval, "MinInterval should be 10 minutes")
	assert.Equal(t, 1*time.Hour, hh.InventoryPayload.MaxInterval, "MaxInterval should be 1 hour")

	// Check flare filename
	assert.Equal(t, "hosthardware.json", hh.FlareFileName, "Flare filename should match")
}

func TestNewHardwareHostProvider_FullMode(t *testing.T) {
	// Test with full mode (default) - should be disabled
	overrides := map[string]any{
		"infrastructure_mode": "full",
	}

	hh := getTestHostHardware(t, overrides)

	// Should be disabled for full mode
	assert.False(t, hh.InventoryPayload.Enabled, "Should be disabled in full mode")
}

func TestNewHardwareHostProvider_BasicMode(t *testing.T) {
	// Test with basic mode - should be disabled
	overrides := map[string]any{
		"infrastructure_mode": "basic",
	}

	hh := getTestHostHardware(t, overrides)

	// Should be disabled for basic mode
	assert.False(t, hh.InventoryPayload.Enabled, "Should be disabled in basic mode")
}

func TestGetPayload(t *testing.T) {
	hh := getTestHostHardware(t, nil)

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
	assert.NotNil(t, p.Metadata, "Host hardware metadata should not be nil")
}

func TestPayloadMarshalJSON(t *testing.T) {
	payload := &Payload{
		Hostname:  "test-host",
		Timestamp: time.Now().UnixNano(),
		UUID:      "test-uuid-12345",
		Metadata: &hostHardwareMetadata{
			Manufacturer: "Lenovo",
			ModelNumber:  "Thinkpad T14s",
			SerialNumber: "ABC123XYZ",
			Name:         "Thinkpad",
			ChassisType:  "Laptop",
			Identifier:   "SKU123",
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
	assert.NotNil(t, result["host_hardware_metadata"])

	// Verify metadata fields
	metadata := result["host_hardware_metadata"].(map[string]interface{})
	assert.Equal(t, "Lenovo", metadata["manufacturer"])
	assert.Equal(t, "Thinkpad T14s", metadata["model_number"])
	assert.Equal(t, "ABC123XYZ", metadata["serial_number"])
	assert.Equal(t, "Thinkpad", metadata["name"])
	assert.Equal(t, "Laptop", metadata["chassis_type"])
	assert.Equal(t, "SKU123", metadata["identifier"])
}

func TestFillData(t *testing.T) {
	hh := getTestHostHardware(t, nil)

	// Call fillData
	hh.fillData()

	// Verify data structure exists
	assert.NotNil(t, hh.data, "Data should not be nil after fillData")

	// Note: The actual values depend on the system where tests run
	// We can only verify the structure is populated
	t.Logf("Collected Hardware Info:")
	t.Logf("  Manufacturer: %s", hh.data.Manufacturer)
	t.Logf("  Model Number: %s", hh.data.ModelNumber)
	t.Logf("  Serial Number: %s", hh.data.SerialNumber)
	t.Logf("  Name: %s", hh.data.Name)
	t.Logf("  Chassis Type: %s", hh.data.ChassisType)
	t.Logf("  Identifier: %s", hh.data.Identifier)
}

func TestPayloadStructure(t *testing.T) {
	// Test that the payload structure matches expected JSON format
	metadata := &hostHardwareMetadata{
		Manufacturer: "Test Manufacturer",
		ModelNumber:  "Test Model",
		SerialNumber: "TEST123",
		Name:         "Test Name",
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
	expectedKeys := []string{"hostname", "timestamp", "host_hardware_metadata", "uuid"}
	for _, key := range expectedKeys {
		assert.Contains(t, result, key, "JSON should contain key: %s", key)
	}

	// Verify metadata keys
	metadataMap := result["host_hardware_metadata"].(map[string]interface{})
	expectedMetadataKeys := []string{
		"manufacturer",
		"model_number",
		"serial_number",
		"name",
		"chassis_type",
		"identifier",
	}
	for _, key := range expectedMetadataKeys {
		assert.Contains(t, metadataMap, key, "Metadata should contain key: %s", key)
	}
}
