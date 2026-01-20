// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package systeminfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetChassisTypeName_VirtualMachineDetection(t *testing.T) {
	// Test VM detection with various case combinations
	vmCases := []struct {
		name         string
		model        string
		manufacturer string
	}{
		{"lowercase model", "virtual machine", "Microsoft Corporation"},
		{"uppercase model", "VIRTUAL MACHINE", "Microsoft"},
		{"mixed case model", "VirTual MaChiNe", "Microsoft"},
		{"lowercase AWS", "t2.micro", "amazon ec2"},
		{"uppercase AWS", "m5.large", "AMAZON EC2"},
		{"mixed case AWS", "c5.xlarge", "Amazon Ec2"},
	}

	for _, tc := range vmCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getChassisTypeName(1, tc.model, tc.manufacturer)
			assert.Equal(t, "Virtual Machine", result, "Should detect as Virtual Machine")
		})
	}
}

func TestGetChassisTypeName_NonVirtualMachines(t *testing.T) {
	// Ensure physical machines are not incorrectly identified as VMs
	nonVMCases := []struct {
		name         string
		chassisType  int32
		model        string
		manufacturer string
		expected     string
	}{
		{
			name:         "Physical Desktop",
			chassisType:  3,
			model:        "OptiPlex 7090",
			manufacturer: "Dell Inc.",
			expected:     "Desktop",
		},
		{
			name:         "Physical Laptop",
			chassisType:  10,
			model:        "ThinkPad X1",
			manufacturer: "Lenovo",
			expected:     "Laptop",
		},
		{
			name:         "Model with 'machine' in name but not VM",
			chassisType:  3,
			model:        "Machine Learning Workstation",
			manufacturer: "Dell Inc.",
			expected:     "Desktop",
		},
	}

	for _, tc := range nonVMCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getChassisTypeName(tc.chassisType, tc.model, tc.manufacturer)
			assert.Equal(t, tc.expected, result, "Should not be detected as Virtual Machine")
		})
	}
}

func TestGetChassisTypeName_EdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		chassisType  int32
		model        string
		manufacturer string
		expected     string
	}{
		{
			name:         "Empty strings",
			chassisType:  3,
			model:        "",
			manufacturer: "",
			expected:     "Desktop",
		},
		{
			name:         "Whitespace in model",
			chassisType:  9,
			model:        "  Latitude 5420  ",
			manufacturer: "Dell Inc.",
			expected:     "Laptop",
		},
		{
			name:         "Special characters in model",
			chassisType:  3,
			model:        "OptiPlex 7090 (Custom)",
			manufacturer: "Dell Inc.",
			expected:     "Desktop",
		},
		{
			name:         "Very long model name",
			chassisType:  10,
			model:        "This is a very long model name that might appear in some enterprise systems with detailed configuration information",
			manufacturer: "Manufacturer",
			expected:     "Laptop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getChassisTypeName(tt.chassisType, tt.model, tt.manufacturer)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestWMICollection tests the actual WMI collection (integration test)
// This test will only pass on actual Windows systems with WMI available
func TestWMICollection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	result, err := Collect()

	// The collection should not error
	assert.NoError(t, err, "Collection should not error")
	assert.NotNil(t, result, "Result should not be nil")

	// We should get at least some basic information
	// Note: These assertions are lenient because not all systems provide all fields
	t.Logf("Collected System Info:")
	t.Logf("  Manufacturer: %s", result.Manufacturer)
	t.Logf("  Model Number: %s", result.ModelNumber)
	t.Logf("  Serial Number: %s", result.SerialNumber)
	t.Logf("  Model Name: %s", result.ModelName)
	t.Logf("  Chassis Type: %s", result.ChassisType)
	t.Logf("  Identifier: %s", result.Identifier)

	// At minimum, we should have manufacturer or model
	hasBasicInfo := result.Manufacturer != "" || result.ModelNumber != ""
	assert.True(t, hasBasicInfo, "Should have at least manufacturer or model information")

	// Chassis type should be one of the expected values
	if result.ChassisType != "" {
		validChassisTypes := []string{"Desktop", "Laptop", "Virtual Machine", "Other"}
		assert.Contains(t, validChassisTypes, result.ChassisType, "Chassis type should be valid")
	}
}
