// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package cuda

import (
	"testing"

	"github.com/stretchr/testify/require"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
)

func TestGetVisibleDevices(t *testing.T) {
	commonPrefix := "GPU-89"
	uuid1 := commonPrefix + "32f937-d72c-4106-c12f-20bd9faed9f6"
	uuid2 := commonPrefix + "02f078-a8da-4036-a78f-4032bbddeaf2"

	dev1 := &ddnvml.PhysicalDevice{
		DeviceInfo: ddnvml.DeviceInfo{
			UUID: uuid1,
		},
	}

	dev2 := &ddnvml.PhysicalDevice{
		DeviceInfo: ddnvml.DeviceInfo{
			UUID: uuid2,
		},
	}

	devList := []ddnvml.Device{dev1, dev2}
	cases := []struct {
		name            string
		visibleDevices  string
		expectedDevices []ddnvml.Device
		expectsError    bool
	}{
		{
			name:            "no visible devices",
			visibleDevices:  "",
			expectedDevices: devList,
			expectsError:    false,
		},
		{
			name:            "UUIDs",
			visibleDevices:  uuid1,
			expectedDevices: []ddnvml.Device{devList[0]},
			expectsError:    false,
		},
		{
			name:            "Index",
			visibleDevices:  "1",
			expectedDevices: []ddnvml.Device{devList[1]},
			expectsError:    false,
		},
		{
			name:            "IndexOutOfRange",
			visibleDevices:  "2",
			expectedDevices: nil,
			expectsError:    true,
		},
		{
			name:            "InvalidIndex",
			visibleDevices:  "a",
			expectedDevices: nil,
			expectsError:    true,
		},
		{
			name:            "UnorderedIndexes",
			visibleDevices:  "1,0",
			expectedDevices: []ddnvml.Device{devList[1], devList[0]},
			expectsError:    false,
		},
		{
			name:            "MixedIndexesAndUUIDs",
			visibleDevices:  "0," + uuid2,
			expectedDevices: []ddnvml.Device{devList[0], devList[1]},
			expectsError:    false,
		},
		{
			name:            "InvalidIndexInMiddle",
			visibleDevices:  "0,235,1",
			expectedDevices: []ddnvml.Device{devList[0]},
			expectsError:    true,
		},
		{
			name:            "SharedPrefix",
			visibleDevices:  commonPrefix,
			expectedDevices: nil,
			expectsError:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			devices, err := ParseVisibleDevices(devList, tc.visibleDevices)
			if tc.expectsError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tc.expectedDevices, devices)
		})
	}
}

func TestGetVisibleDevicesWithMIG(t *testing.T) {
	// Create some sample devices
	gpuNoMig := &ddnvml.PhysicalDevice{
		DeviceInfo: ddnvml.DeviceInfo{
			UUID: "GPU-1234",
		},
	}

	gpuNoMig2 := &ddnvml.PhysicalDevice{
		DeviceInfo: ddnvml.DeviceInfo{
			UUID: "GPU-1235",
		},
	}

	gpuWithOneMigChild := &ddnvml.PhysicalDevice{
		DeviceInfo: ddnvml.DeviceInfo{
			UUID: "GPU-3456",
		},
		HasMIGFeatureEnabled: true,
		MIGChildren: []*ddnvml.MIGDevice{
			{
				DeviceInfo: ddnvml.DeviceInfo{
					UUID: "MIG-3456-1234-1234-1234",
				},
			},
		},
	}

	gpuWithTwoMigChildren := &ddnvml.PhysicalDevice{
		DeviceInfo: ddnvml.DeviceInfo{
			UUID: "GPU-7890",
		},
		HasMIGFeatureEnabled: true,
		MIGChildren: []*ddnvml.MIGDevice{
			{
				DeviceInfo: ddnvml.DeviceInfo{
					UUID: "MIG-7890-1234-1234-1234",
				},
			},
			{
				DeviceInfo: ddnvml.DeviceInfo{
					UUID: "MIG-7891-1234-1234-1234",
				},
			},
		},
	}

	gpuWithMigEnabledButNoChildren := &ddnvml.PhysicalDevice{
		DeviceInfo: ddnvml.DeviceInfo{
			UUID: "GPU-9999",
		},
		HasMIGFeatureEnabled: true,
	}

	// There's no clear documentation on how CUDA_VISIBLE_DEVICES should be interpreted when there are
	// MIG devices involved. These test cases have been derived from the behavior of a custom CUDA application
	// running on a system with:
	// - 8 NVIDIA A100 SXM4 40GB GPUs
	// - CUDA 12.4
	// - Driver version 550.127.05
	// The application was launched with different values of CUDA_VISIBLE_DEVICES, with different
	// combinations of MIG devices and non-MIG devices, and the observed behavior was recorded.
	cases := []struct {
		name            string
		systemDevices   []ddnvml.Device
		visibleDevices  string
		expectedDevices []ddnvml.Device
		expectsError    bool
	}{
		// Basic behavior for MIG: the GPU with MIG devices gets replaced by the first MIG child
		{
			name:            "only one device with one MIG child, no CUDA_VISIBLE_DEVICES",
			systemDevices:   []ddnvml.Device{gpuWithOneMigChild},
			visibleDevices:  "",
			expectedDevices: []ddnvml.Device{gpuWithOneMigChild.MIGChildren[0]},
		},
		{
			name:            "only one device with one MIG child, CUDA_VISIBLE_DEVICES=0",
			systemDevices:   []ddnvml.Device{gpuWithOneMigChild},
			visibleDevices:  "0",
			expectedDevices: []ddnvml.Device{gpuWithOneMigChild.MIGChildren[0]},
		},
		// If we select the parent GPU by UUID, and it has MIG children, the first MIG child gets selected
		{
			name:            "only one device with one MIG child, selected parent by UUID",
			systemDevices:   []ddnvml.Device{gpuWithOneMigChild},
			visibleDevices:  gpuWithOneMigChild.UUID,
			expectedDevices: []ddnvml.Device{gpuWithOneMigChild.MIGChildren[0]},
		},
		{
			name:            "only one device with one MIG child, selected MIG by UUID",
			systemDevices:   []ddnvml.Device{gpuWithOneMigChild},
			visibleDevices:  gpuWithOneMigChild.MIGChildren[0].UUID,
			expectedDevices: []ddnvml.Device{gpuWithOneMigChild.MIGChildren[0]},
		},
		{
			name:            "MIG with two children, no CUDA_VISIBLE_DEVICES",
			systemDevices:   []ddnvml.Device{gpuWithTwoMigChildren},
			visibleDevices:  "",
			expectedDevices: []ddnvml.Device{gpuWithTwoMigChildren.MIGChildren[0]},
		},
		{
			name:            "multiple MIG devices, no CUDA_VISIBLE_DEVICES",
			systemDevices:   []ddnvml.Device{gpuWithOneMigChild, gpuWithTwoMigChildren},
			visibleDevices:  "",
			expectedDevices: []ddnvml.Device{gpuWithOneMigChild.MIGChildren[0]},
		},
		{
			name:            "multiple MIG devices, CUDA_VISIBLE_DEVICES=0",
			systemDevices:   []ddnvml.Device{gpuWithOneMigChild, gpuWithTwoMigChildren},
			visibleDevices:  "0",
			expectedDevices: []ddnvml.Device{gpuWithOneMigChild.MIGChildren[0]},
		},
		{
			name:            "multiple MIG devices, CUDA_VISIBLE_DEVICES=1",
			systemDevices:   []ddnvml.Device{gpuWithOneMigChild, gpuWithTwoMigChildren},
			visibleDevices:  "1",
			expectedDevices: []ddnvml.Device{gpuWithTwoMigChildren.MIGChildren[0]},
		},
		{
			name:            "multiple MIG devices, CUDA_VISIBLE_DEVICES=0,1",
			systemDevices:   []ddnvml.Device{gpuWithOneMigChild, gpuWithTwoMigChildren},
			visibleDevices:  "0,1",
			expectedDevices: []ddnvml.Device{gpuWithOneMigChild.MIGChildren[0]}, // yes, only the first MIG child is selected, not an error
		},
		{
			name:            "multiple MIG devices, selects first MIG child by UUID",
			systemDevices:   []ddnvml.Device{gpuWithOneMigChild, gpuWithTwoMigChildren},
			visibleDevices:  gpuWithOneMigChild.MIGChildren[0].UUID,
			expectedDevices: []ddnvml.Device{gpuWithOneMigChild.MIGChildren[0]},
		},
		{
			name:            "one non-MIG device and one MIG, selects non-MIG device by UUID",
			systemDevices:   []ddnvml.Device{gpuNoMig, gpuWithOneMigChild},
			visibleDevices:  gpuNoMig.UUID,
			expectedDevices: []ddnvml.Device{gpuNoMig},
		},
		{
			name:            "one non-MIG device and one MIG, selects MIG child by UUID",
			systemDevices:   []ddnvml.Device{gpuNoMig, gpuWithOneMigChild},
			visibleDevices:  gpuWithOneMigChild.MIGChildren[0].UUID,
			expectedDevices: []ddnvml.Device{gpuWithOneMigChild.MIGChildren[0]},
		},
		{
			name:            "one non-MIG device and one MIG, no CUDA_VISIBLE_DEVICES",
			systemDevices:   []ddnvml.Device{gpuNoMig, gpuWithOneMigChild},
			visibleDevices:  "",
			expectedDevices: []ddnvml.Device{gpuWithOneMigChild.MIGChildren[0]},
		},
		{
			name:            "one non-MIG device and one MIG, CUDA_VISIBLE_DEVICES=0",
			systemDevices:   []ddnvml.Device{gpuNoMig, gpuWithOneMigChild},
			visibleDevices:  "0",
			expectedDevices: []ddnvml.Device{gpuNoMig},
		},
		{
			name:            "one non-MIG device and one MIG, CUDA_VISIBLE_DEVICES=1",
			systemDevices:   []ddnvml.Device{gpuNoMig, gpuWithOneMigChild},
			visibleDevices:  "1",
			expectedDevices: []ddnvml.Device{gpuWithOneMigChild.MIGChildren[0]},
		},
		// The MIG children get put at the end of the list, so they're not actually the first device
		// The non-MIG devices get put at the beginning of the list
		{
			name:            "one MIG device and one non-MIG device, CUDA_VISIBLE_DEVICES=0",
			systemDevices:   []ddnvml.Device{gpuWithOneMigChild, gpuNoMig},
			visibleDevices:  "0",
			expectedDevices: []ddnvml.Device{gpuNoMig},
		},
		{
			name:            "one MIG device and one non-MIG device, CUDA_VISIBLE_DEVICES=1",
			systemDevices:   []ddnvml.Device{gpuWithOneMigChild, gpuNoMig},
			visibleDevices:  "1",
			expectedDevices: []ddnvml.Device{gpuWithOneMigChild.MIGChildren[0]},
		},
		{
			name:            "device with MIG enabled but no children gets ignored",
			systemDevices:   []ddnvml.Device{gpuWithMigEnabledButNoChildren, gpuWithOneMigChild},
			visibleDevices:  "0",
			expectedDevices: []ddnvml.Device{gpuWithOneMigChild.MIGChildren[0]},
		},
		// Try the same but with multiple GPUs, to see if the order is still correct
		{
			name:            "mixed MIG and non-MIG devices, CUDA_VISIBLE_DEVICES=0",
			systemDevices:   []ddnvml.Device{gpuWithOneMigChild, gpuNoMig, gpuWithTwoMigChildren, gpuNoMig2},
			visibleDevices:  "0",
			expectedDevices: []ddnvml.Device{gpuNoMig},
		},
		{
			name:            "mixed MIG and non-MIG devices, CUDA_VISIBLE_DEVICES=1",
			systemDevices:   []ddnvml.Device{gpuWithOneMigChild, gpuNoMig, gpuWithTwoMigChildren, gpuNoMig2},
			visibleDevices:  "1",
			expectedDevices: []ddnvml.Device{gpuNoMig2},
		},
		{
			name:            "mixed MIG and non-MIG devices, CUDA_VISIBLE_DEVICES=2",
			systemDevices:   []ddnvml.Device{gpuWithOneMigChild, gpuNoMig, gpuWithTwoMigChildren, gpuNoMig2},
			visibleDevices:  "2",
			expectedDevices: []ddnvml.Device{gpuWithOneMigChild.MIGChildren[0]},
		},
		{
			name:            "mixed MIG and non-MIG devices, CUDA_VISIBLE_DEVICES=3",
			systemDevices:   []ddnvml.Device{gpuWithOneMigChild, gpuNoMig, gpuWithTwoMigChildren, gpuNoMig2},
			visibleDevices:  "3",
			expectedDevices: []ddnvml.Device{gpuWithTwoMigChildren.MIGChildren[0]},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			devices, err := ParseVisibleDevices(tc.systemDevices, tc.visibleDevices)
			if tc.expectsError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tc.expectedDevices, devices)
		})
	}
}

func TestGetVisibleDevicesForMig(t *testing.T) {
	gpuNoMig := &ddnvml.PhysicalDevice{
		DeviceInfo: ddnvml.DeviceInfo{
			UUID: "GPU-1234",
		},
	}

	gpuWithOneMigChild := &ddnvml.PhysicalDevice{
		DeviceInfo: ddnvml.DeviceInfo{
			UUID: "GPU-3456",
		},
		HasMIGFeatureEnabled: true,
		MIGChildren: []*ddnvml.MIGDevice{
			{
				DeviceInfo: ddnvml.DeviceInfo{
					UUID: "MIG-3456-1234-1234-1234",
				},
			},
		},
	}

	gpuWithTwoMigChildren := &ddnvml.PhysicalDevice{
		DeviceInfo: ddnvml.DeviceInfo{
			UUID: "GPU-7890",
		},
		HasMIGFeatureEnabled: true,
		MIGChildren: []*ddnvml.MIGDevice{
			{
				DeviceInfo: ddnvml.DeviceInfo{
					UUID: "MIG-7890-1234-1234-1234",
				},
			},
		},
	}

	gpuWithMigEnabledButNoChildren := &ddnvml.PhysicalDevice{
		DeviceInfo: ddnvml.DeviceInfo{
			UUID: "GPU-9999",
		},
		HasMIGFeatureEnabled: true,
	}

	cases := []struct {
		name            string
		systemDevices   []ddnvml.Device
		expectedDevices []ddnvml.Device
	}{
		{
			name:            "no MIG devices",
			systemDevices:   []ddnvml.Device{gpuNoMig},
			expectedDevices: []ddnvml.Device{gpuNoMig},
		},
		{
			name:            "one MIG device",
			systemDevices:   []ddnvml.Device{gpuWithOneMigChild},
			expectedDevices: []ddnvml.Device{gpuWithOneMigChild.MIGChildren[0]},
		},
		{
			name:            "two MIG devices",
			systemDevices:   []ddnvml.Device{gpuWithTwoMigChildren},
			expectedDevices: []ddnvml.Device{gpuWithTwoMigChildren.MIGChildren[0]},
		},
		{
			name:            "one non-MIG device and one MIG device",
			systemDevices:   []ddnvml.Device{gpuNoMig, gpuWithOneMigChild},
			expectedDevices: []ddnvml.Device{gpuNoMig, gpuWithOneMigChild.MIGChildren[0]},
		},
		{
			name:            "MIG-enabled device with no children",
			systemDevices:   []ddnvml.Device{gpuWithMigEnabledButNoChildren},
			expectedDevices: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			devices := getVisibleDevicesForMig(tc.systemDevices)
			require.Equal(t, tc.expectedDevices, devices)
		})
	}
}
