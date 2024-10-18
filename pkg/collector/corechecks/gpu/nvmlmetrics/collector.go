// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package nvmlmetrics holds the logic to collect metrics from the NVIDIA Management Library (NVML).
// The main entry point is the Collector struct, which is responsible for collecting metrics from the NVML library.
// There are multiple subsystems that can be used to collect metrics from NVML: GPM, FieldInfo functions, and general
// API functions. The collection of metrics is done by the subsystems, implementing the subsystemCollector interface.
package nvmlmetrics

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"
)

// Collector is the main struct responsible for collecting metrics from the NVML library.
type Collector struct {
	lib        nvml.Interface
	collectors []subsystemCollector
	devices    []nvml.Device
}

// Metric represents a single metric collected from the NVML library.
type Metric struct {
	Name  string   // Name holds the name of the metric.
	Value float64  // Value holds the value of the metric.
	Tags  []string // Tags holds the tags associated with the metric.
}

// subsystemFactory is a function that creates a new subsystemCollector. lib is the NVML
// library interface, and devices the list of devices that are present in the system in case
// the subsystem needs to preallocate structures
type subsystemFactory func(lib nvml.Interface, devices []nvml.Device) (subsystemCollector, error)

// subsystemCollector defines a collector that gets metric from a specific NVML subsystem
type subsystemCollector interface {
	// collectMetrics collects metrics from the given NVML device. This method should not fill the tags
	// unless they're metric-specific (i.e., all device-specific tags will be added by the Collector itself)
	collectMetrics(dev nvml.Device) ([]Metric, error)

	// close closes the subsystem and releases any resources it might have allocated
	close() error

	// name returns the name of the subsystem
	name() string
}

// allSubsystems is a map of all the subsystems that can be used to collect metrics from NVML.
var allSubsystems = map[string]subsystemFactory{}

// NewCollector creates a new Collector that will collect metrics from the given NVML library.
func NewCollector(lib nvml.Interface) (*Collector, error) {
	return newCollectorWithSubsystems(lib, allSubsystems)
}

// newCollectorWithSubsystems allows specifying which subsystems to use when creating the collector, useful for tests.
func newCollectorWithSubsystems(lib nvml.Interface, subsystems map[string]subsystemFactory) (*Collector, error) {
	ret := nvml.SUCCESS
	coll := &Collector{
		lib: lib,
	}

	devCount, ret := lib.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get device count: %s", nvml.ErrorString(ret))
	}

	for i := 0; i < devCount; i++ {
		dev, ret := coll.lib.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("failed to get device handle for index %d: %s", i, nvml.ErrorString(ret))
		}

		coll.devices = append(coll.devices, dev)
	}

	for name, factory := range subsystems {
		subsystem, err := factory(lib, coll.devices)
		if err != nil {
			_ = coll.Close() // Close all previously created subsystems
			return nil, fmt.Errorf("failed to create subsystem %s: %w", name, err)
		}

		coll.collectors = append(coll.collectors, subsystem)
	}

	return coll, nil
}

// Collect collects metrics from all the subsystems and returns them. It will try to return as many
// metrics as possible, even if some subsystems fail to collect metrics. This means that even if the
// error return is not nil, the returned metrics might still be useful.
func (coll *Collector) Collect() ([]Metric, error) {
	var allMetrics []Metric
	var err error

	for _, dev := range coll.devices {
		tags, tagsErr := getTagsFromDevice(dev)
		if tagsErr != nil {
			return allMetrics, fmt.Errorf("failed to get tags for device: %w", tagsErr)
		}

		for _, subsystem := range coll.collectors {
			metrics, collectErr := subsystem.collectMetrics(dev)
			if collectErr != nil {
				err = multierror.Append(err, fmt.Errorf("failed to collect metrics for subsystem %s: %w", subsystem.name(), collectErr))
			}

			for _, metric := range metrics {
				metric.Tags = append(metric.Tags, tags...)
				allMetrics = append(allMetrics, metric)
			}
		}
	}

	return allMetrics, err
}

// Close closes the collector and releases any resources it might have allocated.
func (coll *Collector) Close() error {
	var errs error

	for _, subsystem := range coll.collectors {
		if err := subsystem.close(); err != nil {
			errs = multierror.Append(errs, err)
		}
	}

	return errs
}

// getTagsFromDevice returns the tags associated with the given NVML device.
func getTagsFromDevice(dev nvml.Device) ([]string, error) {
	uuid, ret := dev.GetUUID()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get device UUID: %s", nvml.ErrorString(ret))
	}

	name, ret := dev.GetName()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get device name: %s", nvml.ErrorString(ret))
	}

	return []string{
		fmt.Sprintf("gpu_device_uuid:%s", uuid),
		fmt.Sprintf("gpu_device_model:%s", name),
		"gpu_device_vendor:nvidia",
	}, nil
}
