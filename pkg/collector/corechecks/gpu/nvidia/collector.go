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
	"maps"
	"slices"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// errUnsupportedDevice is returned when the device does not support the given collector
var errUnsupportedDevice = errors.New("device does not support the given collector")

// CollectorName is the name of the nvml sub-collectors
type CollectorName string

const (
	field        CollectorName = "fields"
	clock        CollectorName = "clocks"
	device       CollectorName = "device"
	remappedRows CollectorName = "remapped_rows"
	samples      CollectorName = "samples"
	nvlink       CollectorName = "nvlink"
	gpm          CollectorName = "gpm"
)

// Metric represents a single metric collected from the NVML library.
type Metric struct {
	Name     string  // Name holds the name of the metric.
	Value    float64 // Value holds the value of the metric.
	Type     metrics.MetricType
	Priority int // Priority is the priority of the metric, indicating which metric to keep in case of duplicates. 0 (default) is the lowest priority.
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
type subsystemBuilder func(device ddnvml.SafeDevice) (Collector, error)

// factory is a map of all the subsystems that can be used to collect metrics from NVML.
var factory = map[CollectorName]subsystemBuilder{
	field:        newFieldsCollector,
	device:       newDeviceCollector,
	remappedRows: newRemappedRowsCollector,
	clock:        newClocksCollector,
	samples:      newSamplesCollector,
	nvlink:       newNVLinkCollector,
	gpm:          newGPMCollector,
}

// CollectorDependencies holds the dependencies needed to create a set of collectors.
type CollectorDependencies struct {
	// DeviceCache is a cache of GPU devices.
	DeviceCache ddnvml.DeviceCache
}

// BuildCollectors returns a set of collectors that can be used to collect metrics from NVML.
func BuildCollectors(deps *CollectorDependencies) ([]Collector, error) {
	return buildCollectors(deps, factory)
}

func buildCollectors(deps *CollectorDependencies, builders map[CollectorName]subsystemBuilder) ([]Collector, error) {
	var collectors []Collector

	for _, dev := range deps.DeviceCache.All() {
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

	return collectors, nil
}

// GetDeviceTagsMapping returns the mapping of tags per GPU device.
func GetDeviceTagsMapping(deviceCache ddnvml.DeviceCache, tagger tagger.Component) map[string][]string {
	devCount := deviceCache.Count()
	if devCount == 0 {
		return nil
	}

	tagsMapping := make(map[string][]string, devCount)

	for _, dev := range deviceCache.All() {
		uuid := dev.GetDeviceInfo().UUID
		entityID := taggertypes.NewEntityID(taggertypes.GPU, uuid)
		tags, err := tagger.Tag(entityID, taggertypes.ChecksConfigCardinality)
		if err != nil {
			log.Warnf("Error collecting GPU tags for GPU UUID %s: %s", uuid, err)
		}

		if len(tags) == 0 {
			// If we get no tags (either WMS hasn't collected GPUs yet, or we are running the check standalone with 'agent check')
			// add at least the UUID as a tag to distinguish the values.
			tags = []string{fmt.Sprintf("gpu_uuid:%s", uuid)}
		}

		tagsMapping[uuid] = tags
	}

	return tagsMapping
}

// RemoveDuplicateMetrics removes duplicate metrics from the given list, keeping the highest priority metric.
func RemoveDuplicateMetrics(metrics []Metric) []Metric {
	metricsByName := make(map[string]Metric)

	for _, metric := range metrics {
		if existing, ok := metricsByName[metric.Name]; !ok || existing.Priority < metric.Priority {
			metricsByName[metric.Name] = metric
		}
	}

	return slices.Collect(maps.Values(metricsByName))
}
