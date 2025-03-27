// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"fmt"
	"slices"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/common"
)

var allSamples = []sampleMetric{
	{"gr_engine_active", nvml.GPU_UTILIZATION_SAMPLES, 0},
	{"dram_active", nvml.MEMORY_UTILIZATION_SAMPLES, 0},
	{"encoder_utilization", nvml.ENC_UTILIZATION_SAMPLES, 0},
	{"decoder_utilization", nvml.DEC_UTILIZATION_SAMPLES, 0},
}

type sampleMetric struct {
	name          string
	samplingType  nvml.SamplingType
	lastTimestamp uint64
}

type samplesCollector struct {
	device           nvml.Device
	samplesToCollect []sampleMetric
}

func newSamplesCollector(device nvml.Device) (Collector, error) {
	c := &samplesCollector{
		device: device,
	}
	c.samplesToCollect = append(c.samplesToCollect, allSamples...) // copy all metrics to avoid modifying the original slice

	c.removeUnsupportedSamples()
	if len(c.samplesToCollect) == 0 {
		return nil, errUnsupportedDevice
	}

	return c, nil
}

func (c *samplesCollector) DeviceUUID() string {
	uuid, _ := c.device.GetUUID()
	return uuid
}

func (c *samplesCollector) removeUnsupportedSamples() {
	metricsToRemove := common.StringSet{}
	for _, metric := range c.samplesToCollect {
		_, _, ret := c.device.GetSamples(metric.samplingType, 0)
		if ret == nvml.ERROR_NOT_SUPPORTED {
			metricsToRemove.Add(metric.name)
		}
	}

	for toRemove := range metricsToRemove {
		c.samplesToCollect = slices.DeleteFunc(c.samplesToCollect, func(m sampleMetric) bool {
			return m.name == toRemove
		})
	}
}

func (c *samplesCollector) Name() CollectorName {
	return samples
}

// Collect collects all the metrics from the given NVML device. This function
// calls the nvml GetSamples function, which returns a list of samples for each
// possible internal counter type. In this function we compute the average over
// time of those samples and report it as the metric for the current interval.
func (c *samplesCollector) Collect() ([]Metric, error) {
	var err error

	values := make([]Metric, 0, len(c.samplesToCollect)) // preallocate to reduce allocations
	for _, metric := range c.samplesToCollect {
		prevTimestamp := metric.lastTimestamp

		// GetSamples returns a list of samples (timestamp + value) for the
		// given counter type (GPU utilization, memory activity, etc).
		// Note that timestamps are in microseconds always.
		// The values returned by GetSamples are of a gauge type, so
		// we need to average them.
		valueType, samples, ret := c.device.GetSamples(metric.samplingType, prevTimestamp)
		if ret != nvml.SUCCESS {
			err = multierror.Append(err, fmt.Errorf("failed to get metric %s: %s", metric.name, nvml.ErrorString(ret)))
			continue
		}

		if len(samples) == 0 {
			err = multierror.Append(err, fmt.Errorf("no samples for metric %s", metric.name))
			continue
		}

		// We have to do a time-based average, as not all of the samples are collected in the same period
		total := 0.0
		lastTimestamp := prevTimestamp

		// We're assuming "samples" is always sorted. Here we traverse the list of samples
		// and compute the average over time, which means weighing each sample by the time
		// it passed since the last sample.
		for _, sample := range samples {
			if sample.TimeStamp < lastTimestamp {
				// some samples have a timestamp of 0, which we take as
				// invalid/placeholder.
				// They can also have the same timestamp as
				// the previous one if the sample is the first one in the list
				// which means it refers to the utilization before
				// 'prevTimestamp', so ignore it
				continue
			}

			sampleInterval := sample.TimeStamp - lastTimestamp

			var value float64
			value, err = metricValueToDouble(valueType, sample.SampleValue)
			if err != nil {
				err = multierror.Append(err, fmt.Errorf("failed to convert sample value %s from %v with type %v: %w", metric.name, sample.SampleValue, valueType, err))
				continue
			}

			total += value * float64(sampleInterval)
			lastTimestamp = sample.TimeStamp
		}

		if lastTimestamp == prevTimestamp {
			// no samples were collected in the period
			continue
		}

		// Divide by the length of the time interval to get the average since the last
		// time we computed these metrics.
		total /= float64(lastTimestamp - prevTimestamp)
		metric.lastTimestamp = lastTimestamp

		values = append(values, Metric{
			Name:  metric.name,
			Value: total,
			Type:  metrics.GaugeType,
		})
	}

	return values, err
}
