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
	"slices"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
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
	field        CollectorName = "fields"
	gpm          CollectorName = "gpm"
	ebpf         CollectorName = "ebpf"
	deviceEvents CollectorName = "device_events"
)

// Metric represents a single metric collected from the NVML library.
type Metric struct {
	Name                string  // Name holds the name of the metric.
	Value               float64 // Value holds the value of the metric.
	Type                metrics.MetricType
	Priority            MetricPriority          // Priority is the priority of the metric, indicating which metric to keep in case of duplicates. Low (default) is the lowest priority.
	Tags                []string                // Tags holds optional metric-specific tags (e.g., "error type").
	AssociatedWorkloads []workloadmeta.EntityID // AssociatedWorkloads represents specific workloads that are associated with the metric, e.g. a process associated with a process-level metric. Used for tagging.
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
type subsystemBuilder func(device ddnvml.Device, deps *CollectorDependencies) (Collector, error)

// factory is a map of all the subsystems that can be used to collect metrics from NVML.
var factory = map[CollectorName]subsystemBuilder{
	// Consolidated collectors that combine multiple collector types into single instances
	stateless: newStatelessCollector, // Consolidates memory, device, clocks, remappedrows
	sampling:  newSamplingCollector,  // Consolidates process, samples

	// Specialized collectors that remain unchanged (complex or unique logic)
	field:        newFieldsCollector,
	gpm:          newGPMCollector,
	deviceEvents: newDeviceEventsCollector,
}

// CollectorDependencies holds the dependencies needed to create a set of collectors.
type CollectorDependencies struct {
	// DeviceEventsGatherer acts like a cache for the most recent device events
	DeviceEventsGatherer *DeviceEventsGatherer
	// SystemProbeCache is a (optional) cache of the latest metrics obtained from system probe
	SystemProbeCache *SystemProbeCache
	// Telemetry is the telemetry component to use for collecting metrics
	Telemetry *CollectorTelemetry
	// Workloadmeta is used for getting auxialiary metadata about containers and GPUs
	Workloadmeta workloadmeta.Component
}

// BuildCollectors returns a set of collectors that can be used to collect metrics from NVML.
// If SystemProbeCache is provided, additional system-probe virtual collectors will be created for all devices.
// disabledCollectors is a list of collector names that should not be created.
func BuildCollectors(devices []ddnvml.Device, deps *CollectorDependencies, disabledCollectors []string) ([]Collector, error) {
	return buildCollectors(devices, deps, factory, disabledCollectors)
}

func buildCollectors(devices []ddnvml.Device, deps *CollectorDependencies, builders map[CollectorName]subsystemBuilder, disabledCollectors []string) ([]Collector, error) {
	if len(devices) == 0 {
		return nil, nil
	}

	var collectors []Collector

	// Check that the disabled collectors are valid
	for _, disabled := range disabledCollectors {
		if _, ok := builders[CollectorName(disabled)]; !ok {
			log.Warnf("invalid disabled collector: %s", disabled)
			continue
		}
	}

	// Step 1: Build NVML collectors for physical devices only,
	// (since most of NVML API doesn't support MIG devices)
	for _, dev := range devices {
		for name, builder := range builders {
			// Skip disabled collectors
			if slices.Contains(disabledCollectors, string(name)) {
				log.Debugf("Skipping disabled collector %s for device %s", name, dev.GetDeviceInfo().UUID)
				deps.Telemetry.addCollectorCreation(name, "disabled")
				continue
			}

			c, err := builder(dev, deps)
			if errors.Is(err, errUnsupportedDevice) {
				log.Warnf("device %s does not support collector %s", dev.GetDeviceInfo().UUID, name)
				deps.Telemetry.addCollectorCreation(name, "unsupported")
				continue
			} else if err != nil {
				log.Warnf("failed to create collector %s for device %s: %s", name, dev.GetDeviceInfo().UUID, err)
				deps.Telemetry.addCollectorCreation(name, "error")
				continue
			}

			deps.Telemetry.addCollectorCreation(name, "success")
			collectors = append(collectors, c)
		}
	}

	// Step 2: Build system-probe virtual collectors for ALL devices (if cache provided)
	if deps.SystemProbeCache != nil {
		// Check if ebpf collector is disabled
		if slices.Contains(disabledCollectors, string(ebpf)) {
			log.Debug("Skipping disabled ebpf collector")
			deps.Telemetry.addCollectorCreation(ebpf, "disabled")
		} else {
			log.Info("GPU monitoring probe is enabled in system-probe, creating ebpf collectors for all devices")
			for _, dev := range devices {
				spCollector, err := newEbpfCollector(dev, deps.SystemProbeCache)
				if err != nil {
					log.Warnf("failed to create system-probe collector for device %s: %s", dev.GetDeviceInfo().UUID, err)
					deps.Telemetry.addCollectorCreation(ebpf, "error")
					continue
				}

				deps.Telemetry.addCollectorCreation(ebpf, "success")
				collectors = append(collectors, spCollector)
			}
		}
	}

	return collectors, nil
}

// CollectorTelemetry holds telemetry metrics for the collector data
type CollectorTelemetry struct {
	Created          telemetry.Counter
	CollectionErrors telemetry.Counter
	Time             telemetry.Histogram
}

// NewCollectorTelemetry creates a new CollectorTelemetry with the given telemetry component
func NewCollectorTelemetry(tm telemetry.Component) *CollectorTelemetry {
	subsystem := consts.GpuTelemetryModule + "__collectors"

	return &CollectorTelemetry{
		Created:          tm.NewCounter(subsystem, "created", []string{"collector", "status"}, "Number of collectors and their creation result"),
		CollectionErrors: tm.NewCounter(subsystem, "collection_errors", []string{"collector"}, "Number of errors from NVML collectors"),
		Time:             tm.NewHistogram(subsystem, "time_ms", []string{"collector"}, "Time taken to collect metrics from NVML collectors, in milliseconds", []float64{10, 100, 500, 1000, 5000}),
	}
}

// addCollector adds a collector to the telemetry, checking that the telemetry is not nil
func (t *CollectorTelemetry) addCollectorCreation(name CollectorName, status string) {
	if t == nil {
		return
	}
	t.Created.Add(1, string(name), status)
}
