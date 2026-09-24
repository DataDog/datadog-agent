// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDriverEventDeviceKey(t *testing.T) {
	for _, tc := range []struct {
		name     string
		event    DriverEvent
		expected string
	}{
		{
			name:     "prefers the UUID",
			event:    DriverEvent{DeviceUUID: "GPU-1", PCIBusID: "0000:35:00.0"},
			expected: "GPU-1",
		},
		{
			// A GPU that has left the PCIe bus cannot be resolved through NVML, so the PCI
			// bus ID is the only identifier the event carries.
			name:     "falls back to the PCI bus ID",
			event:    DriverEvent{PCIBusID: "0000:97:00.0"},
			expected: "0000:97:00.0",
		},
		{
			name:     "is empty when neither identifier is present",
			event:    DriverEvent{},
			expected: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.event.DeviceKey())
		})
	}
}

func TestDriverEventDeviceKeyDistinguishesUnresolvedDevices(t *testing.T) {
	// Grouping on DeviceUUID would collapse these two into a single empty-UUID bucket.
	first := DriverEvent{PCIBusID: "0000:97:00.0"}
	second := DriverEvent{PCIBusID: "0000:98:00.0"}

	require.NotEqual(t, first.DeviceKey(), second.DeviceKey())
}
