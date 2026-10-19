// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

// Package nvidia holds the logic to collect metrics from the NVIDIA Management Library (NVML).
// The main entry point is CollectorBuilder, whose Build method returns a set of collectors that will
// gather metrics from the available NVIDIA devices on the system. Each collector is responsible for
// a specific subsystem of metrics, such as device metrics, GPM metrics, etc. The collected metrics will
// be returned with the associated tags for each device.
package nvidia

import (
	"errors"
	"slices"
	"strconv"
	"sync"
	"time"

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
	field           CollectorName = "fields"
	gpm             CollectorName = "gpm"
	ebpf            CollectorName = "ebpf"
	deviceEvents    CollectorName = "device_events"
	nvlinkPLR       CollectorName = "nvlink_plr"
	nvlinkFEC       CollectorName = "nvlink_fec"
	nvlinkFields    CollectorName = "nvlink_fields"
	nvlinkStateless CollectorName = "nvlink_stateless"
	nvlinkGPM       CollectorName = "nvlink_gpm"
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
	field:           newFieldsCollector,
	nvlinkPLR:       newNVLinkPLRCollector,
	nvlinkFEC:       newNVLinkFECCollector,
	nvlinkFields:    newNVLinkFieldsCollector,
	nvlinkStateless: newNVLinkStatelessCollector,
	nvlinkGPM:       newNVLinkGPMCollector,
	gpm:             newGPMCollector,
	deviceEvents:    newDeviceEventsCollector,
	ebpf:            newEbpfCollector,
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

const (
	minRetryBackoff = 30 * time.Second
	maxRetryBackoff = 5 * time.Minute
)

// retryBackoff schedules the retries of an operation that keeps failing, such as creating a collector, doubling
// the time between attempts after each failure, from minRetryBackoff up to maxRetryBackoff.
type retryBackoff struct {
	failures    int
	nextAttempt time.Time
}

func (r *retryBackoff) canRetry(now time.Time) bool {
	return !now.Before(r.nextAttempt)
}

// recordFailure records a new failure, returning the time to wait before the next attempt
func (r *retryBackoff) recordFailure(now time.Time) time.Duration {
	r.failures++
	backoff := maxRetryBackoff
	// cap the shift to avoid overflowing for operations that keep failing
	if shift := r.failures - 1; shift < 10 {
		backoff = min(minRetryBackoff<<shift, maxRetryBackoff)
	}
	r.nextAttempt = now.Add(backoff)
	return backoff
}

// CollectorBuilder builds the NVML collectors for each device, and retries with a backoff the ones that failed to be
// created (e.g. due to a permission error). Collectors that are disabled or unsupported by the device are not retried.
type CollectorBuilder struct {
	builders map[CollectorName]subsystemBuilder         // if nil, the package-level factory is used
	failed   map[string]map[CollectorName]*retryBackoff // device UUID -> collector name -> retry state
	now      func() time.Time
}

// NewCollectorBuilder creates a new CollectorBuilder with all the available collectors
func NewCollectorBuilder() *CollectorBuilder {
	// builders are resolved on each build, as the factory can be replaced in tests
	return newCollectorBuilder(nil)
}

func newCollectorBuilder(builders map[CollectorName]subsystemBuilder) *CollectorBuilder {
	return &CollectorBuilder{
		builders: builders,
		failed:   make(map[string]map[CollectorName]*retryBackoff),
		now:      time.Now,
	}
}

// Reset discards all the recorded failures
func (b *CollectorBuilder) Reset() {
	clear(b.failed)
}

func (b *CollectorBuilder) subsystemBuilders() map[CollectorName]subsystemBuilder {
	if b.builders == nil {
		return factory
	}
	return b.builders
}

// Build builds all the collectors for the given devices. Devices with collectors pending a retry are skipped,
// as those collectors are retried with RetryFailed.
func (b *CollectorBuilder) Build(devices []ddnvml.Device, deps *CollectorDependencies) []Collector {
	if len(devices) == 0 {
		return nil
	}

	builders := b.subsystemBuilders()

	// Check that the disabled collectors are valid
	for _, disabled := range deps.Config.DisabledCollectors {
		if _, ok := builders[CollectorName(disabled)]; !ok {
			log.Warnf("invalid disabled collector: %s", disabled)
			continue
		}
	}

	var collectors []Collector

	// Step 1: Build NVML collectors for physical devices only,
	// (since most of NVML API doesn't support MIG devices)
	for _, dev := range devices {
		if len(b.failed[dev.GetDeviceInfo().UUID]) > 0 {
			continue
		}

		for name, builder := range builders {
			if collectorDisabled(name, deps.Config) {
				log.Debugf("Skipping disabled collector %s for device %s", name, dev.GetDeviceInfo().UUID)
				deps.Telemetry.addCollectorCreation(name, "disabled", dev)
				continue
			}

			if c := b.buildCollector(name, builder, dev, deps); c != nil {
				collectors = append(collectors, c)
			}
		}
	}

	return collectors
}

// RetryFailed attempts again to build the collectors that previously failed to be created for the given devices,
// if their backoff period has expired. Failures recorded for devices that are not in the list are discarded.
func (b *CollectorBuilder) RetryFailed(devices []ddnvml.Device, deps *CollectorDependencies) []Collector {
	currentDevices := make(map[string]ddnvml.Device, len(devices))
	for _, dev := range devices {
		currentDevices[dev.GetDeviceInfo().UUID] = dev
	}

	builders := b.subsystemBuilders()
	var collectors []Collector
	for deviceUUID, failedCollectors := range b.failed {
		dev, ok := currentDevices[deviceUUID]
		if !ok {
			delete(b.failed, deviceUUID)
			continue
		}

		for name, failure := range failedCollectors {
			builder, ok := builders[name]
			if !ok {
				b.clearFailure(deviceUUID, name)
				continue
			}
			if !failure.canRetry(b.now()) {
				continue
			}

			if c := b.buildCollector(name, builder, dev, deps); c != nil {
				log.Infof("created collector %s for device %s after %d failed attempts", name, deviceUUID, failure.failures)
				collectors = append(collectors, c)
			}
		}
	}

	return collectors
}

// buildCollector builds a collector for the given device, recording failures other than errUnsupportedDevice to retry
// them later. It returns nil if the collector could not be created.
func (b *CollectorBuilder) buildCollector(name CollectorName, builder subsystemBuilder, dev ddnvml.Device, deps *CollectorDependencies) Collector {
	deviceUUID := dev.GetDeviceInfo().UUID

	c, err := builder(dev, deps)
	if errors.Is(err, errUnsupportedDevice) {
		log.Warnf("device %s does not support collector %s", deviceUUID, name)
		deps.Telemetry.addCollectorCreation(name, "unsupported", dev)
		b.clearFailure(deviceUUID, name)
		return nil
	} else if err != nil {
		failures, retryIn := b.recordFailure(deviceUUID, name)
		if failures == 1 {
			log.Warnf("failed to create collector %s for device %s, will retry in %s: %s", name, deviceUUID, retryIn, err)
		} else {
			log.Debugf("failed to create collector %s for device %s after %d attempts, will retry in %s: %s", name, deviceUUID, failures, retryIn, err)
		}
		deps.Telemetry.addCollectorCreation(name, "error", dev)
		return nil
	}

	deps.Telemetry.addCollectorCreation(name, "success", dev)
	b.clearFailure(deviceUUID, name)
	return c
}

// recordFailure records a failure to create the given collector, returning the number of consecutive failures and
// the time to wait before retrying it.
func (b *CollectorBuilder) recordFailure(deviceUUID string, name CollectorName) (int, time.Duration) {
	if b.failed[deviceUUID] == nil {
		b.failed[deviceUUID] = make(map[CollectorName]*retryBackoff)
	}
	failure := b.failed[deviceUUID][name]
	if failure == nil {
		failure = &retryBackoff{}
		b.failed[deviceUUID][name] = failure
	}

	retryIn := failure.recordFailure(b.now())
	return failure.failures, retryIn
}

func (b *CollectorBuilder) clearFailure(deviceUUID string, name CollectorName) {
	delete(b.failed[deviceUUID], name)
	if len(b.failed[deviceUUID]) == 0 {
		delete(b.failed, deviceUUID)
	}
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
// It belongs in this package because CollectorBuilder records creation outcomes, including
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
