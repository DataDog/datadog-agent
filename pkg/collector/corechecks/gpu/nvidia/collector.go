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

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	tagVendor    = "gpu_vendor:nvidia"
	tagNameModel = "gpu_model"
	tagNameUUID  = "gpu_uuid"
)

// Collector defines a collector that gets metric from a specific NVML subsystem and device
type Collector interface {
	// Collect collects metrics from the given NVML device. This method should not fill the tags
	// unless they're metric-specific (i.e., all device-specific tags will be added by the Collector itself)
	Collect() ([]Metric, error)

	// Close closes the subsystem and releases any resources it might have allocated
	Close() error

	// name returns the name of the subsystem
	Name() string
}

// Metric represents a single metric collected from the NVML library.
type Metric struct {
	Name  string   // Name holds the name of the metric.
	Value float64  // Value holds the value of the metric.
	Tags  []string // Tags holds the tags associated with the metric.
}

// errUnsupportedDevice is returned when the device does not support the given collector
var errUnsupportedDevice = errors.New("device does not support the given collector")

// subsystemBuilder is a function that creates a new subsystemCollector. lib is the NVML
// library interface, and device the device it should collect metrics from. It also receives
// the tags associated with the device, the collector should use them when generating metrics.
type subsystemBuilder func(lib nvml.Interface, device nvml.Device, tags []string) (Collector, error)

// allSubsystems is a map of all the subsystems that can be used to collect metrics from NVML.
var allSubsystems = map[string]subsystemBuilder{
	fieldsCollectorName:       newFieldsCollector,
	deviceCollectorName:       newDeviceCollector,
	remappedRowsCollectorName: newRemappedRowsCollector,
	clocksCollectorName:       newClocksCollector,
}

// BuildCollectors returns a set of collectors that can be used to collect metrics from NVML.
func BuildCollectors(lib nvml.Interface) ([]Collector, error) {
	return buildCollectors(lib, allSubsystems)
}

func buildCollectors(lib nvml.Interface, subsystems map[string]subsystemBuilder) ([]Collector, error) {
	var collectors []Collector

	devCount, ret := lib.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get device count: %s", nvml.ErrorString(ret))
	}

	for i := 0; i < devCount; i++ {
		dev, ret := lib.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("failed to get device handle for index %d: %s", i, nvml.ErrorString(ret))
		}

		tags := getTagsFromDevice(dev)

		for name, builder := range subsystems {
			subsystem, err := builder(lib, dev, tags)
			if errors.Is(err, errUnsupportedDevice) {
				log.Warnf("device %s does not support collector %s", dev, name)
				continue
			} else if err != nil {
				log.Warnf("failed to create subsystem %s: %s", name, err)
				continue
			}

			collectors = append(collectors, subsystem)
		}
	}

	return collectors, nil
}

// getTagsFromDevice returns the tags associated with the given NVML device.
func getTagsFromDevice(dev nvml.Device) []string {
	tags := []string{tagVendor}

	uuid, ret := dev.GetUUID()
	if ret == nvml.SUCCESS {
		tags = append(tags, fmt.Sprintf("%s:%s", tagNameUUID, uuid))
	} else {
		log.Warnf("failed to get device UUID: %s", nvml.ErrorString(ret))
	}

	name, ret := dev.GetName()
	if ret == nvml.SUCCESS {
		tags = append(tags, fmt.Sprintf("%s:%s", tagNameModel, name))
	} else {
		log.Warnf("failed to get device name: %s", nvml.ErrorString(ret))
	}

	return tags
}
