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
	for _, apiCall := range memoryAPICallFactory {
		err := apiCall.testFunc(c.device)
		if err == nil || !ddnvml.IsUnsupported(err) {
			c.supportedAPICalls = append(c.supportedAPICalls, apiCall)
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
