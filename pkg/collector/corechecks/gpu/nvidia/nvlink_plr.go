// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	"github.com/DataDog/datadog-agent/pkg/gpu/prm"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type prmMetricsSource interface {
	RegisterRequest(model.PRMRequest)
	GetCounters(deviceUUID string, port int) (map[string]uint64, error)
}

type nvlinkPLRCollector struct {
	device   ddnvml.Device
	ports    []int
	prmCache prmMetricsSource
}

func newNVLinkPLRCollector(device ddnvml.Device, deps *CollectorDependencies) (Collector, error) {
	if deps == nil || deps.PRMCache == nil {
		return nil, fmt.Errorf("%w: PRM cache is required for NVLink PLR collector", errUnsupportedDevice)
	}

	c := &nvlinkPLRCollector{
		device:   device,
		prmCache: deps.PRMCache,
	}

	if device.GetDeviceInfo().Architecture < nvml.DEVICE_ARCH_BLACKWELL {
		return nil, fmt.Errorf("%w: NVLink PLR PRM metrics require Blackwell or newer architecture", errUnsupportedDevice)
	}

	var err error
	c.ports, err = getSupportedNvlinkPorts(device, portIsAlwaysSupported)
	if err != nil {
		return nil, fmt.Errorf("get supported NVLink ports: %w", err)
	}

	for _, port := range c.ports {
		c.prmCache.RegisterRequest(model.PRMRequest{
			DeviceUUID: device.GetDeviceInfo().UUID,
			Port:       port,
			Group:      prm.PPCNTGroupPLR,
		})
	}

	return c, nil
}

// Device returns the device this collector monitors.
func (c *nvlinkPLRCollector) Device() ddnvml.Device {
	return c.device
}

func (c *nvlinkPLRCollector) Name() CollectorName {
	return nvlinkPLR
}

func (c *nvlinkPLRCollector) Collect() ([]Sample, error) {
	var (
		allSamples []Sample
		multiErr   []error
	)

	for _, port := range c.ports {
		samples, err := c.getPortMetrics(port)
		if err != nil {
			multiErr = append(multiErr, fmt.Errorf("get port metrics for port %d: %w", port, err))
			continue
		}
		allSamples = append(allSamples, samples...)
	}

	if len(allSamples) == 0 && len(multiErr) > 0 {
		return nil, errors.Join(multiErr...)
	}

	return allSamples, errors.Join(multiErr...)
}

func (c *nvlinkPLRCollector) getPortMetrics(port int) ([]Sample, error) {
	var samples []Sample

	counters, err := c.prmCache.GetCounters(c.Device().GetDeviceInfo().UUID, port)
	if err != nil {
		return nil, fmt.Errorf("get port metrics for port %d: %w", port, err)
	}

	var multiErr []error

	for _, field := range prm.PLRCounterFields {
		value, found := counters[field]
		if !found {
			multiErr = append(multiErr, fmt.Errorf("missing PLR counter %q for port %d", field, port))
			continue
		}

		samples = append(samples, &Metric{
			baseSample: baseSample{priority: Medium, tags: []string{nvlinkPortTag(port)}},
			Name:       field,
			Value:      float64(value),
			Type:       metrics.GaugeType,
		})
	}

	return samples, errors.Join(multiErr...)
}
