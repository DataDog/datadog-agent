// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

// Package nvidia holds the logic to collect metrics from the NVIDIA Management Library (NVML).
// The main entry point is the BuildCollectors functions, which returns a set of collectors that will
// gather metrics from the available NVIDIA devices on the system. Each collector is responsible for
// a specific subsystem of metrics, such as device metrics, GPM metrics, etc. The collected metrics will
// be returned with the associated tags for each device.
package nvidia

import (
	"errors"
	"fmt"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// errUnsupportedDevice is returned when the device does not support the given collector
var errUnsupportedDevice = errors.New("device does not support the given collector")

// MetricPriority represents the priority level of a metric
type MetricPriority int

const (
	// Low priority is the default priority level (0)
	Low MetricPriority = 0
	// High priority level (10)
	High MetricPriority = 10
)

// CollectorName is the name of the nvml sub-collectors
type CollectorName string

const (
	// Consolidated collectors
	stateless CollectorName = "stateless" // Consolidates memory, device, clock, remappedRows
	sampling  CollectorName = "sampling"  // Consolidates process, samples

	// Specialized collectors (kept separate)
	field CollectorName = "fields"
	gpm   CollectorName = "gpm"
	ebpf  CollectorName = "ebpf"
)

// Metric represents a single metric collected from the NVML library.
type Metric struct {
	Name     string  // Name holds the name of the metric.
	Value    float64 // Value holds the value of the metric.
	Type     metrics.MetricType
	Priority MetricPriority // Priority is the priority of the metric, indicating which metric to keep in case of duplicates. Low (default) is the lowest priority.
	Tags     []string       // Tags holds optional metric-specific tags (e.g., process ID).
}

// Collector defines a collector that gets metric from a specific NVML subsystem and device
type Collector interface {
	// Collect collects metrics from the given NVML device. This method should not fill the tags
	// unless they're metric-specific (i.e., all device-specific tags will be added by the Collector itself)
	Collect() ([]Metric, error)

	// Name returns the name of the subsystem
	Name() CollectorName

	// DeviceUUID returns the UUID of the device this collector is collecting metrics from. Returns an empty string if there's no UUID
	DeviceUUID() string
}

// subsystemBuilder is a function that creates a new subsystem Collector. device the device it should collect metrics from. It also receives
// the tags associated with the device, the collector should use them when generating metrics.
type subsystemBuilder func(device ddnvml.Device) (Collector, error)

// factory is a map of all the subsystems that can be used to collect metrics from NVML.
var factory = map[CollectorName]subsystemBuilder{
	// Consolidated collectors that combine multiple collector types into single instances
	stateless: newStatelessCollector, // Consolidates memory, device, clocks, remappedrows
	sampling:  newSamplingCollector,  // Consolidates process, samples

	// Specialized collectors that remain unchanged (complex or unique logic)
	field: newFieldsCollector,
	gpm:   newGPMCollector,
}

// CollectorDependencies holds the dependencies needed to create a set of collectors.
type CollectorDependencies struct {
	// DeviceCache is a cache of GPU devices.
	DeviceCache ddnvml.DeviceCache
}

// BuildCollectors returns a set of collectors that can be used to collect metrics from NVML.
// If spCache is provided, additional system-probe virtual collectors will be created for all devices.
func BuildCollectors(deps *CollectorDependencies, spCache *SystemProbeCache) ([]Collector, error) {
	return buildCollectors(deps, factory, spCache)
}

func buildCollectors(deps *CollectorDependencies, builders map[CollectorName]subsystemBuilder, spCache *SystemProbeCache) ([]Collector, error) {
	var collectors []Collector

	// Step 1: Build NVML collectors for physical devices only,
	// (since most of NVML API doesn't support MIG devices)
	allPhysicalDevices, err := deps.DeviceCache.AllPhysicalDevices()
	if err != nil {
		return nil, fmt.Errorf("failed to get all physical devices: %w", err)
	}

	for _, dev := range allPhysicalDevices {
		for name, builder := range builders {
			c, err := builder(dev)
			if errors.Is(err, errUnsupportedDevice) {
				log.Warnf("device %s does not support collector %s", dev.GetDeviceInfo().UUID, name)
				continue
			} else if err != nil {
				log.Warnf("failed to create collector %s: %s", name, err)
				continue
			}

			collectors = append(collectors, c)
		}
	}

	// Step 2: Build system-probe virtual collectors for ALL devices (if cache provided)
	if spCache != nil {
		log.Info("GPU monitoring probe is enabled in system-probe, creating ebpf collectors for all devices")
		for _, dev := range allPhysicalDevices {
			spCollector, err := newEbpfCollector(dev, spCache)
			if err != nil {
				log.Warnf("failed to create system-probe collector for device %s: %s", dev.GetDeviceInfo().UUID, err)
				continue
			}
			collectors = append(collectors, spCollector)
		}
	}

	return collectors, nil
}
