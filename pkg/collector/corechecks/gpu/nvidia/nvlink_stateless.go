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

func nvlinkErrorCounterSample(device ddnvml.Device, metricName string, counter nvml.NvLinkErrorCounter) ([]Sample, uint64, error) {
	if device.GetDeviceInfo().Architecture < nvml.DEVICE_ARCH_HOPPER {
		return nil, 0, ddnvml.NewNvmlAPIErrorOrNil("GetNvLinkErrorCounter", nvml.ERROR_NOT_SUPPORTED)
	}

	var samples []Sample
	var multiErr []error

	ports, err := getSupportedNvlinkPorts(device, func(port int) ([]Sample, error) {
		if _, probeErr := device.GetNvLinkErrorCounter(port-1, counter); probeErr != nil {
			if ddnvml.IsAPIUnsupportedOnDevice(probeErr, device) {
				return nil, fmt.Errorf("%w: %w", errUnsupportedDevice, probeErr)
			}
			return nil, probeErr
		}
		return nil, nil
	})

	if err != nil {
		return nil, 0, fmt.Errorf("get supported NVLink ports: %w", err)
	}

	for _, port := range ports {
		link := port - 1
		value, counterErr := device.GetNvLinkErrorCounter(link, counter)
		if counterErr != nil {
			if ddnvml.IsAPIUnsupportedOnDevice(counterErr, device) {
				return nil, 0, fmt.Errorf("%w: %w", errUnsupportedDevice, counterErr)
			}
			multiErr = append(multiErr, fmt.Errorf("get %s for port %d: %w", metricName, port, counterErr))
			continue
		}

		samples = append(samples, &Metric{
			baseSample: baseSample{priority: Medium, tags: []string{nvlinkPortTag(port)}},
			Name:       metricName,
			Value:      float64(value),
			Type:       metrics.GaugeType,
		})
	}

	if len(samples) == 0 {
		return nil, 0, fmt.Errorf("%w: no NVLink error counter metrics collected", errUnsupportedDevice)
	}

	return samples, 0, errors.Join(multiErr...)
}

func createNVLinkStatelessAPIs() []apiCallInfo {
	return []apiCallInfo{
		{
			Name: "nvlink_error_dl_replay",
			Handler: func(device ddnvml.Device, _ uint64) ([]Sample, uint64, error) {
				return nvlinkErrorCounterSample(device, "nvlink.errors.replay", nvml.NVLINK_ERROR_DL_REPLAY)
			},
		},
		{
			Name: "nvlink_error_dl_recovery",
			Handler: func(device ddnvml.Device, _ uint64) ([]Sample, uint64, error) {
				return nvlinkErrorCounterSample(device, "nvlink.errors.recovery", nvml.NVLINK_ERROR_DL_RECOVERY)
			},
		},
		{
			Name: "nvlink_error_dl_crc_flit",
			Handler: func(device ddnvml.Device, _ uint64) ([]Sample, uint64, error) {
				return nvlinkErrorCounterSample(device, "nvlink.errors.crc.flit", nvml.NVLINK_ERROR_DL_CRC_FLIT)
			},
		},
		{
			Name: "nvlink_error_dl_ecc",
			Handler: func(device ddnvml.Device, _ uint64) ([]Sample, uint64, error) {
				return nvlinkErrorCounterSample(device, "nvlink.errors.ecc", nvml.NVLINK_ERROR_DL_ECC_DATA)
			},
		},
	}
}

var nvlinkStatelessAPIFactory = createNVLinkStatelessAPIs

func newNVLinkStatelessCollector(device ddnvml.Device, _ *CollectorDependencies) (Collector, error) {
	return NewBaseCollector(nvlinkStateless, device, nvlinkStatelessAPIFactory())
}
