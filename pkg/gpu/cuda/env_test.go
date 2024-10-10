// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package cuda

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockGpuDevice struct {
	mock.Mock
}

func (m *mockGpuDevice) GetUUID() (string, nvml.Return) {
	args := m.Called()
	return args.String(0), args.Get(1).(nvml.Return)
}

func TestGetVisibleDevices(t *testing.T) {
	uuid1 := "GPU-8932f937-d72c-4106-c12f-20bd9faed9f6"
	uuid2 := "GPU-1902f078-a8da-4036-a78f-4032bbddeaf2"

	devList := []WithUUID{
		&mockGpuDevice{},
		&mockGpuDevice{},
	}

	devList[0].(*mockGpuDevice).On("GetUUID").Return(uuid1, nvml.SUCCESS)
	devList[1].(*mockGpuDevice).On("GetUUID").Return(uuid2, nvml.SUCCESS)

	cases := []struct {
		name            string
		visibleDevices  string
		expectedDevices []WithUUID
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
			expectedDevices: []WithUUID{devList[0]},
			expectsError:    false,
		},
		{
			name:            "Index",
			visibleDevices:  "1",
			expectedDevices: []WithUUID{devList[1]},
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
			expectedDevices: []WithUUID{devList[1], devList[0]},
			expectsError:    false,
		},
		{
			name:            "MixedIndexesAndUUIDs",
			visibleDevices:  "0," + uuid2,
			expectedDevices: []WithUUID{devList[0], devList[1]},
			expectsError:    false,
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
