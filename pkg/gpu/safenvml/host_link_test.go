// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// c2cDevice overrides the C2C mode query, whose versioned handler the generated
// mock cannot construct.
type c2cDevice struct {
	SafeDevice
	info nvml.C2cModeInfo_v1
	err  error
}

func (d *c2cDevice) GetC2cModeInfo() (nvml.C2cModeInfo_v1, error) { return d.info, d.err }

func TestQueryHostLinkC2C(t *testing.T) {
	gb200 := testutil.WithCustomHook(func(d *testutil.MockDevice) {
		d.GetMaxPcieLinkGenerationFunc = func() (int, nvml.Return) { return 6, nvml.SUCCESS }
		d.GetGpuMaxPcieLinkGenerationFunc = func() (int, nvml.Return) { return 6, nvml.SUCCESS }
	})
	bandwidth := map[uint32]map[uint32]testutil.MockFieldValue{nvml.FI_DEV_C2C_LINK_GET_MAX_BW: {
		0: testutil.NewFieldValue(44712), 1: testutil.FieldError(nvml.ERROR_NOT_SUPPORTED), 2: testutil.NewFieldValue(44800),
	}}
	enabled, disabled := true, false
	for _, tc := range []struct {
		name    string
		options []testutil.NvmlDeviceOption
		info    nvml.C2cModeInfo_v1
		err     error
		want    HostLinkInfo
	}{
		{
			name:    "enabled with links",
			options: []testutil.NvmlDeviceOption{gb200, testutil.WithC2CLinkCount(3), testutil.WithScopedFieldValues(bandwidth)},
			info:    nvml.C2cModeInfo_v1{IsC2cEnabled: 1},
			want:    HostLinkInfo{C2CEnabled: &enabled, C2CLinkCount: 3, C2CMaxBandwidthMBps: 44800, PCIeMaxGeneration: 6, PCIeGPUMaxGeneration: 6, PCIeMaxWidth: 16},
		},
		{
			name:    "enabled without link count",
			options: []testutil.NvmlDeviceOption{gb200, testutil.WithUnsupportedFields(nvml.FI_DEV_C2C_LINK_COUNT)},
			info:    nvml.C2cModeInfo_v1{IsC2cEnabled: 1},
			want:    HostLinkInfo{C2CEnabled: &enabled, PCIeMaxGeneration: 6, PCIeGPUMaxGeneration: 6, PCIeMaxWidth: 16},
		},
		{
			name:    "reported disabled",
			options: []testutil.NvmlDeviceOption{testutil.WithC2CLinkCount(3)},
			info:    nvml.C2cModeInfo_v1{IsC2cEnabled: 0},
			want:    HostLinkInfo{C2CEnabled: &disabled, PCIeMaxGeneration: 4, PCIeGPUMaxGeneration: 4, PCIeMaxWidth: 16},
		},
		{
			name: "unsupported on the device",
			err:  NewNvmlAPIErrorOrNil("GetC2cModeInfoV", nvml.ERROR_NOT_SUPPORTED),
			want: HostLinkInfo{C2CEnabled: &disabled, PCIeMaxGeneration: 4, PCIeGPUMaxGeneration: 4, PCIeMaxWidth: 16},
		},
		{
			name: "query failure is unknown",
			err:  NewNvmlAPIErrorOrNil("GetC2cModeInfoV", nvml.ERROR_UNKNOWN),
			want: HostLinkInfo{PCIeMaxGeneration: 4, PCIeGPUMaxGeneration: 4, PCIeMaxWidth: 16},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockNvml := testutil.NewMockNVML(testutil.WithDeviceOptions(0, tc.options...))
			WithMockNVML(t, mockNvml)
			physical, err := NewPhysicalDevice(mockNvml.Device(0))
			require.NoError(t, err)
			device := &c2cDevice{SafeDevice: physical.SafeDevice, info: tc.info, err: tc.err}
			require.Equal(t, tc.want, physical.DeviceInfo.queryHostLink(device))
		})
	}
}

func TestGetC2cModeInfo(t *testing.T) {
	info, err := getC2cModeInfo(func() (nvml.C2cModeInfo_v1, nvml.Return) {
		return nvml.C2cModeInfo_v1{IsC2cEnabled: 1}, nvml.SUCCESS
	})
	require.NoError(t, err)
	require.Equal(t, uint32(1), info.IsC2cEnabled)

	info, err = getC2cModeInfo(func() (nvml.C2cModeInfo_v1, nvml.Return) {
		return nvml.C2cModeInfo_v1{IsC2cEnabled: 1}, nvml.ERROR_NOT_SUPPORTED
	})
	require.True(t, IsUnsupported(err))
	require.Equal(t, nvml.C2cModeInfo_v1{}, info)
}

func TestGetC2cModeInfoRequiresVersionedAPISymbol(t *testing.T) {
	mockNvml := testutil.NewMockNVML()
	WithMockNVML(t, mockNvml)
	device, err := NewPhysicalDevice(mockNvml.Device(0))
	require.NoError(t, err)
	_, err = device.GetC2cModeInfo()
	require.True(t, IsUnsupported(err))
}
