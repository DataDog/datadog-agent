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
	"strconv"
	"sync"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// errUnsupportedDevice is returned when the device does not support the given collector
var errUnsupportedDevice = errors.New("device does not support the given collector")

// Internal collector names used by the factory
const (
	// Consolidated collectors
	stateless CollectorName = "stateless" // Consolidates memory, device, clock, remappedRows
	sampling  CollectorName = "sampling"  // Consolidates process, samples

	// Specialized collectors (kept separate)
	field        CollectorName = "fields"
	gpm          CollectorName = "gpm"
	ebpf         CollectorName = "ebpf"
	deviceEvents CollectorName = "device_events"
	nvlinkPLR    CollectorName = "nvlink_plr"
	nvlinkFEC    CollectorName = "nvlink_fec"
	nvlinkFields CollectorName = "nvlink_fields"
	nvlinkGPM    CollectorName = "nvlink_gpm"
)

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
	nvlinkPLR:    newNVLinkPLRCollector,
	nvlinkFEC:    newNVLinkFECCollector,
	nvlinkFields: newNVLinkFieldsCollector,
	nvlinkGPM:    newNVLinkGPMCollector,
	gpm:          newGPMCollector,
	deviceEvents: newDeviceEventsCollector,
	ebpf:         newEbpfCollector,
}

// CollectorDependencies holds the dependencies needed to create a set of collectors.
type CollectorDependencies struct {
	// DeviceEventsGatherer acts like a cache for the most recent device events
	DeviceEventsGatherer *DeviceEventsGatherer
	// SystemProbeCache is a (optional) cache of the latest metrics obtained from system probe
	SystemProbeCache *SystemProbeCache
	// PRMCache is a cache of privileged PRM metrics obtained from system-probe
	PRMCache *PRMCache
	// Telemetry is the telemetry component to use for collecting metrics
	Telemetry *CollectorTelemetry
	// Workloadmeta is used for getting auxialiary metadata about containers and GPUs
	Workloadmeta workloadmeta.Component
	// Config contains the parsed GPU configuration shared with system-probe.
	Config gpuconfig.Config
}

// BuildCollectors returns a set of collectors that can be used to collect metrics from NVML.
func BuildCollectors(devices []ddnvml.Device, deps *CollectorDependencies) ([]Collector, error) {
	return buildCollectors(devices, deps, factory)
}

func buildCollectors(devices []ddnvml.Device, deps *CollectorDependencies, builders map[CollectorName]subsystemBuilder) ([]Collector, error) {
	if len(devices) == 0 {
		return nil, nil
	}

	var collectors []Collector

	// Check that the disabled collectors are valid
	for _, disabled := range deps.Config.DisabledCollectors {
		if _, ok := builders[CollectorName(disabled)]; !ok {
			log.Warnf("invalid disabled collector: %s", disabled)
			continue
		}
	}

	// Step 1: Build NVML collectors for physical devices only,
	// (since most of NVML API doesn't support MIG devices)
	for _, dev := range devices {
		for name, builder := range builders {
			if collectorDisabled(name, deps.Config) {
				log.Debugf("Skipping disabled collector %s for device %s", name, dev.GetDeviceInfo().UUID)
				deps.Telemetry.addCollectorCreation(name, "disabled", dev)
				continue
			}

			c, err := builder(dev, deps)
			if errors.Is(err, errUnsupportedDevice) {
				log.Warnf("device %s does not support collector %s", dev.GetDeviceInfo().UUID, name)
				deps.Telemetry.addCollectorCreation(name, "unsupported", dev)
				continue
			} else if err != nil {
				log.Warnf("failed to create collector %s for device %s: %s", name, dev.GetDeviceInfo().UUID, err)
				deps.Telemetry.addCollectorCreation(name, "error", dev)
				continue
			}

			deps.Telemetry.addCollectorCreation(name, "success", dev)
			collectors = append(collectors, c)
		}
	}

	return collectors, nil
}

func collectorDisabled(name CollectorName, config gpuconfig.Config) bool {
	if slices.Contains(config.DisabledCollectors, string(name)) {
		return true
	}

	switch name {
	case ebpf:
		return !config.Enabled || !config.EnableEBPFProbes
	case nvlinkPLR:
		return !config.Enabled || !config.PRMEndpointEnabled
	default:
		return false
	}
}

// CollectorTelemetry holds telemetry metrics for NVIDIA collector creation and execution.
// It belongs in this package because BuildCollectors records creation outcomes, including
// failures that occur before a collector instance exists.
type CollectorTelemetry struct {
	CollectionRuns   telemetry.Counter
	Created          telemetry.Counter
	CollectionErrors telemetry.Counter
	Time             telemetry.Histogram
}

// NewCollectorTelemetry creates a new CollectorTelemetry with the given telemetry component
func NewCollectorTelemetry(tm telemetry.Component) *CollectorTelemetry {
	subsystem := consts.GpuTelemetryModule + "__collectors"

	return &CollectorTelemetry{
		CollectionRuns:   tm.NewCounter(subsystem, "collection_runs", collectorTelemetryTagNames, "Number of collector runs"),
		Created:          tm.NewCounter(subsystem, "created", collectorCreationTelemetryTagNames, "Number of collectors and their creation result"),
		CollectionErrors: tm.NewCounter(subsystem, "collection_errors", collectorTelemetryTagNames, "Number of errors from NVML collectors"),
		Time:             tm.NewHistogram(subsystem, "time_ms", collectorTelemetryTagNames, "Time taken to collect metrics from NVML collectors, in milliseconds", []float64{10, 100, 500, 1000, 5000}),
	}
}

var collectorTelemetryTagNames = []string{
	"collector",
	"gpu_device",
	"gpu_virtualization_mode",
	"gpu_architecture",
	"gpu_slicing_mode",
	"gpu_nvlink_capable",
	"gpu_nvlink_version",
	"gpu_driver_version",
}

var (
	cachedDriverVersion     string
	cachedDriverVersionOnce sync.Once
)

var collectorCreationTelemetryTagNames = append([]string{"status"}, collectorTelemetryTagNames...)

// CollectorTelemetryTags returns the telemetry tag values for a collector and its device.
func CollectorTelemetryTags(collector Collector) []string {
	return collectorTelemetryTags(collector.Name(), collector.Device())
}

func collectorTelemetryTags(name CollectorName, device ddnvml.Device) []string {
	deviceInfo := device.GetDeviceInfo()
	return []string{
		string(name),
		gpuutil.NormalizeGPUDeviceName(deviceInfo.Name),
		gpuutil.VirtualizationModeToString(deviceInfo.VirtualizationMode),
		gpuutil.ArchToString(deviceInfo.Architecture),
		slicingModeTag(device),
		strconv.FormatBool(deviceInfo.NVLinkLinkCount > 0),
		deviceInfo.NVLinkVersion,
		driverVersionForTelemetry(),
	}
}

func driverVersionForTelemetry() string {
	cachedDriverVersionOnce.Do(func() {
		lib, err := ddnvml.GetSafeNvmlLib()
		if err != nil {
			return
		}

		driverVersion, err := lib.SystemGetDriverVersion()
		if err != nil {
			log.Debugf("failed to get driver version for collector telemetry: %v", err)
			return
		}

		cachedDriverVersion = driverVersion
	})

	return cachedDriverVersion
}

func slicingModeTag(device ddnvml.Device) string {
	switch device := device.(type) {
	case *ddnvml.MIGDevice:
		return "mig"
	case *ddnvml.PhysicalDevice:
		if len(device.MIGChildren) > 0 {
			return "mig-parent"
		}
	}
	return "none"
}

// addCollector adds a collector to the telemetry, checking that the telemetry is not nil
func (t *CollectorTelemetry) addCollectorCreation(name CollectorName, status string, device ddnvml.Device) {
	if t == nil {
		return
	}
	tags := []string{status}
	collectorTags := collectorTelemetryTags(name, device)
	tags = append(tags, collectorTags...)
	t.Created.Add(1, tags...)
}
