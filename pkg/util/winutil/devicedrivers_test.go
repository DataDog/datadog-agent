// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnumDeviceDrivers verifies against the real host that the SetupAPI device pass finds a
// non-empty, duplicate-free set of present devices, and that the service short-circuit leaves
// every other field empty for a device already driven by a kernel service.
func TestEnumDeviceDrivers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	devices, err := EnumDeviceDrivers()
	require.NoError(t, err)
	require.NotEmpty(t, devices, "a Windows host always has present devices")

	seen := make(map[string]struct{}, len(devices))
	for _, device := range devices {
		assert.NotEmpty(t, device.InstanceID, "every device record has an instance id")

		_, duplicate := seen[device.InstanceID]
		assert.False(t, duplicate, "device %q enumerated more than once", device.InstanceID)
		seen[device.InstanceID] = struct{}{}

		if device.Service != "" {
			assert.Empty(t, device.HardwareID,
				"device %q has a service, so its hardware ID should be left unread", device.InstanceID)
			assert.Empty(t, device.Description,
				"device %q has a service, so its description should be left unread", device.InstanceID)
			assert.Empty(t, device.InfName,
				"device %q has a service, so its INF name should be left unread", device.InstanceID)
			assert.Empty(t, device.DriverVersion,
				"device %q has a service, so its driver version should be left unread", device.InstanceID)
		}
	}
}
