// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package status

import (
	"fmt"
	"strings"
)

const (
	statusLevelOK      = "ok"
	statusLevelWarning = "warning"
	statusLevelError   = "error"
)

// DeviceDisplay is a device row enriched for status rendering.
type DeviceDisplay struct {
	Address           string
	Port              int
	Profile           string
	DeviceName        string
	StreamState       string
	StatusLevel       string
	StatusLabel       string
	Started           bool
	Transport         string
	ReconnectCount    int
	ReceivedSamples   int
	CachedPaths       int
	MetricsCollected  int
	CollectTopology   bool
	EverConnected     bool
	LastConnectedAt   int64
	LastError         string
	LastErrorAt       int64
	NextReconnectAt   int64
	NextReconnectDue  bool
	StatusUpdatedAt   int64
	SubscriptionPaths []string
	SearchText        string
}

func buildDeviceDisplay(device DeviceState) DeviceDisplay {
	statusLevel, statusLabel := deviceStatusLevel(device)
	deviceName := device.DeviceName
	if deviceName == "" {
		deviceName = device.Profile
	}
	if deviceName == "" {
		deviceName = device.Address
	}

	display := DeviceDisplay{
		Address:           device.Address,
		Port:              device.Port,
		Profile:           device.Profile,
		DeviceName:        deviceName,
		StreamState:       device.StreamState,
		StatusLevel:       statusLevel,
		StatusLabel:       statusLabel,
		Started:           device.Started,
		Transport:         device.Transport,
		ReconnectCount:    device.ReconnectCount,
		ReceivedSamples:   device.ReceivedSamples,
		CachedPaths:       device.CachedPaths,
		MetricsCollected:  device.MetricsCollected,
		CollectTopology:   device.CollectTopology,
		EverConnected:     device.EverConnected,
		LastConnectedAt:   device.LastConnectedAt,
		LastError:         device.LastError,
		LastErrorAt:       device.LastErrorAt,
		NextReconnectAt:   device.NextReconnectAt,
		NextReconnectDue:  device.NextReconnectDue,
		StatusUpdatedAt:   device.StatusUpdatedAt,
		SubscriptionPaths: append([]string(nil), device.SubscriptionPaths...),
	}
	display.SearchText = buildDeviceSearchText(display)
	return display
}

func buildDeviceSearchText(device DeviceDisplay) string {
	parts := []string{
		device.DeviceName,
		device.Profile,
		device.Address,
		fmt.Sprintf("%d", device.Port),
		fmt.Sprintf("%s:%d", device.Address, device.Port),
		device.StreamState,
		device.StatusLabel,
		device.LastError,
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func deviceStatusLevel(device DeviceState) (string, string) {
	switch device.StreamState {
	case "connected":
		return statusLevelOK, "Connected"
	case "reconnecting":
		if device.LastError != "" {
			return statusLevelError, "Reconnecting"
		}
		return statusLevelWarning, "Reconnecting"
	case "not_ready":
		if device.LastError != "" {
			return statusLevelError, "Not ready"
		}
		return statusLevelWarning, "Not ready"
	default:
		if device.LastError != "" {
			return statusLevelError, device.StreamState
		}
		return statusLevelWarning, device.StreamState
	}
}
