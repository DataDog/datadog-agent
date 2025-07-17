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

type nvlinkCollector struct {
	device       ddnvml.SafeDevice
	totalNVLinks int
}

func newNVLinkCollector(device ddnvml.SafeDevice) (Collector, error) {
	fields := []nvml.FieldValue{
		{
			FieldId: nvml.FI_DEV_NVLINK_LINK_COUNT,
			ScopeId: 0,
		},
	}

	if err := device.GetFieldValues(fields); err != nil {
		return nil, fmt.Errorf("%w : %w", errUnsupportedDevice, err)
	}

	linksCount, convErr := fieldValueToNumber[int](nvml.ValueType(fields[0].ValueType), fields[0].Value)
	if convErr != nil {
		return nil, fmt.Errorf("failed to convert number of nvlinks to integer: %s", convErr)
	}

	return &nvlinkCollector{
		device:       device,
		totalNVLinks: linksCount,
	}, nil
}

func (c *nvlinkCollector) DeviceUUID() string {
	uuid, _ := c.device.GetUUID()
	return uuid
}

func (c *nvlinkCollector) Name() CollectorName {
	return nvlink
}

func (c *nvlinkCollector) Collect() ([]Metric, error) {
	var multiErr error

	active, inactive := 0, 0

	// iterate over all existing nvlinks for the device
	for i := 0; i < c.totalNVLinks; i++ {
		state, err := c.device.GetNvLinkState(i)
		if err != nil {
			multiErr = multierror.Append(multiErr, fmt.Errorf("failed to get NVLink state for link %d: %w", i, err))
			continue
		}

		// Count active and inactive links
		if state == nvml.FEATURE_ENABLED {
			active++
		} else if state == nvml.FEATURE_DISABLED {
			inactive++
		}
	}
	//TODO: Once we start supporting metrics per nvlink, we should change the metrics to the following format:
	// "nvlink.[link_index].* where link_index is the index of the nvlink
	// and * represents all different metrics that will be gathered under the relevant nvlink
	// (e.g: capability, state, bandwidth mode, version, error counters, throughput, speed, etc...)
	allMetrics := [3]Metric{
		{
			Name:  "nvlink.count.total",
			Value: float64(c.totalNVLinks),
			Type:  metrics.GaugeType,
		},
		{
			Name:  "nvlink.count.active",
			Value: float64(active),
			Type:  metrics.GaugeType,
		},
		{
			Name:  "nvlink.count.inactive",
			Value: float64(inactive),
			Type:  metrics.GaugeType,
		},
	}

	return allMetrics[:], multiErr
}
