// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package nvidia holds the logic to collect metrics from the NVIDIA Management Library (NVML).
// The main entry point is the BuildCollectors functions, which returns a set of collectors that will
// gather metrics from the available NVIDIA devices on the system. Each collector is responsible for
// a specific subsystem of metrics, such as device metrics, GPM metrics, etc. The collected metrics will
// be returned with the associated tags for each device.
package nvidia

import (
	"errors"
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
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
)

// Metric represents a single metric collected from the NVML library.
type Metric struct {
	Name  string   // Name holds the name of the metric.
	Value float64  // Value holds the value of the metric.
	Tags  []string // Tags holds the tags associated with the metric.
	Type  metrics.MetricType
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
type subsystemBuilder func(device nvml.Device, tags []string) (Collector, error)

// factory is a map of all the subsystems that can be used to collect metrics from NVML.
var factory = map[CollectorName]subsystemBuilder{
	field:        newFieldsCollector,
	device:       newDeviceCollector,
	remappedRows: newRemappedRowsCollector,
	clock:        newClocksCollector,
	samples:      newSamplesCollector,
}

// CollectorDependencies holds the dependencies needed to create a set of collectors.
type CollectorDependencies struct {
	// Tagger is the tagger component used to tag the metrics.
	Tagger tagger.Component

	// NVML is the NVML library interface used to interact with the NVIDIA devices.
	NVML nvml.Interface
}

// BuildCollectors returns a set of collectors that can be used to collect metrics from NVML.
func BuildCollectors(deps *CollectorDependencies) ([]Collector, error) {
	return buildCollectors(deps, factory)
}

func buildCollectors(deps *CollectorDependencies, builders map[CollectorName]subsystemBuilder) ([]Collector, error) {
	var collectors []Collector

	devCount, ret := deps.NVML.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get device count: %s", nvml.ErrorString(ret))
	}

	for i := 0; i < devCount; i++ {
		dev, ret := deps.NVML.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("failed to get device handle for index %d: %s", i, nvml.ErrorString(ret))
		}

		tags, err := getTagsFromDevice(dev, deps.Tagger)
		if err != nil {
			log.Warnf("failed to get tags for device %s: %s", dev, err)
			continue
		}

		for name, builder := range builders {
			c, err := builder(dev, tags)
			if errors.Is(err, errUnsupportedDevice) {
				log.Warnf("device %s does not support collector %s", dev, name)
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

// getTagsFromDevice returns the tags associated with the given NVML device.
func getTagsFromDevice(dev nvml.Device, tagger tagger.Component) ([]string, error) {
	uuid, ret := dev.GetUUID()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get device UUID: %s", nvml.ErrorString(ret))
	}

	entityID := taggertypes.NewEntityID(taggertypes.GPU, uuid)
	tags, err := tagger.Tag(entityID, tagger.ChecksCardinality())
	if err != nil {
		log.Warnf("Error collecting GPU tags for GPU UUID %s: %s", uuid, err)
	}

	if len(tags) == 0 {
		// If we get no tags (either WMS hasn't collected GPUs yet, or we are running the check standalone with 'agent check')
		// add at least the UUID as a tag to distinguish the values.
		tags = []string{fmt.Sprintf("gpu_uuid:%s", uuid)}
	}

	return tags, nil
}
