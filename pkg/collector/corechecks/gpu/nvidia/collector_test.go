// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nvidia

import (
	"errors"
	"testing"

	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"
)

// this mock returns proper values only for devices with index 0 and 1, otherwise it will return an error
func getBasicNvmlDeviceMock(index int) nvml.Device {
	return &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			switch index {
			case 0:
				return "GPU-123", nvml.SUCCESS
			case 1:
				return "GPU-456", nvml.SUCCESS
			default:
				return "", nvml.ERROR_UNKNOWN
			}
		},
		GetNameFunc: func() (string, nvml.Return) {
			switch index {
			case 0:
				return "Tesla UltraMegaPower", nvml.SUCCESS
			case 1:
				return "H100", nvml.SUCCESS
			default:
				return "", nvml.ERROR_UNKNOWN
			}
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
		DeviceGetHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
			return getBasicNvmlDeviceMock(index), nvml.SUCCESS
		},
	}
}

func TestCollectorsStillInitIfOneFails(t *testing.T) {
	succeedCollector := &mockCollector{}
	factorySucceeded := false

	// On the first call, this function returns correctly. On the second it fails.
	// We need this as we cannot rely on the order of the subsystems in the map.
	factory := func(_ nvml.Device) (Collector, error) {
		if !factorySucceeded {
			factorySucceeded = true
			return succeedCollector, nil
		}
		return nil, errors.New("failure")
	}

	nvmlMock := getBasicNvmlMock()
	deps := &CollectorDependencies{NVML: nvmlMock}
	collectors, err := buildCollectors(deps, map[CollectorName]subsystemBuilder{"ok": factory, "fail": factory})
	require.NotNil(t, collectors)
	require.NoError(t, err)

}

func TestGetDeviceTagsMapping(t *testing.T) {
	tests := []struct {
		name      string
		mockSetup func() (*nvmlmock.Interface, taggermock.Mock)
		expected  func(t *testing.T, tagsMapping map[string][]string)
	}{
		{
			name: "Happy flow with 2 devices",
			mockSetup: func() (*nvmlmock.Interface, taggermock.Mock) {
				nvmlMock := &nvmlmock.Interface{
					DeviceGetCountFunc: func() (int, nvml.Return) {
						return 2, nvml.SUCCESS
					},
					DeviceGetHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
						return getBasicNvmlDeviceMock(index), nvml.SUCCESS
					},
				}
				fakeTagger := taggermock.SetupFakeTagger(t)
				fakeTagger.SetTags(types.NewEntityID(types.GPU, "GPU-123"), "foo", []string{"gpu_uuid=GPU-123", "gpu_vendor=nvidia", "gpu_arch=pascal"}, nil, nil, nil)
				fakeTagger.SetTags(types.NewEntityID(types.GPU, "GPU-456"), "foo", []string{"gpu_uuid=GPU-456", "gpu_vendor=nvidia", "gpu_arch=turing"}, nil, nil, nil)
				return nvmlMock, fakeTagger
			},
			expected: func(t *testing.T, tagsMapping map[string][]string) {
				require.Len(t, tagsMapping, 2)
				tags, ok := tagsMapping["GPU-123"]
				require.True(t, ok)
				require.ElementsMatch(t, tags, []string{"gpu_vendor=nvidia", "gpu_arch=pascal", "gpu_uuid=GPU-123"})

				tags, ok = tagsMapping["GPU-456"]
				require.True(t, ok)
				require.ElementsMatch(t, tags, []string{"gpu_vendor=nvidia", "gpu_arch=turing", "gpu_uuid=GPU-456"})
			},
		},
		{
			name: "No available devices",
			mockSetup: func() (*nvmlmock.Interface, taggermock.Mock) {
				nvmlMock := &nvmlmock.Interface{
					DeviceGetCountFunc: func() (int, nvml.Return) {
						return 0, nvml.ERROR_UNKNOWN
					},
				}
				fakeTagger := taggermock.SetupFakeTagger(t)
				return nvmlMock, fakeTagger
			},
			expected: func(t *testing.T, tagsMapping map[string][]string) {
				require.Nil(t, tagsMapping)
			},
		},
		{
			name: "Only one device successfully retrieved",
			mockSetup: func() (*nvmlmock.Interface, taggermock.Mock) {
				nvmlMock := &nvmlmock.Interface{
					DeviceGetCountFunc: func() (int, nvml.Return) {
						return 2, nvml.SUCCESS
					},
					DeviceGetHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
						// off by 1 on index will return error for device with index==1 and succeed for the device with index==0
						return getBasicNvmlDeviceMock(index + 1), nvml.SUCCESS
					},
				}
				fakeTagger := taggermock.SetupFakeTagger(t)
				fakeTagger.SetTags(types.NewEntityID(types.GPU, "GPU-456"), "foo", []string{"gpu_vendor=nvidia", "gpu_arch=pascal", "gpu_uuid=GPU-456"}, nil, nil, nil)
				return nvmlMock, fakeTagger
			},
			expected: func(t *testing.T, tagsMapping map[string][]string) {
				require.Len(t, tagsMapping, 1)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			nvmlMock, fakeTagger := tc.mockSetup()

			// Execute
			tagsMapping := GetDeviceTagsMapping(nvmlMock, fakeTagger)

			// Assert
			tc.expected(t, tagsMapping)
		})
	}
}
