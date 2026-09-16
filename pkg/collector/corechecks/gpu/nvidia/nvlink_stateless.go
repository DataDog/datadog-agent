// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type nvlinkAPICallInfo struct {
	Name    string                                                     // Name of the API call for logging/debugging
	Handler func(ddnvml.Device, uint64, int) ([]Sample, uint64, error) // Function to handle the API call for a port and return samples (samples, newTimestamp, error)
}

func nvlinkErrorCounterSample(device ddnvml.Device, metricName string, counter nvml.NvLinkErrorCounter, port int) ([]Sample, uint64, error) {
	if device.GetDeviceInfo().Architecture < nvml.DEVICE_ARCH_HOPPER {
		return nil, 0, ddnvml.NewNvmlAPIErrorOrNil("GetNvLinkErrorCounter", nvml.ERROR_NOT_SUPPORTED)
	}

	link := port - 1
	value, err := device.GetNvLinkErrorCounter(link, counter)
	if err != nil {
		return nil, 0, fmt.Errorf("get %s for port %d: %w", metricName, port, err)
	}

	samples := []Sample{&Metric{
		baseSample: baseSample{priority: Medium, tags: []string{nvlinkPortTag(port)}},
		Name:       metricName,
		Value:      float64(value),
		Type:       metrics.GaugeType,
	}}

	return samples, 0, nil
}

// createNVLinkStatelessAPIs creates the API calls for the NVLink stateless collector, spawning multiple simple collectors
// for each API call and port.
func createNVLinkStatelessAPIs(device ddnvml.Device) []apiCallInfo {
	nvlinkAPICalls := []nvlinkAPICallInfo{
		{
			Name: "nvlink_error_dl_replay",
			Handler: func(device ddnvml.Device, _ uint64, port int) ([]Sample, uint64, error) {
				return nvlinkErrorCounterSample(device, "nvlink.errors.replay", nvml.NVLINK_ERROR_DL_REPLAY, port)
			},
		},
		{
			Name: "nvlink_error_dl_recovery",
			Handler: func(device ddnvml.Device, _ uint64, port int) ([]Sample, uint64, error) {
				return nvlinkErrorCounterSample(device, "nvlink.errors.recovery", nvml.NVLINK_ERROR_DL_RECOVERY, port)
			},
		},
		{
			Name: "nvlink_error_dl_crc_flit",
			Handler: func(device ddnvml.Device, _ uint64, port int) ([]Sample, uint64, error) {
				return nvlinkErrorCounterSample(device, "nvlink.errors.crc.flit", nvml.NVLINK_ERROR_DL_CRC_FLIT, port)
			},
		},
		{
			Name: "nvlink_error_dl_ecc",
			Handler: func(device ddnvml.Device, _ uint64, port int) ([]Sample, uint64, error) {
				return nvlinkErrorCounterSample(device, "nvlink.errors.ecc", nvml.NVLINK_ERROR_DL_ECC_DATA, port)
			},
		},
	}

	var apiCalls []apiCallInfo
	for _, nvlinkAPICall := range nvlinkAPICalls {
		ports, err := getSupportedNvlinkPorts(device, func(port int) ([]Sample, error) {
			samples, _, err := nvlinkAPICall.Handler(device, 0, port)
			return samples, err
		})
		if err != nil {
			log.Warnf("error getting supported nvlink ports for %s: %v", nvlinkAPICall.Name, err)

			// only skip ports if the error is because the API is unsupported
			if ddnvml.IsAPIUnsupportedOnDevice(err, device) {
				continue
			}
		}

		for _, port := range ports {
			apiCalls = append(apiCalls, apiCallInfo{
				Name: nvlinkAPICall.Name,
				Handler: func(device ddnvml.Device, _ uint64) ([]Sample, uint64, error) {
					return nvlinkAPICall.Handler(device, 0, port)
				},
			})
		}
	}

	return apiCalls
}

func newNVLinkStatelessCollector(device ddnvml.Device, _ *CollectorDependencies) (Collector, error) {
	return NewBaseCollector(nvlinkStateless, device, createNVLinkStatelessAPIs(device))
}
