// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	ddmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
)

// processSample handles the complex time-weighted averaging logic for NVML sample types
func processSample(device ddnvml.Device, metricName string, samplingType nvml.SamplingType, lastTimestamp uint64) ([]Metric, uint64, error) {
	// GetSamples returns a list of samples (timestamp + value) for the
	// given counter type (GPU utilization, memory activity, etc).
	// Note that timestamps are in microseconds always.
	// The values returned by GetSamples are of a gauge type, so
	// we need to average them.
	valueType, samples, err := device.GetSamples(samplingType, lastTimestamp)
	if err != nil {
		var nvmlErr *ddnvml.NvmlAPIError
		if errors.As(err, &nvmlErr) && errors.Is(nvmlErr.NvmlErrorCode, nvml.ERROR_NOT_FOUND) {
			// NOT_FOUND is a valid scenario when no samples are available, such as vGPU. Ignore it and treat it the same way as
			// if there were no samples returned.
			return nil, lastTimestamp, nil
		}

		return nil, 0, fmt.Errorf("failed to get samples for %s: %w", metricName, err)
	}

	if len(samples) == 0 {
		//no available samples
		return nil, lastTimestamp, nil
	}

	// We have to do a time-based average, as not all the samples are collected in the same period
	total := 0.0
	currentTimestamp := lastTimestamp
	var multiErr error

	// We're assuming "samples" is a sorted array by time. Here we traverse the list of samples
	// and compute the average over time, which means weighing each sample by the time
	// it passed from the last run of this collector.
	for _, s := range samples {
		if s.TimeStamp < currentTimestamp {
			// some samples have a timestamp of 0, which we take as
			// invalid/placeholder.
			// They can also have the same timestamp as
			// the previous one if the sample is the first one in the list
			// which means it refers to the utilization before
			// 'lastTimestamp', so ignore it
			continue
		}

		sampleInterval := s.TimeStamp - currentTimestamp

		var value float64
		value, err = fieldValueToNumber[float64](valueType, s.SampleValue)
		if err != nil {
			multiErr = multierror.Append(multiErr, fmt.Errorf("failed to convert sample value %s from %v with type %v: %w", metricName, s.SampleValue, valueType, err))
			continue
		}

		total += value * float64(sampleInterval)
		currentTimestamp = s.TimeStamp
	}

	if currentTimestamp == lastTimestamp {
		// no samples were collected in the period
		return nil, lastTimestamp, nil
	}

	// Divide by the length of the time interval to get the average since the last
	// time we computed these metrics.
	total /= float64(currentTimestamp - lastTimestamp)

	metric := Metric{
		Name:  metricName,
		Value: total,
		Type:  ddmetrics.GaugeType,
	}

	return []Metric{metric}, currentTimestamp, multiErr
}

// processUtilizationSample handles process utilization sampling logic
func processUtilizationSample(device ddnvml.Device, lastTimestamp uint64) ([]Metric, uint64, error) {
	currentTime := uint64(time.Now().UnixMicro())
	processSamples, err := device.GetProcessUtilization(lastTimestamp)

	var allMetrics []Metric
	var allWorkloadIDs []workloadmeta.EntityID
	var maxSmUtil, sumSmUtil uint32

	// Handle ERROR_NOT_FOUND as a valid scenario when no process utilization data is available
	if err != nil {
		var nvmlErr *ddnvml.NvmlAPIError
		if errors.As(err, &nvmlErr) && errors.Is(nvmlErr.NvmlErrorCode, nvml.ERROR_NOT_FOUND) {
			err = nil // Clear the error for NOT_FOUND case
		}
	} else {
		for _, sample := range processSamples {
			workloads := []workloadmeta.EntityID{{
				Kind: workloadmeta.KindProcess,
				ID:   strconv.Itoa(int(sample.Pid)),
			}}
			allWorkloadIDs = append(allWorkloadIDs, workloads...)

			allMetrics = append(allMetrics,
				Metric{Name: "process.sm_active", Value: float64(sample.SmUtil), Type: ddmetrics.GaugeType, AssociatedWorkloads: workloads, Priority: Medium}, // There's an ebpf based fallback for this metric which should have lower priority
				Metric{Name: "process.dram_active", Value: float64(sample.MemUtil), Type: ddmetrics.GaugeType, AssociatedWorkloads: workloads},
				Metric{Name: "process.encoder_active", Value: float64(sample.EncUtil), Type: ddmetrics.GaugeType, AssociatedWorkloads: workloads},
				Metric{Name: "process.decoder_active", Value: float64(sample.DecUtil), Type: ddmetrics.GaugeType, AssociatedWorkloads: workloads},
			)

			if sample.SmUtil > maxSmUtil {
				maxSmUtil = sample.SmUtil
			}
			sumSmUtil += sample.SmUtil
		}
	}

	// Device-wide sm_active metric
	if sumSmUtil > 100 {
		sumSmUtil = 100
	}
	deviceSmActive := float64(maxSmUtil+sumSmUtil) / 2.0

	allMetrics = append(allMetrics,
		Metric{Name: "sm_active", Value: deviceSmActive, Type: ddmetrics.GaugeType, Priority: Medium}, // There's an ebpf based fallback for this metric which should have lower priority
		Metric{Name: "core.limit", Value: float64(device.GetDeviceInfo().CoreCount), Type: ddmetrics.GaugeType, AssociatedWorkloads: allWorkloadIDs},
	)

	return allMetrics, currentTime, err
}

// createSampleAPIs creates API call definitions for all sampling metrics on demand
func createSampleAPIs() []apiCallInfo {
	return []apiCallInfo{
		// Process utilization APIs (sample - requires timestamp tracking)
		{
			Name: "process_utilization",
			Handler: func(device ddnvml.Device, lastTimestamp uint64) ([]Metric, uint64, error) {
				return processUtilizationSample(device, lastTimestamp)
			},
		},
		// Samples collector APIs - each sample type is separate for independent failure handling
		{
			Name: "gr_engine_samples",
			Handler: func(device ddnvml.Device, lastTimestamp uint64) ([]Metric, uint64, error) {
				return processSample(device, "gr_engine_active", nvml.GPU_UTILIZATION_SAMPLES, lastTimestamp)
			},
		},
		{
			Name: "dram_active_samples",
			Handler: func(device ddnvml.Device, lastTimestamp uint64) ([]Metric, uint64, error) {
				return processSample(device, "dram_active", nvml.MEMORY_UTILIZATION_SAMPLES, lastTimestamp)
			},
		},
		{
			Name: "encoder_samples",
			Handler: func(device ddnvml.Device, lastTimestamp uint64) ([]Metric, uint64, error) {
				return processSample(device, "encoder_active", nvml.ENC_UTILIZATION_SAMPLES, lastTimestamp)
			},
		},
		{
			Name: "decoder_samples",
			Handler: func(device ddnvml.Device, lastTimestamp uint64) ([]Metric, uint64, error) {
				return processSample(device, "decoder_active", nvml.DEC_UTILIZATION_SAMPLES, lastTimestamp)
			},
		}}
}

// newStatefulCollector creates a new baseCollector configured for sampling-based metrics.
// It initializes timestamps for sampling collectors like process and samples.
func newStatefulCollector(name CollectorName, device ddnvml.Device, apiCalls []apiCallInfo) (Collector, error) {
	c, err := newBaseCollector(name, device, apiCalls)
	if err != nil {
		return nil, err
	}

	// Initialize timestamps for sampling collectors
	currentTime := uint64(time.Now().UnixMicro())
	for _, apiCall := range c.supportedAPIs {
		c.lastTimestamps[apiCall.Name] = currentTime
	}

	return c, nil
}

// sampleAPIFactory allows overriding API creation for testing
var sampleAPIFactory = createSampleAPIs

// newSamplingCollector creates a collector that consolidates all sampling collector types
func newSamplingCollector(device ddnvml.Device, _ *CollectorDependencies) (Collector, error) {
	return newStatefulCollector(sampling, device, sampleAPIFactory())
}
