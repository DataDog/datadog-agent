// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nvmlmetrics

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"
)

const nvmlGpmMetricsGetVersion = 1
const gpmMetricsCollectorName = "gpm"
const numGpmSamples = 2

type gpmMetric struct {
	name        string
	gpmMetricId nvml.GpmMetricId
}

type gpmMetricsCollector struct {
	lib                nvml.Interface
	device             nvml.Device
	samples            []nvml.GpmSample
	hasPreviousSample  bool
	currentSampleIndex int
}

func newGpmMetricsCollector(lib nvml.Interface, dev nvml.Device) (subsystemCollector, error) {
	c := &gpmMetricsCollector{
		lib: lib,
	}

	// First, check if the device supports GPM metrics
	support, ret := dev.GpmQueryDeviceSupport()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to query GPM support for device: %s", nvml.ErrorString(ret))
	}
	if support.IsSupportedDevice == 0 {
		return nil, errUnsupportedDevice
	}

	for i := 0; i < numGpmSamples; i++ {
		sample, ret := lib.GpmSampleAlloc()
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("failed to allocate GPM sample: %s", nvml.ErrorString(ret))
		}
		c.samples = append(c.samples, sample)
	}

	return c, nil
}

func (c *gpmMetricsCollector) name() string {
	return gpmMetricsCollectorName
}

// collectAllNvmlGpmMetrics collects all the GPM metrics from the given NVML device.
func (c *gpmMetricsCollector) collect() ([]Metric, error) {
	var err error

	metricsGet, err := c.getMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to get GPM metrics: %w", err)
	}

	var values []Metric
	if metricsGet != nil { // We can get nil if this is the first time we collect samples
		values = make([]Metric, 0, len(allNvmlGpmMetrics))
		for i, metric := range allNvmlGpmMetrics {
			if metricsGet.Metrics[i].NvmlReturn != uint32(nvml.SUCCESS) {
				err = multierror.Append(err, fmt.Errorf("failed to get metric %s: %s", metric.name, nvml.ErrorString(nvml.Return(metricsGet.Metrics[i].NvmlReturn))))
				continue
			}

			values = append(values, Metric{
				Name:  metric.name,
				Value: float64(metricsGet.Metrics[i].Value),
			})
		}
	}

	return values, err
}

func (c *gpmMetricsCollector) close() error {
	var err error
	for _, sample := range c.samples {
		sampleErr := sample.Free()
		if sampleErr != nvml.SUCCESS {
			err = multierror.Append(err, sampleErr)
		}
	}

	return err
}

func (c *gpmMetricsCollector) currentSample() nvml.GpmSample {
	return c.samples[c.currentSampleIndex]
}

func (c *gpmMetricsCollector) previousSample() nvml.GpmSample {
	return c.samples[(c.currentSampleIndex+1)%numGpmSamples]
}

func (c *gpmMetricsCollector) markCollectedSample() {
	c.hasPreviousSample = true
	c.currentSampleIndex = (c.currentSampleIndex + 1) % numGpmSamples
}

func (c *gpmMetricsCollector) getMetrics() (*nvml.GpmMetricsGetType, error) {
	ret := c.currentSample().Get(c.device)
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get GPM sample: %s", nvml.ErrorString(ret))
	}
	defer c.markCollectedSample()

	var metricsGet *nvml.GpmMetricsGetType
	if c.hasPreviousSample {
		metricsGet = &nvml.GpmMetricsGetType{
			Version:    nvmlGpmMetricsGetVersion,
			Sample1:    c.previousSample(),
			Sample2:    c.currentSample(),
			NumMetrics: 0,
		}

		// Fixed size array in the NVML API, so we need to control the number of
		// metrics we request. This is a sanity check, we are not requesting
		// anywhere close to the limit (which is 98 in the current code as of
		// Oct 2024), so we don't really need to do batching here
		maxMetrics := len(allNvmlGpmMetrics)
		for i, metric := range allNvmlGpmMetrics {
			if i >= maxMetrics {
				return nil, fmt.Errorf("too many GPM metrics to collect, max is %d", maxMetrics)
			}

			metricsGet.Metrics[i].MetricId = uint32(metric.gpmMetricId)
			metricsGet.NumMetrics++
		}

		ret := c.lib.GpmMetricsGet(metricsGet)
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("failed to get GPM metrics: %s", nvml.ErrorString(ret))
		}
	}

	return metricsGet, nil
}

var allNvmlGpmMetrics = []gpmMetric{
	{"pci.rx_throughput", nvml.GPM_METRIC_PCIE_RX_PER_SEC},
	{"pci.tx_throughput", nvml.GPM_METRIC_PCIE_TX_PER_SEC},
	{"pipe.fp16_active", nvml.GPM_METRIC_FP16_UTIL},
	{"pipe.fp32_active", nvml.GPM_METRIC_FP32_UTIL},
	{"pipe.fp64_active", nvml.GPM_METRIC_FP64_UTIL},
	{"pipe.tensor_active", nvml.GPM_METRIC_ANY_TENSOR_UTIL},
	{"pipe.int_active", nvml.GPM_METRIC_INTEGER_UTIL},
}
