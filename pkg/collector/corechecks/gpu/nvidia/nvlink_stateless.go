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

type nvlinkErrorCounterMetric struct {
	name    string
	counter nvml.NvLinkErrorCounter
}

// nvlinkStatelessErrorCounters maps nvidia-smi nvlink -e/-ec counters to agent metrics on Hopper+.
var nvlinkStatelessErrorCounters = []nvlinkErrorCounterMetric{
	{name: "nvlink.errors.replay", counter: nvml.NVLINK_ERROR_DL_REPLAY},
	{name: "nvlink.errors.recovery", counter: nvml.NVLINK_ERROR_DL_RECOVERY},
	{name: "nvlink.errors.crc.flit", counter: nvml.NVLINK_ERROR_DL_CRC_FLIT},
	{name: "nvlink.errors.ecc", counter: nvml.NVLINK_ERROR_DL_ECC_DATA},
}

func nvlinkErrorCounterSample(device ddnvml.Device) ([]Sample, uint64, error) {
	if device.GetDeviceInfo().Architecture < nvml.DEVICE_ARCH_HOPPER {
		return nil, 0, ddnvml.NewNvmlAPIErrorOrNil("GetNvLinkErrorCounter", nvml.ERROR_NOT_SUPPORTED)
	}

	var samples []Sample
	var multiErr []error

	ports, err := getSupportedNvlinkPorts(device, func(port int) ([]Sample, error) {
		if _, probeErr := device.GetNvLinkErrorCounter(port-1, nvml.NVLINK_ERROR_DL_REPLAY); probeErr != nil {
			if ddnvml.IsAPIUnsupportedOnDevice(probeErr, device) {
				return nil, fmt.Errorf("%w: %w", errUnsupportedDevice, probeErr)
			}
			return nil, probeErr
		}
		return nil, nil
	})
	if len(ports) == 0 {
		if err != nil {
			return nil, 0, fmt.Errorf("get supported NVLink ports: %w", err)
		}
		return nil, 0, fmt.Errorf("%w: no supported NVLink ports found", errUnsupportedDevice)
	}

	for _, port := range ports {
		link := port - 1
		for _, metric := range nvlinkStatelessErrorCounters {
			value, counterErr := device.GetNvLinkErrorCounter(link, metric.counter)
			if counterErr != nil {
				if ddnvml.IsAPIUnsupportedOnDevice(counterErr, device) {
					return nil, 0, fmt.Errorf("%w: %w", errUnsupportedDevice, counterErr)
				}
				multiErr = append(multiErr, fmt.Errorf("get %s for port %d: %w", metric.name, port, counterErr))
				continue
			}

			samples = append(samples, &Metric{
				baseSample: baseSample{priority: Medium, tags: []string{nvlinkPortTag(port)}},
				Name:       metric.name,
				Value:      float64(value),
				Type:       metrics.GaugeType,
			})
		}
	}

	if len(samples) == 0 {
		return nil, 0, fmt.Errorf("%w: no NVLink error counter metrics collected", errUnsupportedDevice)
	}

	return samples, 0, errors.Join(multiErr...)
}

func createNVLinkStatelessAPIs() []apiCallInfo {
	return []apiCallInfo{
		{
			Name: "nvlink_error_counters",
			Handler: func(device ddnvml.Device, _ uint64) ([]Sample, uint64, error) {
				return nvlinkErrorCounterSample(device)
			},
		},
	}
}

var nvlinkStatelessAPIFactory = createNVLinkStatelessAPIs

func newNVLinkStatelessCollector(device ddnvml.Device, _ *CollectorDependencies) (Collector, error) {
	return NewBaseCollector(nvlinkStateless, device, nvlinkStatelessAPIFactory())
}
