// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package status

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// DeviceState holds runtime status for a single gNMI device check instance.
type DeviceState struct {
	Address           string
	Port              int
	Profile           string
	CollectTopology   bool
	Started           bool
	StreamState       string
	Transport         string
	ReconnectCount    int
	ReceivedSamples   int
	CachedPaths       int
	SubscriptionPaths []string
	EverConnected     bool
	LastConnectedAt   int64
	LastError         string
	LastErrorAt       int64
	NextReconnectAt   int64
	NextReconnectDue  bool
	StatusUpdatedAt   int64
}

type registry struct {
	mu      sync.RWMutex
	devices map[string]*DeviceState
}

var globalRegistry = &registry{
	devices: make(map[string]*DeviceState),
}

func deviceKey(address string, port int) string {
	return fmt.Sprintf("%s:%d", address, port)
}

// RegisterDevice records a configured gNMI device in the status registry.
func RegisterDevice(address string, port int, profile string, collectTopology bool, subscriptionPaths []string) {
	key := deviceKey(address, port)
	paths := append([]string(nil), subscriptionPaths...)

	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	globalRegistry.devices[key] = &DeviceState{
		Address:           address,
		Port:              port,
		Profile:           profile,
		CollectTopology:   collectTopology,
		SubscriptionPaths: paths,
	}
}

// UnregisterDevice removes a device from the status registry.
func UnregisterDevice(address string, port int) {
	key := deviceKey(address, port)

	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	delete(globalRegistry.devices, key)
}

// UpdateDevice mutates the runtime fields for a registered device.
func UpdateDevice(address string, port int, update func(*DeviceState)) {
	key := deviceKey(address, port)

	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	device, ok := globalRegistry.devices[key]
	if !ok {
		return
	}
	update(device)
}

func listDevices() []DeviceState {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	devices := make([]DeviceState, 0, len(globalRegistry.devices))
	for _, device := range globalRegistry.devices {
		copied := *device
		copied.SubscriptionPaths = append([]string(nil), device.SubscriptionPaths...)
		devices = append(devices, copied)
	}

	sort.Slice(devices, func(i, j int) bool {
		if devices[i].Address == devices[j].Address {
			return devices[i].Port < devices[j].Port
		}
		return devices[i].Address < devices[j].Address
	})

	return devices
}

func listDevicesForDisplay() []DeviceState {
	devices := listDevices()
	now := time.Now()

	for i := range devices {
		if devices[i].NextReconnectAt > 0 {
			nextReconnect := time.Unix(0, devices[i].NextReconnectAt)
			devices[i].NextReconnectDue = !nextReconnect.After(now)
		}
	}

	return devices
}

// resetRegistryForTesting clears all registered devices.
func resetRegistryForTesting() {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	globalRegistry.devices = make(map[string]*DeviceState)
}
