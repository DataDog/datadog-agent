// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"errors"
	"maps"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestNewDevice(t *testing.T) {
	// Create mock with all symbols available
	mockNvml := testutil.NewMockNVML(
		testutil.WithSymbolsMock(allSymbols),
	)

	// Use WithMockNVML to set the mock
	WithMockNVML(t, mockNvml)

	// Test device creation
	mockDevice := mockNvml.Device(0)
	device, err := NewPhysicalDevice(mockDevice)

	// Verify results
	require.NoError(t, err)
	require.NotNil(t, device)
	require.Equal(t, testutil.GPUUUIDs[0], device.UUID)
	require.Equal(t, testutil.GPUCores[0], device.CoreCount)
	require.Equal(t, 0, device.Index)
	require.Equal(t, testutil.DefaultTotalMemory, device.Memory)
	require.Equal(t, uint32(75), device.SMVersion) // 7*10 + 5
}

func TestNewDeviceNVLinkLinkCount(t *testing.T) {
	tests := []struct {
		name            string
		options         []testutil.NvmlMockOption
		expectedCount   int
		expectedVersion string
	}{
		{
			name: "links present",
			options: []testutil.NvmlMockOption{
				testutil.WithCapabilities(testutil.Capabilities{NvLinkGenerationSupported: 1, NvLinkLinkCount: 2}),
			},
			expectedCount:   2,
			expectedVersion: "1.0",
		},
		{
			name: "disabled link",
			options: []testutil.NvmlMockOption{
				testutil.WithCapabilities(testutil.Capabilities{NvLinkGenerationSupported: 1}),
				testutil.WithNVLinkStates([]nvml.EnableState{nvml.FEATURE_DISABLED}, nil),
			},
			expectedCount:   1,
			expectedVersion: "1.0",
		},
		{
			name:          "no links",
			options:       []testutil.NvmlMockOption{testutil.WithNVLinkLinkCount(0)},
			expectedCount: 0,
		},
		{
			name:          "unsupported link count field",
			options:       []testutil.NvmlMockOption{testutil.WithUnsupportedFields(nvml.FI_DEV_NVLINK_LINK_COUNT)},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockNvml := testutil.NewMockNVML(
				append(tt.options, testutil.WithSymbolsMock(allSymbols))...,
			)
			WithMockNVML(t, mockNvml)

			nvmlDev, ret := mockNvml.DeviceGetHandleByIndex(0)
			require.Equal(t, nvml.SUCCESS, ret)

			device, err := NewPhysicalDevice(nvmlDev)

			require.NoError(t, err)
			require.Equal(t, tt.expectedCount, device.NVLinkLinkCount)
			require.Equal(t, tt.expectedVersion, device.NVLinkVersion)
		})
	}
}

func TestNewDeviceUUIDFailure(t *testing.T) {
	// Create mock with all symbols available
	mockNvml := testutil.NewMockNVML(
		testutil.WithSymbolsMock(allSymbols),
		testutil.WithDeviceOptions(0, testutil.WithCustomHook(func(device *testutil.MockDevice) {
			device.GetUUIDFunc = func() (string, nvml.Return) {
				return "", nvml.ERROR_INVALID_ARGUMENT
			}
		})),
	)

	// Use WithMockNVML to set the mock
	WithMockNVML(t, mockNvml)

	// Test device creation with failing UUID
	device, err := NewPhysicalDevice(mockNvml.Device(0))

	// Verify failure
	require.Error(t, err)
	require.Nil(t, device)

	// Check that it's the correct type of error using errors.As
	var nvmlErr *NvmlAPIError
	require.True(t, errors.As(err, &nvmlErr), "Expected error to be of type *NvmlAPIError")
	require.Equal(t, "GetUUID", nvmlErr.APIName)
	require.Equal(t, nvml.ERROR_INVALID_ARGUMENT, nvmlErr.NvmlErrorCode)
}

func TestDeviceWithMissingSymbol(t *testing.T) {
	// Create mock with MaxClockInfo symbol missing, not critical, should succeed
	symbols := maps.Clone(allSymbols)
	delete(symbols, toNativeName("GetMaxClockInfo"))

	mockNvml := testutil.NewMockNVML(
		testutil.WithSymbolsMock(symbols),
	)

	// Use WithMockNVML to set the mock
	WithPartialMockNVML(t, mockNvml, symbols)

	// Create device
	mockDevice := mockNvml.Device(0)
	device, err := NewPhysicalDevice(mockDevice)
	require.NoError(t, err)
	require.NotNil(t, device)

	// Expect the cache fields to be populated correctly
	require.Equal(t, testutil.GPUUUIDs[0], device.UUID)

	// Test calling a method with a missing symbol
	_, err = device.GetMaxClockInfo(nvml.CLOCK_MEM)
	require.Error(t, err)

	// Check that it's the correct type of error using errors.As
	var nvmlErr *NvmlAPIError
	require.True(t, errors.As(err, &nvmlErr), "Expected error to be of type *NvmlAPIError")
	require.Equal(t, toNativeName("GetMaxClockInfo"), nvmlErr.APIName)
	require.Equal(t, nvml.ERROR_FUNCTION_NOT_FOUND, nvmlErr.NvmlErrorCode)
}

func TestDeviceSafeMethodSuccess(t *testing.T) {
	// Create mock with all symbols available
	mockNvml := testutil.NewMockNVML(
		testutil.WithSymbolsMock(allSymbols),
	)

	// Use WithMockNVML to set the mock
	WithMockNVML(t, mockNvml)

	// Create device
	mockDevice := mockNvml.Device(0)
	device, err := NewPhysicalDevice(mockDevice)
	require.NoError(t, err)
	require.NotNil(t, device)

	// Test a method that calls the underlying NVML device
	memInfo, err := device.GetMemoryInfo()
	require.NoError(t, err)
	require.Equal(t, testutil.DefaultTotalMemory, memInfo.Total)

	// Test the embedded interface delegation
	cores, err := device.GetNumGpuCores()
	require.NoError(t, err)
	require.Equal(t, testutil.DefaultGpuCores, cores)
}

func TestGetGpuFabricInfoRequiresVersionedAPISymbol(t *testing.T) {
	symbols := maps.Clone(allSymbols)
	delete(symbols, toNativeName("GetGpuFabricInfoV"))

	device := &safeDeviceImpl{
		lib: &safeNvml{capabilities: symbols},
	}

	_, err := device.GetGpuFabricInfo()
	require.Error(t, err)
	require.True(t, IsUnsupported(err))
}

func TestGetMIGInstanceProfileNameRequiresGpuInstanceSymbols(t *testing.T) {
	for _, symbol := range []string{
		toNativeName("GetGpuInstanceById"),
		"nvmlGpuInstanceGetInfo",
		toNativeName("GetGpuInstanceProfileInfoByIdV"),
	} {
		t.Run(symbol, func(t *testing.T) {
			symbols := maps.Clone(allSymbols)
			delete(symbols, symbol)

			device := &safeDeviceImpl{
				lib: &safeNvml{capabilities: symbols},
			}

			_, err := device.GetMIGInstanceProfileName(3)
			require.Error(t, err)
			require.True(t, IsUnsupported(err))
		})
	}
}

func TestGetMIGInstanceProfileName(t *testing.T) {
	for _, tt := range []struct {
		name                  string
		driverName            string
		instanceRet           nvml.Return
		instanceInfoRet       nvml.Return
		profileInfoRet        nvml.Return
		want                  string
		wantErrorAPI          string
		wantErrorCode         nvml.Return
		wantInstanceInfoCalls int
		wantProfileInfoCalls  int
	}{
		{
			name:                  "resolves profile ID and strips MIG prefix",
			driverName:            "MIG 1g.35gb",
			want:                  "1g.35gb",
			wantInstanceInfoCalls: 1,
			wantProfileInfoCalls:  1,
		},
		{
			name:                  "preserves media extension",
			driverName:            "MIG 1g.35gb+me",
			want:                  "1g.35gb+me",
			wantInstanceInfoCalls: 1,
			wantProfileInfoCalls:  1,
		},
		{
			name:                  "empty profile name",
			wantInstanceInfoCalls: 1,
			wantProfileInfoCalls:  1,
		},
		{
			name:          "GPU instance lookup error",
			instanceRet:   nvml.ERROR_NOT_FOUND,
			wantErrorAPI:  "GetGpuInstanceById",
			wantErrorCode: nvml.ERROR_NOT_FOUND,
		},
		{
			name:                  "GPU instance info error",
			instanceInfoRet:       nvml.ERROR_GPU_IS_LOST,
			wantErrorAPI:          "GpuInstanceGetInfo",
			wantErrorCode:         nvml.ERROR_GPU_IS_LOST,
			wantInstanceInfoCalls: 1,
		},
		{
			name:                  "profile info unsupported",
			profileInfoRet:        nvml.ERROR_NOT_SUPPORTED,
			wantErrorAPI:          "GetGpuInstanceProfileInfoByIdV",
			wantErrorCode:         nvml.ERROR_NOT_SUPPORTED,
			wantInstanceInfoCalls: 1,
			wantProfileInfoCalls:  1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gpuInstance := &nvmlmock.GpuInstance{
				GetInfoFunc: func() (nvml.GpuInstanceInfo, nvml.Return) {
					return nvml.GpuInstanceInfo{Id: 3, ProfileId: 19}, tt.instanceInfoRet
				},
			}
			mockDevice := &nvmlmock.Device{
				GetGpuInstanceByIdFunc: func(id int) (nvml.GpuInstance, nvml.Return) {
					require.Equal(t, 3, id)
					if tt.instanceRet != nvml.SUCCESS {
						return nil, tt.instanceRet
					}
					return gpuInstance, nvml.SUCCESS
				},
			}
			device := &safeDeviceImpl{
				nvmlDevice: mockDevice,
				lib:        &safeNvml{capabilities: maps.Clone(allSymbols)},
			}
			profileInfoCalls := 0
			name, err := device.getMIGInstanceProfileName(3, func(profileID int) (nvml.GpuInstanceProfileInfo_v2, nvml.Return) {
				profileInfoCalls++
				// The profile ID is not the GPU instance ID.
				require.Equal(t, 19, profileID)
				var info nvml.GpuInstanceProfileInfo_v2
				copy(info.Name[:], int8s(tt.driverName))
				return info, tt.profileInfoRet
			})
			if tt.wantErrorAPI == "" {
				require.NoError(t, err)
			} else {
				var nvmlErr *NvmlAPIError
				require.ErrorAs(t, err, &nvmlErr)
				require.Equal(t, tt.wantErrorAPI, nvmlErr.APIName)
				require.Equal(t, tt.wantErrorCode, nvmlErr.NvmlErrorCode)
			}
			require.Equal(t, tt.want, name)
			require.Len(t, mockDevice.GetGpuInstanceByIdCalls(), 1)
			require.Len(t, gpuInstance.GetInfoCalls(), tt.wantInstanceInfoCalls)
			require.Equal(t, tt.wantProfileInfoCalls, profileInfoCalls)
		})
	}
}

func TestFixedSizeString(t *testing.T) {
	tests := []struct {
		name     string
		input    []int8
		expected string
	}{
		{
			name:     "plain profile name",
			input:    int8s("1g.35gb"),
			expected: "1g.35gb",
		},
		{
			name:     "media extension profile",
			input:    int8s("1g.35gb+me"),
			expected: "1g.35gb+me",
		},
		{
			name:     "NUL-terminated",
			input:    append(int8s("1g.10gb"), 0, 0, 0),
			expected: "1g.10gb",
		},
		{
			name:     "empty",
			input:    []int8{0, 0, 0},
			expected: "",
		},
		{
			name:     "trailing whitespace trimmed",
			input:    append(int8s(" 1g.20gb "), 0),
			expected: "1g.20gb",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, fixedSizeString(tt.input))
		})
	}
}

func TestParseMIGProfileFromDeviceName(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain profile", input: "NVIDIA H200 MIG 1g.35gb", want: "1g.35gb"},
		{name: "media extension profile", input: "NVIDIA H200 MIG 1g.18gb+me", want: "1g.18gb+me"},
		{name: "model with memory suffix", input: "NVIDIA A100-SXM4-40GB MIG 3g.20gb", want: "3g.20gb"},
		// Blackwell (RTX PRO 6000) profile names, as advertised in live
		// ResourceSlices.
		{name: "media engines excluded", input: "NVIDIA RTX PRO 6000 Blackwell Server Edition MIG 1g.24gb-me", want: "1g.24gb-me"},
		{name: "all media engines", input: "NVIDIA RTX PRO 6000 Blackwell Server Edition MIG 1g.24gb+me.all", want: "1g.24gb+me.all"},
		{name: "graphics", input: "NVIDIA RTX PRO 6000 Blackwell Server Edition MIG 4g.96gb+gfx", want: "4g.96gb+gfx"},
		// Split compute instances prefix the compute-slice count; the tag
		// describes the GPU instance.
		{name: "split compute instance", input: "NVIDIA A100-SXM4-40GB MIG 1c.3g.20gb", want: "3g.20gb"},
		{name: "split compute instance with suffix", input: "NVIDIA H100 80GB HBM3 MIG 2c.4g.40gb+me", want: "4g.40gb+me"},
		{name: "physical device has no profile", input: "NVIDIA H200", want: ""},
		{name: "MIG marker without profile", input: "NVIDIA H200 MIG", want: ""},
		{name: "unrecognized suffix", input: "NVIDIA H200 MIG unknown", want: ""},
		{name: "empty", input: "", want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ParseMIGProfileFromDeviceName(tt.input))
		})
	}
}

func TestNormalizeMIGProfileName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "driver format with MIG prefix",
			input:    "MIG 1g.35gb",
			expected: "1g.35gb",
		},
		{
			name:     "driver format with media extension",
			input:    "MIG 1g.35gb+me",
			expected: "1g.35gb+me",
		},
		{
			name:     "already canonical",
			input:    "1g.18gb",
			expected: "1g.18gb",
		},
		{
			name:     "padding trimmed",
			input:    "  MIG  1g.10gb  ",
			expected: "1g.10gb",
		},
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, normalizeMIGProfileName(tt.input))
		})
	}
}

func int8s(s string) []int8 {
	out := make([]int8, len(s))
	for i, c := range []byte(s) {
		out[i] = int8(c)
	}
	return out
}
