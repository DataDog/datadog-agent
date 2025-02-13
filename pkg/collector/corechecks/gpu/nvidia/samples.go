// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nvidia

import (
	"fmt"
	"slices"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/util/common"
)

const samplesCollectorName = "samples"

var allSamples = []sampleMetric{
	{"gr_engine_active", nvml.GPU_UTILIZATION_SAMPLES},
	{"dram.active", nvml.MEMORY_UTILIZATION_SAMPLES},
	{"encoder.active", nvml.ENC_UTILIZATION_SAMPLES},
	{"decoder.active", nvml.DEC_UTILIZATION_SAMPLES},
}

type sampleMetric struct {
	name         string
	samplingType nvml.SamplingType
}

type samplesCollector struct {
	device           nvml.Device
	tags             []string
	lastTimestamps   map[nvml.SamplingType]uint64
	samplesToCollect []sampleMetric
}

func newSamplesCollector(_ nvml.Interface, device nvml.Device, tags []string) (Collector, error) {
	c := &samplesCollector{
		device:         device,
		tags:           tags,
		lastTimestamps: make(map[nvml.SamplingType]uint64),
	}
	c.samplesToCollect = append(c.samplesToCollect, allSamples...) // copy all metrics to avoid modifying the original slice

	c.removeUnsupportedSamples()
	if len(c.samplesToCollect) == 0 {
		return nil, errUnsupportedDevice
	}

	return c, nil
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

func (c *samplesCollector) Close() error {
	return nil
}

func (samplesCollector) Name() string {
	return samplesCollectorName
}

// Collect collects all the metrics from the given NVML device.
func (c *samplesCollector) Collect() ([]Metric, error) {
	var err error

	values := make([]Metric, 0, len(allSamples)) // preallocate to reduce allocations
	for _, metric := range allSamples {
		prevTimestamp := c.lastTimestamps[metric.samplingType]
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

		// We're assuming "samples" is always sorted
		for _, sample := range samples {
			if sample.TimeStamp == 0 {
				// some samples have a timestamp of 0, which we take as invalid/placeholder
				continue
			}

			sampleInterval := sample.TimeStamp - lastTimestamp
			if sampleInterval == 0 {
				// this can happen if the sample is the first one in the list
				// which means it refers to the utilization before 'prevTimestamp',
				// so ignore it
				continue
			}

			value, err := metricValueToDouble(valueType, sample.SampleValue)
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

		total /= float64(lastTimestamp - prevTimestamp)
		c.lastTimestamps[metric.samplingType] = lastTimestamp

		values = append(values, Metric{
			Name:  metric.name,
			Value: total,
			Tags:  c.tags,
		})
	}

	return values, err
}
