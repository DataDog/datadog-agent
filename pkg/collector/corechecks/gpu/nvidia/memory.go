// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"fmt"

	"github.com/hashicorp/go-multierror"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type memoryAPICall struct {
	name     string
	testFunc func(ddnvml.Device) error
	callFunc func(*memoryCollector) ([]Metric, error)
}

var memoryAPICallFactory = []memoryAPICall{
	{
		name: "bar1_memory",
		testFunc: func(d ddnvml.Device) error {
			_, err := d.GetBAR1MemoryInfo()
			return err
		},
		callFunc: (*memoryCollector).collectBAR1MemoryMetrics,
	},
	{
		name: "device_memory_v2",
		testFunc: func(d ddnvml.Device) error {
			_, err := d.GetMemoryInfo_v2()
			return err
		},
		callFunc: (*memoryCollector).collectDeviceMemoryV2Metrics,
	},
	{
		name: "device_memory_v1",
		testFunc: func(d ddnvml.Device) error {
			_, err := d.GetMemoryInfo()
			return err
		},
		callFunc: (*memoryCollector).collectDeviceMemoryV1Metrics,
	},
}

type memoryCollector struct {
	device            ddnvml.Device
	supportedAPICalls []memoryAPICall
}

func newMemoryCollector(device ddnvml.Device) (Collector, error) {
	c := &memoryCollector{device: device}

	c.removeUnsupportedMetrics()
	if len(c.supportedAPICalls) == 0 {
		return nil, errUnsupportedDevice
	}

	return c, nil
}

func (c *memoryCollector) removeUnsupportedMetrics() {
	var hasDeviceMemoryAPI bool

	for _, apiCall := range memoryAPICallFactory {
		// For device memory APIs, prefer v2 over v1
		if apiCall.name == "device_memory_v1" && hasDeviceMemoryAPI {
			continue // Skip v1 if we already have v2
		}

		err := apiCall.testFunc(c.device)
		if err == nil || !ddnvml.IsUnsupported(err) {
			c.supportedAPICalls = append(c.supportedAPICalls, apiCall)

			// Mark that we have a device memory API
			if apiCall.name == "device_memory_v2" || apiCall.name == "device_memory_v1" {
				hasDeviceMemoryAPI = true
			}
		}
	}
}

func (c *memoryCollector) DeviceUUID() string {
	return c.device.GetDeviceInfo().UUID
}

func (c *memoryCollector) Name() CollectorName {
	return memory
}

func (c *memoryCollector) Collect() ([]Metric, error) {
	var allMetrics []Metric
	var multiErr error

	for _, apiCall := range c.supportedAPICalls {
		collectedMetrics, err := apiCall.callFunc(c)
		if err != nil {
			multiErr = multierror.Append(multiErr, fmt.Errorf("%s returned an error: %w", apiCall.name, err))
			continue
		}

		allMetrics = append(allMetrics, collectedMetrics...)
	}

	return allMetrics, multiErr
}

// collectBAR1MemoryMetrics collects BAR1 memory metrics with a single API call
func (c *memoryCollector) collectBAR1MemoryMetrics() ([]Metric, error) {
	bar1Info, err := c.device.GetBAR1MemoryInfo()
	if err != nil {
		return nil, err
	}

	return []Metric{
		{
			Name:  "memory.bar1.total",
			Value: float64(bar1Info.Bar1Total),
			Type:  metrics.GaugeType,
		},
		{
			Name:  "memory.bar1.free",
			Value: float64(bar1Info.Bar1Free),
			Type:  metrics.GaugeType,
		},
		{
			Name:  "memory.bar1.used",
			Value: float64(bar1Info.Bar1Used),
			Type:  metrics.GaugeType,
		},
	}, nil
}

// collectDeviceMemoryV2Metrics collects device memory metrics using v2 API (includes reserved memory)
func (c *memoryCollector) collectDeviceMemoryV2Metrics() ([]Metric, error) {
	memInfo, err := c.device.GetMemoryInfo_v2()
	if err != nil {
		return nil, err
	}

	return []Metric{
		{
			Name:  "memory.total",
			Value: float64(memInfo.Total),
			Type:  metrics.GaugeType,
		},
		{
			Name:  "memory.free",
			Value: float64(memInfo.Free),
			Type:  metrics.GaugeType,
		},
		{
			Name:  "memory.used",
			Value: float64(memInfo.Used),
			Type:  metrics.GaugeType,
		},
		{
			Name:  "memory.reserved",
			Value: float64(memInfo.Reserved),
			Type:  metrics.GaugeType,
		},
	}, nil
}

// collectDeviceMemoryV1Metrics collects device memory metrics using v1 API (fallback)
func (c *memoryCollector) collectDeviceMemoryV1Metrics() ([]Metric, error) {
	memInfo, err := c.device.GetMemoryInfo()
	if err != nil {
		return nil, err
	}

	return []Metric{
		{
			Name:  "memory.total",
			Value: float64(memInfo.Total),
			Type:  metrics.GaugeType,
		},
		{
			Name:  "memory.free",
			Value: float64(memInfo.Free),
			Type:  metrics.GaugeType,
		},
		{
			Name:  "memory.used",
			Value: float64(memInfo.Used),
			Type:  metrics.GaugeType,
		},
		// Note: v1 API doesn't provide reserved memory, so we don't emit that metric
	}, nil
}
