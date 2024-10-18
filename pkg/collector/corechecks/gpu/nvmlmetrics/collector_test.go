// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nvmlmetrics

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// GetBasicNvmlMock returns a mock of the nvml.Interface with a single device with 10 cores,
// useful for basic tests that need only the basic interaction with NVML to be working.
func getBasicNvmlMock() *nvmlmock.Interface {
	return &nvmlmock.Interface{
		DeviceGetCountFunc: func() (int, nvml.Return) {
			return 1, nvml.SUCCESS
		},
		DeviceGetHandleByIndexFunc: func(int) (nvml.Device, nvml.Return) {
			return &nvmlmock.Device{
				GetUUIDFunc: func() (string, nvml.Return) {
					return "GPU-123", nvml.SUCCESS
				},
				GetNameFunc: func() (string, nvml.Return) {
					return "Tesla UltraMegaPower", nvml.SUCCESS
				},
			}, nvml.SUCCESS
		},
	}
}

func TestCollectorsGetClosedIfInitFails(t *testing.T) {
	succeedCollector := &mockSubsystemCollector{}
	factorySucceeded := false

	// On the first call, this function returns correctly. On the second it fails.
	// We need this as we cannot rely on the order of the subsystems in the map.
	factory := func(lib nvml.Interface, devices []nvml.Device) (subsystemCollector, error) {
		if !factorySucceeded {
			factorySucceeded = true
			return succeedCollector, nil
		}
		return nil, errors.New("failure")
	}

	succeedCollector.EXPECT().close().Return(nil)

	collector, err := newCollectorWithSubsystems(getBasicNvmlMock(), map[string]subsystemFactory{"ok": factory, "fail": factory})
	require.Nil(t, collector)
	require.Error(t, err)
}

func TestCollectorsCollectMetricsEvenInCaseOfFailure(t *testing.T) {
	dummy := &mockSubsystemCollector{}
	factory := func(lib nvml.Interface, devices []nvml.Device) (subsystemCollector, error) {
		return dummy, nil
	}

	collector, err := newCollectorWithSubsystems(getBasicNvmlMock(), map[string]subsystemFactory{"one": factory, "two": factory})
	require.NotNil(t, collector)
	require.NoError(t, err)

	// change the collectors so that they're executed in the order we want
	succeedCollector := &mockSubsystemCollector{}
	failCollector := &mockSubsystemCollector{}
	collector.collectors = []subsystemCollector{succeedCollector, failCollector}

	succeedCollector.EXPECT().collectMetrics(mock.Anything).Return([]Metric{{Name: "succeed"}}, nil)
	succeedCollector.EXPECT().name().Return("succeed").Maybe()
	failCollector.EXPECT().collectMetrics(mock.Anything).Return(nil, errors.New("failure"))
	failCollector.EXPECT().name().Return("fail").Maybe()

	metrics, err := collector.Collect()
	require.Error(t, err)
	require.Len(t, metrics, 1)
	require.Equal(t, "succeed", metrics[0].Name)
	require.NotEmpty(t, metrics[0].Tags)
}
