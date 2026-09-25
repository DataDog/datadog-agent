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
	DeviceName        string
	CollectTopology   bool
	Started           bool
	StreamState       string
	Transport         string
	ReconnectCount    int
	ReceivedSamples   int
	CachedPaths       int
	MetricsCollected  int
	SubscriptionPaths []string
	EverConnected     bool
	LastConnectedAt   int64
	LastError         string
	LastErrorAt       int64
	NextReconnectAt   int64
	NextReconnectDue  bool
	StatusUpdatedAt   int64
}

// Registry holds the runtime status of configured gNMI devices.
// It is owned by the gnmistatus component and shared between the gNMI check
// instances, which record device state, and the agent status provider, which
// renders it.
type Registry struct {
	mu      sync.RWMutex
	devices map[string]*DeviceState
}

// NewRegistry returns an empty device status registry.
func NewRegistry() *Registry {
	return &Registry{devices: make(map[string]*DeviceState)}
}

func deviceKey(address string, port int) string {
	return fmt.Sprintf("%s:%d", address, port)
}

// RegisterDevice records a configured gNMI device in the status registry.
func (r *Registry) RegisterDevice(address string, port int, profile string, collectTopology bool, subscriptionPaths []string) {
	key := deviceKey(address, port)
	paths := append([]string(nil), subscriptionPaths...)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.devices[key] = &DeviceState{
		Address:           address,
		Port:              port,
		Profile:           profile,
		CollectTopology:   collectTopology,
		SubscriptionPaths: paths,
	}
}

// UnregisterDevice removes a device from the status registry.
func (r *Registry) UnregisterDevice(address string, port int) {
	key := deviceKey(address, port)

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.devices, key)
}

// UpdateDevice mutates the runtime fields for a registered device.
func (r *Registry) UpdateDevice(address string, port int, update func(*DeviceState)) {
	key := deviceKey(address, port)

	r.mu.Lock()
	defer r.mu.Unlock()

	device, ok := r.devices[key]
	if !ok {
		return
	}
	update(device)
}

func (r *Registry) listDevices() []DeviceState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	devices := make([]DeviceState, 0, len(r.devices))
	for _, device := range r.devices {
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

func (r *Registry) listDevicesForDisplay() []DeviceDisplay {
	devices := r.listDevices()
	now := time.Now()

	out := make([]DeviceDisplay, 0, len(devices))
	for i := range devices {
		if devices[i].NextReconnectAt > 0 {
			nextReconnect := time.Unix(0, devices[i].NextReconnectAt)
			devices[i].NextReconnectDue = !nextReconnect.After(now)
		}
		out = append(out, buildDeviceDisplay(devices[i]))
	}

	return out
}
