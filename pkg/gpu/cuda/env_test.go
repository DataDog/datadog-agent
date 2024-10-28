// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package cuda

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"
)

func TestGetVisibleDevices(t *testing.T) {
	commonPrefix := "GPU-89"
	uuid1 := commonPrefix + "32f937-d72c-4106-c12f-20bd9faed9f6"
	uuid2 := commonPrefix + "02f078-a8da-4036-a78f-4032bbddeaf2"

	dev1 := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid1, nvml.SUCCESS
		},
	}

	dev2 := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid2, nvml.SUCCESS
		},
	}

	devList := []nvml.Device{dev1, dev2}
	cases := []struct {
		name            string
		visibleDevices  string
		expectedDevices []nvml.Device
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
			expectedDevices: []nvml.Device{devList[0]},
			expectsError:    false,
		},
		{
			name:            "Index",
			visibleDevices:  "1",
			expectedDevices: []nvml.Device{devList[1]},
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
			name:            "MIGDevices",
			visibleDevices:  "MIG-GPU-1",
			expectedDevices: nil,
			expectsError:    true,
		},
		{name: "UnorderedIndexes",
			visibleDevices:  "1,0",
			expectedDevices: []nvml.Device{devList[1], devList[0]},
			expectsError:    false,
		},
		{
			name:            "MixedIndexesAndUUIDs",
			visibleDevices:  "0," + uuid2,
			expectedDevices: []nvml.Device{devList[0], devList[1]},
			expectsError:    false,
		},
		{
			name:            "InvalidIndexInMiddle",
			visibleDevices:  "0,235,1",
			expectedDevices: []nvml.Device{devList[0]},
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
			devices, err := getVisibleDevices(devList, tc.visibleDevices)
			if tc.expectsError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tc.expectedDevices, devices)
		})
	}
}
