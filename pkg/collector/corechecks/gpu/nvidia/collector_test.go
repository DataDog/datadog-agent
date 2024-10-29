// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nvidia

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"
)

func getBasicNvmlDeviceMock() nvml.Device {
	return &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-123", nvml.SUCCESS
		},
		GetNameFunc: func() (string, nvml.Return) {
			return "Tesla UltraMegaPower", nvml.SUCCESS
		},
	}
}

// getBasicNvmlMock returns a mock of the nvml.Interface with a single device with 10 cores,
// useful for basic tests that need only the basic interaction with NVML to be working.
func getBasicNvmlMock() *nvmlmock.Interface {
	return &nvmlmock.Interface{
		DeviceGetCountFunc: func() (int, nvml.Return) {
			return 1, nvml.SUCCESS
		},
		DeviceGetHandleByIndexFunc: func(int) (nvml.Device, nvml.Return) {
			return getBasicNvmlDeviceMock(), nvml.SUCCESS
		},
	}
}

func TestCollectorsStillInitIfOneFails(t *testing.T) {
	succeedCollector := &mockCollector{}
	factorySucceeded := false

	// On the first call, this function returns correctly. On the second it fails.
	// We need this as we cannot rely on the order of the subsystems in the map.
	factory := func(_ nvml.Interface, _ nvml.Device, _ []string) (Collector, error) {
		if !factorySucceeded {
			factorySucceeded = true
			return succeedCollector, nil
		}
		return nil, errors.New("failure")
	}

	collectors, err := buildCollectors(getBasicNvmlMock(), map[string]subsystemBuilder{"ok": factory, "fail": factory})
	require.NotNil(t, collectors)
	require.NoError(t, err)
}

func TestGetTagsFromDeviceGetsTagsEvenIfOneFails(t *testing.T) {
	device := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "GPU-123", nvml.SUCCESS
		},
		GetNameFunc: func() (string, nvml.Return) {
			return "", nvml.ERROR_GPU_IS_LOST
		},
	}

	result := getTagsFromDevice(device)
	expected := []string{tagVendor, tagNameUUID + ":GPU-123"}
	require.ElementsMatch(t, expected, result)
}
