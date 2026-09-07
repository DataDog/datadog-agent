// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package status

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetProvider(t *testing.T) {
	t.Cleanup(resetRegistryForTesting)

	provider := GetProvider()
	require.NotNil(t, provider)
	assert.Equal(t, "GNMI", provider.Name())
	assert.Equal(t, "GNMI", provider.Section())
}

func TestStatusWithDevices(t *testing.T) {
	t.Cleanup(resetRegistryForTesting)

	RegisterDevice("10.0.0.1", 57400, "interface-stats", true, []string{
		"/openconfig/system/state/hostname",
		"/openconfig/interfaces/interface/state/admin-status{interface=name}",
	})
	UpdateDevice("10.0.0.1", 57400, func(device *DeviceState) {
		device.Started = true
		device.DeviceName = "router-1"
		device.StreamState = "connected"
		device.ReconnectCount = 2
		device.ReceivedSamples = 42
		device.MetricsCollected = 17
		device.CachedPaths = 17
		device.LastError = "start gNMI client failed: dial failed"
	})

	provider := Provider{}
	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			err := provider.JSON(false, stats)
			require.NoError(t, err)

			devices, ok := stats["devices"].([]DeviceDisplay)
			require.True(t, ok)
			require.Len(t, devices, 1)
			assert.Equal(t, "10.0.0.1", devices[0].Address)
			assert.Equal(t, 57400, devices[0].Port)
			assert.Equal(t, "router-1", devices[0].DeviceName)
			assert.Equal(t, "connected", devices[0].StreamState)
			assert.Equal(t, 42, devices[0].ReceivedSamples)
			assert.Equal(t, 17, devices[0].MetricsCollected)
			assert.Equal(t, statusLevelOK, devices[0].StatusLevel)
		}},
		{"Text summary", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.Text(false, b)
			require.NoError(t, err)

			output := strings.ReplaceAll(b.String(), "\r\n", "\n")
			assert.Contains(t, output, "router-1")
			assert.Contains(t, output, "10.0.0.1:57400")
			assert.Contains(t, output, "state=connected")
			assert.Contains(t, output, "metrics=17")
			assert.NotContains(t, output, "Subscription paths:")
		}},
		{"Text verbose", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.Text(true, b)
			require.NoError(t, err)

			output := strings.ReplaceAll(b.String(), "\r\n", "\n")
			assert.Contains(t, output, "/openconfig/system/state/hostname")
		}},
		{"HTML summary", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.HTML(false, b)
			require.NoError(t, err)

			output := strings.ReplaceAll(b.String(), "\r\n", "\n")
			assert.Contains(t, output, "<span class=\"stat_title\">gNMI Devices</span>")
			assert.Contains(t, output, "router-1")
			assert.Contains(t, output, "10.0.0.1:57400")
			assert.NotContains(t, output, "gnmi_devices_table")
		}},
		{"HTML verbose", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.HTML(true, b)
			require.NoError(t, err)

			output := strings.ReplaceAll(b.String(), "\r\n", "\n")
			assert.Contains(t, output, "gnmi_devices_table")
			assert.Contains(t, output, "gnmi-device-filter")
			assert.Contains(t, output, "router-1")
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}

func TestStatusWithoutDevices(t *testing.T) {
	t.Cleanup(resetRegistryForTesting)

	provider := Provider{}

	stats := make(map[string]interface{})
	require.NoError(t, provider.JSON(false, stats))

	devices, ok := stats["devices"].([]DeviceDisplay)
	require.True(t, ok)
	assert.Empty(t, devices)

	b := new(bytes.Buffer)
	require.NoError(t, provider.Text(false, b))
	assert.Contains(t, b.String(), "No gNMI devices configured.")
}

func TestStatusWithReconnectingDevice(t *testing.T) {
	t.Cleanup(resetRegistryForTesting)

	now := time.Now().UnixNano()
	RegisterDevice("10.0.0.3", 9339, "interface-stats", false, nil)
	UpdateDevice("10.0.0.3", 9339, func(device *DeviceState) {
		device.Started = true
		device.StreamState = "reconnecting"
		device.ReconnectCount = 3
		device.EverConnected = true
		device.LastConnectedAt = now - int64(2*time.Minute)
		device.LastError = "receive subscribe response: rpc error: code = Unavailable desc = connection reset"
		device.LastErrorAt = now - int64(30*time.Second)
		device.NextReconnectAt = now + int64(15*time.Second)
	})

	provider := Provider{}
	b := new(bytes.Buffer)
	require.NoError(t, provider.Text(false, b))

	output := strings.ReplaceAll(b.String(), "\r\n", "\n")
	assert.Contains(t, output, "state=reconnecting")
	assert.Contains(t, output, "receive subscribe response")

	b = new(bytes.Buffer)
	require.NoError(t, provider.Text(true, b))
	output = strings.ReplaceAll(b.String(), "\r\n", "\n")
	assert.Contains(t, output, "Connection details:")
	assert.Contains(t, output, "Next reconnect:")
}

func TestUnregisterDevice(t *testing.T) {
	t.Cleanup(resetRegistryForTesting)

	RegisterDevice("10.0.0.2", 57400, "interface-stats", false, nil)
	UnregisterDevice("10.0.0.2", 57400)

	assert.Empty(t, listDevices())
}

func TestDeviceStatusLevel(t *testing.T) {
	level, label := deviceStatusLevel(DeviceState{StreamState: "connected"})
	assert.Equal(t, statusLevelOK, level)
	assert.Equal(t, "Connected", label)

	level, label = deviceStatusLevel(DeviceState{StreamState: "reconnecting"})
	assert.Equal(t, statusLevelWarning, level)
	assert.Equal(t, "Reconnecting", label)

	level, _ = deviceStatusLevel(DeviceState{
		StreamState: "reconnecting",
		LastError:   "boom",
	})
	assert.Equal(t, statusLevelError, level)
}
