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
	"errors"
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	tagVendor    = "gpu_vendor:nvidia"
	tagNameModel = "gpu_model"
	tagNameUUID  = "gpu_uuid"
)

// Collector is the main struct responsible for collecting metrics from the NVML library.
type Collector struct {
	lib        nvml.Interface
	collectors map[nvml.Device][]subsystemCollector
}

// errUnsupportedDevice is returned when the device does not support the given collector
var errUnsupportedDevice = errors.New("device does not support the given collector")

// Metric represents a single metric collected from the NVML library.
type Metric struct {
	Name  string   // Name holds the name of the metric.
	Value float64  // Value holds the value of the metric.
	Tags  []string // Tags holds the tags associated with the metric.
}

// subsystemFactory is a function that creates a new subsystemCollector. lib is the NVML
// library interface, and device the device it should collect metrics from
type subsystemFactory func(lib nvml.Interface, device nvml.Device) (subsystemCollector, error)

// subsystemCollector defines a collector that gets metric from a specific NVML subsystem and device
type subsystemCollector interface {
	// collect collects metrics from the given NVML device. This method should not fill the tags
	// unless they're metric-specific (i.e., all device-specific tags will be added by the Collector itself)
	collect() ([]Metric, error)

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
	coll := &Collector{
		lib:        lib,
		collectors: make(map[nvml.Device][]subsystemCollector),
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

		for name, factory := range subsystems {
			subsystem, err := factory(lib, dev)
			if errors.Is(err, errUnsupportedDevice) {
				log.Warnf("device %s does not support collector %s", dev, name)
				continue
			} else if err != nil {
				log.Warnf("failed to create subsystem %s: %s", name, err)
				continue
			}

			coll.collectors[dev] = append(coll.collectors[dev], subsystem)
		}
	}

	return coll, nil
}

// Collect collects metrics from all the subsystems and returns them. It will try to return as many
// metrics as possible, even if some subsystems fail to collect metrics. This means that even if the
// error return is not nil, the returned metrics might still be useful.
func (coll *Collector) Collect() ([]Metric, error) {
	var allMetrics []Metric
	var err error

	for dev, collectors := range coll.collectors {
		tags := getTagsFromDevice(dev)

		for _, subsystem := range collectors {
			metrics, collectErr := subsystem.collect()
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

	for _, collectors := range coll.collectors {
		for _, subsystem := range collectors {
			if err := subsystem.close(); err != nil {
				errs = multierror.Append(errs, err)
			}
		}
	}

	return errs
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
