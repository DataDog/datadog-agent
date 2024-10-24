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

// nvmlGpmMetricsGetVersion is the version of the GPM metrics, there are no more versions as of now.
const nvmlGpmMetricsGetVersion = 1
const gpmMetricsCollectorName = "gpm"
const numGpmSamples = 2

// gpmMetricsCollector collects GPM metrics from an NVML device. GPM metrics are
// special as they are extracted from two samples. This means that, on each call
// to collect, we need to get a sample and, if we have two samples, only then
// get the metrics. To avoid reallocating memory, we preallocate a fixed number
// of samples (2), and we keep track of which sample is the current one.
type gpmMetricsCollector struct {
	lib                nvml.Interface
	device             nvml.Device
	samples            [numGpmSamples]nvml.GpmSample
	hasPreviousSample  bool
	currentSampleIndex int
	tags               []string
}

func newGpmMetricsCollector(lib nvml.Interface, dev nvml.Device, tags []string) (Collector, error) {
	var samples [numGpmSamples]nvml.GpmSample

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
		samples[i] = sample
	}

	return &gpmMetricsCollector{
		lib:     lib,
		samples: samples,
		device:  dev,
		tags:    tags,
	}, nil
}

// Name returns the name of the collector.
func (c *gpmMetricsCollector) Name() string {
	return gpmMetricsCollectorName
}

// Collect collects all the GPM metrics from the given NVML device.
func (c *gpmMetricsCollector) Collect() ([]Metric, error) {
	var err error

	metricsGet, err := c.sampleAndTryGetMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to get GPM metrics: %w", err)
	}
	if metricsGet == nil {
		return nil, nil // Not enough samples to collect metrics, return nothing
	}

	values := make([]Metric, 0, len(allNvmlGpmMetrics))
	for i, metric := range allNvmlGpmMetrics {
		if metricsGet.Metrics[i].NvmlReturn != uint32(nvml.SUCCESS) {
			err = multierror.Append(err, fmt.Errorf("failed to get metric %s: %s", metric.name, nvml.ErrorString(nvml.Return(metricsGet.Metrics[i].NvmlReturn))))
			continue
		}

		values = append(values, Metric{
			Name:  metric.name,
			Value: float64(metricsGet.Metrics[i].Value),
			Tags:  c.tags,
		})
	}

	return values, err
}

// Close cleans up any resources used by the collector.
func (c *gpmMetricsCollector) Close() error {
	var err error
	for _, sample := range c.samples {
		sampleErr := sample.Free()
		if sampleErr != nvml.SUCCESS {
			err = multierror.Append(err, sampleErr)
		}
	}

	return err
}

// currentSample returns the current GPM sample.
func (c *gpmMetricsCollector) currentSample() nvml.GpmSample {
	return c.samples[c.currentSampleIndex]
}

// previousSample returns the previous GPM sample.
func (c *gpmMetricsCollector) previousSample() nvml.GpmSample {
	return c.samples[(c.currentSampleIndex-1+numGpmSamples)%numGpmSamples]
}

// markCollectedSample marks the current sample as collected and updates the index, rotating through the samples.
func (c *gpmMetricsCollector) markCollectedSample() {
	c.hasPreviousSample = true
	c.currentSampleIndex = (c.currentSampleIndex + 1) % numGpmSamples
}

// getMetrics gets a sample and, if possible, returns the GPM metrics.
func (c *gpmMetricsCollector) sampleAndTryGetMetrics() (*nvml.GpmMetricsGetType, error) {
	ret := c.currentSample().Get(c.device)
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get GPM sample: %s", nvml.ErrorString(ret))
	}
	defer c.markCollectedSample() // always mark the sample as collected after getting it

	// We need at least two samples to get metrics, if we don't have a previous sample, return nil
	if !c.hasPreviousSample {
		return nil, nil
	}

	metricsGet := &nvml.GpmMetricsGetType{
		Version:    nvmlGpmMetricsGetVersion,
		Sample1:    c.previousSample(),
		Sample2:    c.currentSample(),
		NumMetrics: 0,
	}

	// metricsGet.Metrics is a fixed size array in the NVML API, so we need to
	// control the number of metrics we request. This is a sanity check, we are
	// not requesting anywhere close to the limit (which is 98 in the current
	// code as of Oct 2024), so we don't really need to do batching here
	maxMetrics := len(allNvmlGpmMetrics)
	for i, metric := range allNvmlGpmMetrics {
		if i >= maxMetrics {
			return nil, fmt.Errorf("too many GPM metrics to collect, max is %d", maxMetrics)
		}

		metricsGet.Metrics[i].MetricId = uint32(metric.gpmMetricID)
		metricsGet.NumMetrics++
	}

	ret = c.lib.GpmMetricsGet(metricsGet)
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get GPM metrics: %s", nvml.ErrorString(ret))
	}

	return metricsGet, nil
}

// gpmMetric struct holds the ID of a GPM metric to be retrieved and the
// corresponding name to assign to the outgoing metric.
type gpmMetric struct {
	name        string
	gpmMetricID nvml.GpmMetricId
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
