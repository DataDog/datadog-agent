// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package systeminfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollect(t *testing.T) {
	info, err := Collect()
	require.NoError(t, err, "Collect should not return an error")
	require.NotNil(t, info, "Collect should return system info")

	// On Darwin, manufacturer should always be Apple Inc.
	assert.Equal(t, "Apple Inc.", info.Manufacturer, "Manufacturer should be Apple Inc.")

	// Verify that required fields are populated
	// Note: We don't test exact values as they depend on the test machine
	t.Logf("Collected System Info:")
	t.Logf("  Manufacturer: %s", info.Manufacturer)
	t.Logf("  ModelNumber: %s", info.ModelNumber)
	t.Logf("  SerialNumber: %s", info.SerialNumber)
	t.Logf("  ModelName: %s", info.ModelName)
	t.Logf("  ChassisType: %s", info.ChassisType)
	t.Logf("  Identifier: %s", info.Identifier)

	// Chassis type should be one of the expected values
	validChassisTypes := []string{"Laptop", "Desktop", "Virtual Machine", "Other"}
	assert.Contains(t, validChassisTypes, info.ChassisType, "ChassisType should be a valid type")
}

func TestGetChassisType(t *testing.T) {
	tests := []struct {
		name            string
		productName     string
		modelIdentifier string
		expected        string
	}{
		{
			name:            "MacBook Pro",
			productName:     "MacBook Pro",
			modelIdentifier: "MacBookPro18,1",
			expected:        "Laptop",
		},
		{
			name:            "MacBook Air",
			productName:     "MacBook Air",
			modelIdentifier: "MacBookAir10,1",
			expected:        "Laptop",
		},
		{
			name:            "MacBook (generic)",
			productName:     "MacBook",
			modelIdentifier: "MacBook10,1",
			expected:        "Laptop",
		},
		{
			name:            "iMac",
			productName:     "iMac",
			modelIdentifier: "iMac21,1",
			expected:        "Desktop",
		},
		{
			name:            "Mac mini",
			productName:     "Mac mini",
			modelIdentifier: "Macmini9,1",
			expected:        "Desktop",
		},
		{
			name:            "Mac Pro",
			productName:     "Mac Pro",
			modelIdentifier: "MacPro7,1",
			expected:        "Desktop",
		},
		{
			name:            "Mac Studio",
			productName:     "Mac Studio",
			modelIdentifier: "Mac13,1",
			expected:        "Desktop",
		},
		{
			name:            "VMware VM",
			productName:     "VMware Virtual Platform",
			modelIdentifier: "VMware7,1",
			expected:        "Virtual Machine",
		},
		{
			name:            "VMware VM (mixed case)",
			productName:     "VMware Virtual Platform",
			modelIdentifier: "vmware7,1",
			expected:        "Virtual Machine",
		},
		{
			name:            "Apple Virtual Machine",
			productName:     "Apple Virtual Machine 1",
			modelIdentifier: "VirtualMac2,1",
			expected:        "Virtual Machine",
		},
		{
			name:            "Virtual in model identifier",
			productName:     "Some Mac",
			modelIdentifier: "VirtualMac2,1",
			expected:        "Virtual Machine",
		},
		{
			name:            "Virtual in product name",
			productName:     "Virtual Device",
			modelIdentifier: "Mac1,1",
			expected:        "Virtual Machine",
		},
		{
			name:            "Parallels VM",
			productName:     "Parallels Virtual Platform",
			modelIdentifier: "Parallels-ARM",
			expected:        "Virtual Machine",
		},
		{
			name:            "Parallels in identifier (mixed case)",
			productName:     "Some Device",
			modelIdentifier: "parallels-x86",
			expected:        "Virtual Machine",
		},
		{
			name:            "Unknown Apple device",
			productName:     "Apple Device",
			modelIdentifier: "Unknown1,1",
			expected:        "Other",
		},
		{
			name:            "Empty strings",
			productName:     "",
			modelIdentifier: "",
			expected:        "Other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getChassisType(tt.productName, tt.modelIdentifier)
			assert.Equal(t, tt.expected, result, "getChassisType(%q, %q) = %q, want %q",
				tt.productName, tt.modelIdentifier, result, tt.expected)
		})
	}
}

func TestGetChassisType_CaseInsensitive(t *testing.T) {
	// Test that the function handles different cases correctly
	testCases := []struct {
		name            string
		productName     string
		modelIdentifier string
		expected        string
	}{
		{"Uppercase MacBook", "MACBOOK PRO", "MacBookPro18,1", "Laptop"},
		{"Lowercase macbook", "macbook pro", "MacBookPro18,1", "Laptop"},
		{"Mixed case macBook", "macBook Pro", "MacBookPro18,1", "Laptop"},
		{"Uppercase iMac", "IMAC", "iMac21,1", "Desktop"},
		{"Lowercase imac", "imac", "iMac21,1", "Desktop"},
		{"Uppercase VM identifier", "Some Device", "VMWARE7,1", "Virtual Machine"},
		{"Lowercase vm identifier", "Some Device", "vmware7,1", "Virtual Machine"},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			result := getChassisType(tt.productName, tt.modelIdentifier)
			assert.Equal(t, tt.expected, result)
		})
	}
}
