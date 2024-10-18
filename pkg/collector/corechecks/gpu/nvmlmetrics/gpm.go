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

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const nvmlGpmMetricsGetVersion = 1
const gpmMetricsCollectorName = "gpm"

type gpmMetric struct {
	name        string
	gpmMetricId nvml.GpmMetricId
}

type gpmMetricsCollector struct {
	lib     nvml.Interface
	samples map[string]deviceSamples
}

func newGpmMetricsCollector(lib nvml.Interface, devices []nvml.Device) (subsystemCollector, error) {
	c := &gpmMetricsCollector{
		lib:     lib,
		samples: make(map[string]deviceSamples),
	}

	for _, dev := range devices {
		uuid, ret := dev.GetUUID()
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("failed to get device UUID: %s", nvml.ErrorString(ret))
		}

		// First, check if the device supports GPM metrics
		support, ret := dev.GpmQueryDeviceSupport()
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("failed to query GPM support for device %s: %s", uuid, nvml.ErrorString(ret))
		}
		if support.IsSupportedDevice == 0 {
			log.Warnf("GPU device %s does not support GPM metrics", uuid)
			continue
		}

		samples, err := newDeviceSamples(lib, 2)
		if err != nil {
			return nil, fmt.Errorf("failed to create GPM samples for device %s: %w", uuid, err)
		}

		c.samples[uuid] = samples
	}

	return c, nil
}

func (c *gpmMetricsCollector) name() string {
	return gpmMetricsCollectorName
}

// collectAllNvmlGpmMetrics collects all the GPM metrics from the given NVML device.
func (c *gpmMetricsCollector) collectMetrics(dev nvml.Device) ([]Metric, error) {
	var err error

	uuid, ret := dev.GetUUID()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get device index: %s", nvml.ErrorString(ret))
	}

	sample, ok := c.samples[uuid]
	if !ok {
		return nil, nil // Device does not support GPM metrics, don't collect anything
	}

	metricsGet, err := sample.getMetrics(c.lib, dev)
	if err != nil {
		return nil, fmt.Errorf("failed to get GPM metrics for device %s: %w", uuid, err)
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
		sampleErr := sample.release()
		if sampleErr != nvml.SUCCESS {
			err = multierror.Append(err, sampleErr)
		}
	}

	return err
}

type deviceSamples struct {
	numSamples         int
	samples            []nvml.GpmSample
	hasPreviousSample  bool
	currentSampleIndex int
}

func newDeviceSamples(lib nvml.Interface, numSamples int) (deviceSamples, error) {
	smpl := deviceSamples{
		numSamples: numSamples,
		samples:    make([]nvml.GpmSample, numSamples),
	}

	for i := 0; i < smpl.numSamples; i++ {
		sample, ret := lib.GpmSampleAlloc()
		if ret != nvml.SUCCESS {
			return smpl, fmt.Errorf("failed to allocate GPM sample: %s", nvml.ErrorString(ret))
		}

		smpl.samples = append(smpl.samples, sample)
	}

	return smpl, nil
}

func (smpl *deviceSamples) currentSample() nvml.GpmSample {
	return smpl.samples[smpl.currentSampleIndex]
}

func (smpl *deviceSamples) previousSample() nvml.GpmSample {
	return smpl.samples[(smpl.currentSampleIndex+1)%smpl.numSamples]
}

func (smpl *deviceSamples) markCollectedSample() {
	smpl.hasPreviousSample = true
	smpl.currentSampleIndex = (smpl.currentSampleIndex + 1) % smpl.numSamples
}

func (smpl *deviceSamples) release() error {
	var err error
	for _, sample := range smpl.samples {
		ret := sample.Free()
		if ret != nvml.SUCCESS {
			err = multierror.Append(err, fmt.Errorf("failed to free GPM sample: %s", nvml.ErrorString(ret)))
		}
	}

	return err
}

func (smpl *deviceSamples) getMetrics(lib nvml.Interface, dev nvml.Device) (*nvml.GpmMetricsGetType, error) {
	ret := smpl.currentSample().Get(dev)
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get GPM sample: %s", nvml.ErrorString(ret))
	}
	defer smpl.markCollectedSample()

	var metricsGet *nvml.GpmMetricsGetType
	if smpl.hasPreviousSample {
		metricsGet = &nvml.GpmMetricsGetType{
			Version:    nvmlGpmMetricsGetVersion,
			Sample1:    smpl.previousSample(),
			Sample2:    smpl.currentSample(),
			NumMetrics: 0,
		}

		// Fixed size array in the NVML API, so we need to control the number of
		// metrics we request This is a sanity check, we are not requesting
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

		ret := lib.GpmMetricsGet(metricsGet)
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
