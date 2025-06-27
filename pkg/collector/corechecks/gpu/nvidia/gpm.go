// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/hashicorp/go-multierror"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const sampleBufferSize = 2

type gpmCollector struct {
	lib                 ddnvml.SafeNVML
	device              ddnvml.SafeDevice
	samples             []nvml.GpmSample
	metricsToCollect    map[nvml.GpmMetricId]gpmMetric
	nextSampleToCollect int
}

type gpmMetric struct {
	name       string
	metricType metrics.MetricType
}

var allGpmMetrics = map[nvml.GpmMetricId]gpmMetric{
	nvml.GPM_METRIC_GRAPHICS_UTIL: {
		name:       "gr_engine_active",
		metricType: metrics.GaugeType,
	},
	nvml.GPM_METRIC_SM_UTIL: {
		name:       "sm_active",
		metricType: metrics.GaugeType,
	},
	nvml.GPM_METRIC_SM_OCCUPANCY: {
		name:       "sm_occupancy",
		metricType: metrics.GaugeType,
	},
	nvml.GPM_METRIC_INTEGER_UTIL: {
		name:       "integer_active",
		metricType: metrics.GaugeType,
	},
	nvml.GPM_METRIC_FP16_UTIL: {
		name:       "fp16_active",
		metricType: metrics.GaugeType,
	},
	nvml.GPM_METRIC_FP32_UTIL: {
		name:       "fp32_active",
		metricType: metrics.GaugeType,
	},
	nvml.GPM_METRIC_FP64_UTIL: {
		name:       "fp64_active",
		metricType: metrics.GaugeType,
	},
	nvml.GPM_METRIC_ANY_TENSOR_UTIL: {
		name:       "tensor_active",
		metricType: metrics.GaugeType,
	},
}

func newGPMCollector(device ddnvml.SafeDevice) (c Collector, err error) {
	support, err := device.GpmQueryDeviceSupport()
	if err != nil {
		return nil, fmt.Errorf("failed to get GPM support: %w", err)
	}

	if support.IsSupportedDevice == 0 {
		return nil, errUnsupportedDevice
	}

	// Clone the global allGpmMetrics map to avoid mutating global state
	clonedMetrics := make(map[nvml.GpmMetricId]gpmMetric, len(allGpmMetrics))
	for key, value := range allGpmMetrics {
		clonedMetrics[key] = value
	}

	collector := &gpmCollector{
		device:           device,
		metricsToCollect: clonedMetrics,
	}

	collector.lib, err = ddnvml.GetSafeNvmlLib()
	if err != nil {
		return nil, fmt.Errorf("failed to get NVML library: %w", err)
	}

	defer func() {
		if err != nil {
			// return all allocated samples to NVML if we fail after they have been allocated
			collector.freeSamples()
		}
	}()

	for i := 0; i < sampleBufferSize; i++ {
		sample, err := collector.lib.GpmSampleAlloc()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate GPM sample: %w", err)
		}
		collector.samples = append(collector.samples, sample)
	}

	if err := collector.removeUnsupportedMetrics(); err != nil {
		return nil, fmt.Errorf("failed to remove unsupported metrics: %w", err)
	}

	if len(collector.metricsToCollect) == 0 {
		return nil, errUnsupportedDevice
	}

	return collector, nil
}

func (c *gpmCollector) removeUnsupportedMetrics() error {
	// Now collect two samples and try to get the metrics, to discard any unsupported ones
	for i := 0; i < 2; i++ {
		err := c.collectSample()
		if err != nil {
			return fmt.Errorf("failed to collect GPM sample: %w", err)
		}
	}

	metrics, err := c.calculateGpmMetrics()
	if err != nil {
		return fmt.Errorf("failed to get GPM metrics: %w", err)
	}

	for i := uint32(0); i < metrics.NumMetrics; i++ {
		if metrics.Metrics[i].NvmlReturn != uint32(nvml.SUCCESS) {
			delete(c.metricsToCollect, nvml.GpmMetricId(metrics.Metrics[i].MetricId))
		}
	}

	return nil
}

func (c *gpmCollector) collectSample() error {
	err := c.device.GpmSampleGet(c.samples[c.nextSampleToCollect])
	if err != nil {
		return fmt.Errorf("failed to collect GPM sample: %w", err)
	}
	c.nextSampleToCollect = (c.nextSampleToCollect + 1) % sampleBufferSize
	return nil
}

func (c *gpmCollector) freeSamples() {
	for _, sample := range c.samples {
		_ = c.lib.GpmSampleFree(sample)
	}
}

func (c *gpmCollector) getLastTwoSamples() (nvml.GpmSample, nvml.GpmSample) {
	// add sampleBufferSize to avoid negative indices
	lastCollected := (c.nextSampleToCollect - 1 + sampleBufferSize) % sampleBufferSize
	secondLastCollected := (c.nextSampleToCollect - 2 + sampleBufferSize) % sampleBufferSize

	return c.samples[lastCollected], c.samples[secondLastCollected]
}

func (c *gpmCollector) calculateGpmMetrics() (*nvml.GpmMetricsGetType, error) {
	sample1, sample2 := c.getLastTwoSamples()
	metricsGet := &nvml.GpmMetricsGetType{
		NumMetrics: uint32(len(c.metricsToCollect)),
		Version:    nvml.GPM_METRICS_GET_VERSION,
		Sample1:    sample1,
		Sample2:    sample2,
		Metrics:    [98]nvml.GpmMetric{},
	}

	metricIndex := 0
	for metricId := range c.metricsToCollect {
		metricsGet.Metrics[metricIndex] = nvml.GpmMetric{
			MetricId: uint32(metricId),
		}
		metricIndex++
	}

	err := c.lib.GpmMetricsGet(metricsGet)
	if err != nil {
		return nil, fmt.Errorf("failed to get GPM metrics: %w", err)
	}

	return metricsGet, nil
}

func (c *gpmCollector) DeviceUUID() string {
	uuid, _ := c.device.GetUUID()
	return uuid
}

func (c *gpmCollector) Name() CollectorName {
	return gpm
}

func (c *gpmCollector) Collect() ([]Metric, error) {
	err := c.collectSample()
	if err != nil {
		return nil, fmt.Errorf("failed to collect GPM sample: %w", err)
	}

	gpmMetrics, err := c.calculateGpmMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to get GPM metrics: %w", err)
	}

	metrics := make([]Metric, 0, len(c.metricsToCollect))
	for i := uint32(0); i < gpmMetrics.NumMetrics; i++ {
		metric := gpmMetrics.Metrics[i]
		if metric.NvmlReturn != uint32(nvml.SUCCESS) {
			err = multierror.Append(err, fmt.Errorf("failed to get GPM metric %d: %s", metric.MetricId, nvml.ErrorString(nvml.Return(metric.NvmlReturn))))
			continue
		}

		metricData, ok := c.metricsToCollect[nvml.GpmMetricId(metric.MetricId)]
		if !ok {
			err = multierror.Append(err, fmt.Errorf("unknown metric ID %d: %s", metric.MetricId, nvml.ErrorString(nvml.Return(metric.NvmlReturn))))
			continue
		}

		metrics = append(metrics, Metric{
			Name:  metricData.name,
			Value: metric.Value,
			Type:  metricData.metricType,
		})
	}

	return metrics, err
}
