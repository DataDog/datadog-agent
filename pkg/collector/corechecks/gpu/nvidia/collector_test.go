// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestCollectorsStillInitIfOneFails(t *testing.T) {
	succeedCollector := &mockCollector{}
	factorySucceeded := false

	// On the first call, this function returns correctly. On the second it fails.
	// We need this as we cannot rely on the order of the subsystems in the map.
	factory := func(_ ddnvml.SafeDevice) (Collector, error) {
		if !factorySucceeded {
			factorySucceeded = true
			return succeedCollector, nil
		}
		return nil, errors.New("failure")
	}

	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled())
	ddnvml.WithMockNVML(t, nvmlMock)
	deviceCache, err := ddnvml.NewDeviceCache()
	require.NoError(t, err)
	deps := &CollectorDependencies{DeviceCache: deviceCache}
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
				fakeTagger := taggerfxmock.SetupFakeTagger(t)
				fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.GPU, testutil.GPUUUIDs[0]), "foo", []string{"gpu_uuid=" + testutil.GPUUUIDs[0], "gpu_vendor=nvidia", "gpu_arch=pascal"}, nil, nil, nil)
				fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.GPU, testutil.GPUUUIDs[1]), "foo", []string{"gpu_uuid=" + testutil.GPUUUIDs[1], "gpu_vendor=nvidia", "gpu_arch=turing"}, nil, nil, nil)
				return nvmlMock, fakeTagger
			},
			expected: func(t *testing.T, tagsMapping map[string][]string) {
				require.Len(t, tagsMapping, 2)
				tags, ok := tagsMapping[testutil.GPUUUIDs[0]]
				require.True(t, ok)
				require.ElementsMatch(t, tags, []string{"gpu_vendor=nvidia", "gpu_arch=pascal", "gpu_uuid=" + testutil.GPUUUIDs[0]})

				tags, ok = tagsMapping[testutil.GPUUUIDs[1]]
				require.True(t, ok)
				require.ElementsMatch(t, tags, []string{"gpu_vendor=nvidia", "gpu_arch=turing", "gpu_uuid=" + testutil.GPUUUIDs[1]})
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
				fakeTagger := taggerfxmock.SetupFakeTagger(t)
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
						if index == 0 {
							return testutil.GetDeviceMock(index), nvml.SUCCESS
						}
						return nil, nvml.ERROR_INVALID_ARGUMENT
					},
				}
				fakeTagger := taggerfxmock.SetupFakeTagger(t)
				fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.GPU, testutil.GPUUUIDs[1]), "foo", []string{"gpu_vendor=nvidia", "gpu_arch=pascal", "gpu_uuid=" + testutil.GPUUUIDs[1]}, nil, nil, nil)
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
			ddnvml.WithMockNVML(t, nvmlMock)

			// Execute
			deviceCache, err := ddnvml.NewDeviceCache()
			require.NoError(t, err)
			tagsMapping := GetDeviceTagsMapping(deviceCache, fakeTagger)

			// Assert
			tc.expected(t, tagsMapping)
		})
	}
}

func TestAllCollectorsWork(t *testing.T) {
	// This test doesn't validate the results of the collectors, it only checks that they work with
	// the basic mock and we don't have any panics or anything.

	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled(), testutil.WithMockAllFunctions())
	ddnvml.WithMockNVML(t, nvmlMock)
	deviceCache, err := ddnvml.NewDeviceCache()
	require.NoError(t, err)
	deps := &CollectorDependencies{DeviceCache: deviceCache}
	collectors, err := BuildCollectors(deps)
	require.NoError(t, err)
	require.NotNil(t, collectors)

	seenCollectors := make(map[CollectorName]struct{})

	for _, collector := range collectors {
		result, err := collector.Collect()
		require.NoError(t, err, "collector %s failed to collect", collector.Name())
		require.NotEmpty(t, result, "collector %s returned empty result", collector.Name())
		seenCollectors[collector.Name()] = struct{}{}
	}

	// We should have seen all the collectors
	for name := range factory {
		_, ok := seenCollectors[name]
		require.True(t, ok, "collector %s not seen", name)
	}
}

func TestRemoveDuplicateMetrics(t *testing.T) {
	metrics := []Metric{
		{Name: "metric1", Priority: 0},
		{Name: "metric2", Priority: 1},
		{Name: "metric1", Priority: 2},
	}

	deduplicated := RemoveDuplicateMetrics(metrics)
	require.Len(t, deduplicated, 2)
	require.ElementsMatch(t, deduplicated, []Metric{
		{Name: "metric1", Priority: 2},
		{Name: "metric2", Priority: 1},
	})
}
