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

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const baseNvlinkRxGpm = nvml.GPM_METRIC_NVLINK_L0_RX_PER_SEC
const baseNvlinkTxGpm = nvml.GPM_METRIC_NVLINK_L0_TX_PER_SEC
const maxNvlinkPorts = 18

type nvlinkGpmCollector struct {
	perPortCollector map[int]*gpmCollector
	device           ddnvml.Device
	deps             *CollectorDependencies
}

func newNVLinkGPMCollector(device ddnvml.Device, deps *CollectorDependencies) (Collector, error) {
	collector := &nvlinkGpmCollector{
		perPortCollector: make(map[int]*gpmCollector),
		device:           device,
		deps:             deps,
	}

	// no need to store the ports, they get added automatically to the perPortCollector map
	_, err := getSupportedNvlinkPorts(device, collector.getPortMetrics)
	if err != nil {
		return nil, err
	}

	return collector, nil
}

// Device returns the device this collector monitors.
func (c *nvlinkGpmCollector) Device() ddnvml.Device {
	return c.device
}

func (c *nvlinkGpmCollector) Name() CollectorName {
	return nvlinkGPM
}

func (c *nvlinkGpmCollector) Collect() ([]Sample, error) {
	var samples []Sample
	var errs []error
	for port := range c.perPortCollector {
		portSamples, err := c.getPortMetrics(port)
		if err != nil {
			errs = append(errs, fmt.Errorf("get port metrics for port %d: %w", port, err))
			continue
		}
		samples = append(samples, portSamples...)
	}
	return samples, errors.Join(errs...)
}

func (c *nvlinkGpmCollector) getOrCreateGpmCollector(port int) (*gpmCollector, error) {
	if collector, ok := c.perPortCollector[port]; ok {
		return collector, nil
	}

	if port > maxNvlinkPorts {
		return nil, fmt.Errorf("%w: port %d is out of range", errUnsupportedDevice, port)
	}

	portGpmMetrics := make(map[nvml.GpmMetricId]gpmMetric, 2)

	// GPM metric IDs are offset by 2 for each port, we have NVML_GPM_METRIC_NVLINK_L0_RX_PER_SEC = 62, then NVML_GPM_METRIC_NVLINK_L1_RX_PER_SEC = 64, etc.
	// so we can calculate the metric ID for the RX and TX metrics for the given port by adding 2*port to the base metric ID.
	// note that port is 1-indexed, so we subtract 1 to get the 0-indexed port.
	rxMetricID := int(baseNvlinkRxGpm) + 2*(port-1)
	txMetricID := int(baseNvlinkTxGpm) + 2*(port-1)

	portGpmMetrics[nvml.GpmMetricId(rxMetricID)] = gpmMetric{
		name:       "nvlink.throughput.data.rx",
		metricType: metrics.GaugeType,
	}
	portGpmMetrics[nvml.GpmMetricId(txMetricID)] = gpmMetric{
		name:       "nvlink.throughput.data.tx",
		metricType: metrics.GaugeType,
	}

	collector, err := newGPMCollectorWithMetrics(c.device, portGpmMetrics, c.deps)
	if err != nil {
		return nil, err
	}
	gpmCollector, ok := collector.(*gpmCollector)
	if !ok {
		return nil, errors.New("failed to cast collector to gpmCollector")
	}
	c.perPortCollector[port] = gpmCollector

	return gpmCollector, nil
}

func (c *nvlinkGpmCollector) getPortMetrics(port int) ([]Sample, error) {
	collector, err := c.getOrCreateGpmCollector(port)
	if err != nil {
		return nil, err
	}
	samples, err := collector.Collect()
	if err != nil {
		return nil, err
	}

	// GPM returns data in MiB/s, we need to convert to kB/s. Also, set priority high
	// to override metrics from fields collectors and add the nvlink_port tag.
	portTag := nvlinkPortTag(port)
	for _, sample := range samples {
		metric, ok := sample.(*Metric)
		if !ok {
			return nil, fmt.Errorf("GPM collector returned unexpected sample type %T", sample)
		}
		metric.Value = metric.Value * 1024
		metric.priority = High
		metric.tags = append(metric.tags, portTag)
	}
	return samples, nil
}
