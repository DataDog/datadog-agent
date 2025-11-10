// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"fmt"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/hashicorp/go-multierror"
)

// apiCallInfo represents a single NVML API call that can produce one or more metrics.
// It supports both stateless collectors (ignore timestamp) and sampling collectors (use timestamp).
type apiCallInfo struct {
	Name    string                                                // Name of the API call for logging/debugging
	Handler func(ddnvml.Device, uint64) ([]Metric, uint64, error) // Function to handle the API call and return metrics (metrics, newTimestamp, error)
}

// baseCollector is a unified collector template that consolidates multiple collector types into one instance.
// It can handle both stateless and sampling-based API calls from different collector types (memory, device, process, etc.).
type baseCollector struct {
	name           CollectorName     // Name of the consolidated collector (e.g., "stateless" or "sampling")
	device         ddnvml.Device     // NVML device this collector monitors
	supportedAPIs  []apiCallInfo     // List of supported API calls from all consolidated collector types
	lastTimestamps map[string]uint64 // Per-API call timestamps (empty for stateless collectors)
}

// NewBaseCollector creates a new baseCollector for the given device and API calls.
// It automatically filters out unsupported APIs by testing them against the device.
func NewBaseCollector(name CollectorName, device ddnvml.Device, apiCalls []apiCallInfo) (Collector, error) {
	return newBaseCollector(name, device, apiCalls)
}

func newBaseCollector(name CollectorName, device ddnvml.Device, apiCalls []apiCallInfo) (*baseCollector, error) {
	c := &baseCollector{
		name:           name,
		device:         device,
		lastTimestamps: make(map[string]uint64),
	}

	// Filter supported APIs
	c.supportedAPIs = filterSupportedAPIs(device, apiCalls)
	if len(c.supportedAPIs) == 0 {
		return nil, errUnsupportedDevice
	}

	return c, nil
}

// DeviceUUID returns the UUID of the device this collector monitors.
func (c *baseCollector) DeviceUUID() string {
	return c.device.GetDeviceInfo().UUID
}

// Name returns the name of the collector.
func (c *baseCollector) Name() CollectorName {
	return c.name
}

// Collect executes all supported API calls and returns the collected metrics.
// For stateless collectors, timestamps are ignored (passed as 0, returned timestamp ignored).
// For sampling collectors, timestamps are maintained per API call.
func (c *baseCollector) Collect() ([]Metric, error) {
	var allMetrics []Metric
	var multiErr error

	for _, apiCall := range c.supportedAPIs {
		// Get last timestamp for this API call (0 for stateless collectors)
		lastTimestamp := c.lastTimestamps[apiCall.Name]

		// Execute the API call
		metrics, newTimestamp, err := apiCall.Handler(c.device, lastTimestamp)
		if err != nil {
			multiErr = multierror.Append(multiErr, fmt.Errorf("%s returned an error: %w", apiCall.Name, err))
		}

		// Update timestamp if this is a sampling collector (newTimestamp != 0)
		if newTimestamp != 0 {
			c.lastTimestamps[apiCall.Name] = newTimestamp
		}

		allMetrics = append(allMetrics, metrics...)
	}

	return allMetrics, multiErr
}
