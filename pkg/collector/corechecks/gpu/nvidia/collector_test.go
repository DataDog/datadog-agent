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

	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/nvml"
	testutil "github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

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

	nvmlMock := testutil.GetBasicNvmlMock()
	deviceCache, err := ddnvml.NewDeviceCacheWithOptions(nvmlMock)
	require.NoError(t, err)
	deps := &CollectorDependencies{NVML: nvmlMock, DeviceCache: deviceCache}
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
						return testutil.GetDeviceMock(index), nvml.SUCCESS
					},
				}
				fakeTagger := taggermock.SetupFakeTagger(t)
				fakeTagger.SetTags(types.NewEntityID(types.GPU, testutil.GPUUUIDs[0]), "foo", []string{"gpu_uuid=GPU-123", "gpu_vendor=nvidia", "gpu_arch=pascal"}, nil, nil, nil)
				fakeTagger.SetTags(types.NewEntityID(types.GPU, testutil.GPUUUIDs[1]), "foo", []string{"gpu_uuid=GPU-456", "gpu_vendor=nvidia", "gpu_arch=turing"}, nil, nil, nil)
				return nvmlMock, fakeTagger
			},
			expected: func(t *testing.T, tagsMapping map[string][]string) {
				require.Len(t, tagsMapping, 2)
				tags, ok := tagsMapping[testutil.GPUUUIDs[0]]
				require.True(t, ok)
				require.ElementsMatch(t, tags, []string{"gpu_vendor=nvidia", "gpu_arch=pascal", "gpu_uuid=GPU-123"})

				tags, ok = tagsMapping[testutil.GPUUUIDs[1]]
				require.True(t, ok)
				require.ElementsMatch(t, tags, []string{"gpu_vendor=nvidia", "gpu_arch=turing", "gpu_uuid=GPU-456"})
			},
		},
		{
			name: "No available devices",
			mockSetup: func() (*nvmlmock.Interface, taggermock.Mock) {
				nvmlMock := &nvmlmock.Interface{
					DeviceGetCountFunc: func() (int, nvml.Return) {
						return 0, nvml.SUCCESS
					},
				}
				fakeTagger := taggermock.SetupFakeTagger(t)
				return nvmlMock, fakeTagger
			},
			expected: func(t *testing.T, tagsMapping map[string][]string) {
				require.Empty(t, tagsMapping)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			nvmlMock, fakeTagger := tc.mockSetup()

			// Execute
			deviceCache, err := ddnvml.NewDeviceCacheWithOptions(nvmlMock)
			require.NoError(t, err)
			tagsMapping := GetDeviceTagsMapping(deviceCache, fakeTagger)

			// Assert
			tc.expected(t, tagsMapping)
		})
	}
}
