// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package ntp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test the helper function that parses registry values
func TestGetNptServersFromRegKeyValue(t *testing.T) {
	testCases := []struct {
		name            string
		regKeyValue     string
		expectedServers []string
		expectError     bool
	}{
		{
			name:            "Single server with flags",
			regKeyValue:     "time.windows.com,0x9",
			expectedServers: []string{"time.windows.com"},
			expectError:     false,
		},
		{
			name:            "Multiple servers space-separated",
			regKeyValue:     "pool.ntp.org time.windows.com time.apple.com time.google.com",
			expectedServers: []string{"pool.ntp.org", "time.windows.com", "time.apple.com", "time.google.com"},
			expectError:     false,
		},
		{
			name:            "Multiple servers with flags",
			regKeyValue:     "ntp1.example.com,0x1 ntp2.example.com,0x9",
			expectedServers: []string{"ntp1.example.com", "ntp2.example.com"},
			expectError:     false,
		},
		{
			name:            "Empty string",
			regKeyValue:     "",
			expectedServers: nil,
			expectError:     true,
		},
		{
			name:            "Whitespace only",
			regKeyValue:     "   \t\n   ",
			expectedServers: nil,
			expectError:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			servers, err := getNptServersFromRegKeyValue(tc.regKeyValue)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expectedServers, servers)
		})
	}
}

// Integration test for getLocalDefinedNTPServers
// This test will only work on Windows systems with proper registry permissions
func TestGetLocalDefinedNTPServers_Integration(t *testing.T) {
	// Skip this test in CI or on non-Windows systems
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test attempts to read the actual registry
	// It may fail if:
	// - Not running on Windows
	// - No NTP servers configured
	// - Insufficient permissions
	servers, err := getLocalDefinedNTPServers()

	// We can't predict what servers will be configured, but we can check
	// that the function handles various registry states gracefully
	if err != nil {
		// Error is acceptable - might not have permissions or NTP not configured
		t.Logf("Registry read failed (this may be expected): %v", err)
		assert.Contains(t, err.Error(), "Cannot")
	} else {
		// If successful, we should have at least one server
		assert.NotEmpty(t, servers)
		t.Logf("Found NTP servers: %v", servers)

		// Each server should be a non-empty string
		for _, server := range servers {
			assert.NotEmpty(t, server)
		}
	}
}
