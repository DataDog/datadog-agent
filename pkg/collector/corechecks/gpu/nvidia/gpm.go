// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"fmt"
	"maps"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/hashicorp/go-multierror"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const sampleBufferSize = 2

type gpmCollector struct {
	lib                 ddnvml.SafeNVML
	device              ddnvml.SafeDevice
	samples             [sampleBufferSize]nvml.GpmSample
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
		return nil, fmt.Errorf("failed to query for GPM support: %w", err)
	}

	if support.IsSupportedDevice == 0 {
		return nil, errUnsupportedDevice
	}

	// Clone the global allGpmMetrics map to avoid mutating global state
	clonedMetrics := maps.Clone(allGpmMetrics)

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
		collector.samples[i] = sample
	}

	collector.removeUnsupportedMetrics()

	if len(collector.metricsToCollect) == 0 {
		return nil, errUnsupportedDevice
	}

	return collector, nil
}

func (c *gpmCollector) removeUnsupportedMetrics() {
	// Now collect two samples and try to get the metrics, to discard any unsupported ones
	// It's a best-effort approach, so any errors in the process are ignored. If they are not temporary,
	// the collector will fail to collect metrics later and that will show in the logs.
	for i := 0; i < 2; i++ {
		err := c.collectSample()
		if err != nil {
			return
		}
	}

	metrics, err := c.calculateGpmMetrics()
	if err != nil {
		return
	}

	for i := uint32(0); i < metrics.NumMetrics; i++ {
		if metrics.Metrics[i].NvmlReturn != uint32(nvml.SUCCESS) {
			delete(c.metricsToCollect, nvml.GpmMetricId(metrics.Metrics[i].MetricId))
		}
	}
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
		if sample != nil {
			_ = c.lib.GpmSampleFree(sample)
		}
	}
}

// getLastTwoSamples returns the last two samples collected (first and second return values)
// example: lastSample, secondToLastSample = getLastTwoSamples
func (c *gpmCollector) getLastTwoSamples() (nvml.GpmSample, nvml.GpmSample) {
	// Treat c.samples as a circular buffer, so we can get the last two samples by using the current index
	// and subtracting from that.
	// add sampleBufferSize to avoid negative indices.
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
	}

	metricIndex := 0
	for metricID := range c.metricsToCollect {
		metricsGet.Metrics[metricIndex] = nvml.GpmMetric{
			MetricId: uint32(metricID),
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
			Name:     metricData.name,
			Value:    metric.Value,
			Type:     metricData.metricType,
			Priority: 10, // All GPM metrics have priority over other collectors
		})
	}

	return metrics, err
}
