// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubelet && kubeapiserver

package kubelet

import (
	"testing"

	"github.com/stretchr/testify/require"
	resourcev1 "k8s.io/api/resource/v1"

	kubeletutil "github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

func TestResolveDRAResourceFromSlices(t *testing.T) {
	resource := kubeletutil.ContainerClaimResource{
		DriverName: nvidiaDRADriverName,
		PoolName:   "node-a",
		DeviceName: "gpu-0",
	}

	tests := []struct {
		name     string
		resource kubeletutil.ContainerClaimResource
		slices   []*resourcev1.ResourceSlice
		expected kubeletutil.ContainerAllocatedResource
		ok       bool
	}{
		{
			name:     "physical GPU UUID",
			resource: resource,
			slices: []*resourcev1.ResourceSlice{
				resourceSlice("slice-1", nvidiaDRADriverName, "node-a", 2, 1,
					device("gpu-0", "GPU-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")),
			},
			expected: kubeletutil.ContainerAllocatedResource{
				Name: nvidiaDRADriverName,
				ID:   "GPU-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			},
			ok: true,
		},
		{
			name:     "MIG UUID",
			resource: resource,
			slices: []*resourcev1.ResourceSlice{
				resourceSlice("slice-1", nvidiaDRADriverName, "node-a", 2, 1,
					device("gpu-0", "MIG-GPU-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee/1/0")),
			},
			expected: kubeletutil.ContainerAllocatedResource{
				Name: nvidiaDRADriverName,
				ID:   "MIG-GPU-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee/1/0",
			},
			ok: true,
		},
		{
			name:     "wrong driver",
			resource: resource,
			slices: []*resourcev1.ResourceSlice{
				resourceSlice("slice-1", "example.com/driver", "node-a", 2, 1,
					device("gpu-0", "GPU-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")),
			},
		},
		{
			name:     "wrong pool",
			resource: resource,
			slices: []*resourcev1.ResourceSlice{
				resourceSlice("slice-1", nvidiaDRADriverName, "node-b", 2, 1,
					device("gpu-0", "GPU-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")),
			},
		},
		{
			name:     "wrong device",
			resource: resource,
			slices: []*resourcev1.ResourceSlice{
				resourceSlice("slice-1", nvidiaDRADriverName, "node-a", 2, 1,
					device("gpu-1", "GPU-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")),
			},
		},
		{
			name:     "missing UUID attribute",
			resource: resource,
			slices: []*resourcev1.ResourceSlice{
				resourceSlice("slice-1", nvidiaDRADriverName, "node-a", 2, 1,
					resourcev1.Device{Name: "gpu-0"}),
			},
		},
		{
			name:     "wrong typed UUID attribute",
			resource: resource,
			slices: []*resourcev1.ResourceSlice{
				resourceSlice("slice-1", nvidiaDRADriverName, "node-a", 2, 1,
					deviceWithAttributes("gpu-0", map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
						"uuid": {IntValue: ptr(int64(1))},
					})),
			},
		},
		{
			name:     "incomplete highest generation",
			resource: resource,
			slices: []*resourcev1.ResourceSlice{
				resourceSlice("slice-1", nvidiaDRADriverName, "node-a", 2, 1,
					device("gpu-0", "GPU-stale")),
				resourceSlice("slice-2", nvidiaDRADriverName, "node-a", 3, 2,
					device("gpu-0", "GPU-new")),
			},
		},
		{
			name:     "uses complete highest generation",
			resource: resource,
			slices: []*resourcev1.ResourceSlice{
				resourceSlice("slice-1", nvidiaDRADriverName, "node-a", 2, 1,
					device("gpu-0", "GPU-stale")),
				resourceSlice("slice-2", nvidiaDRADriverName, "node-a", 3, 2,
					device("gpu-1", "GPU-other")),
				resourceSlice("slice-3", nvidiaDRADriverName, "node-a", 3, 2,
					device("gpu-0", "GPU-new")),
			},
			expected: kubeletutil.ContainerAllocatedResource{
				Name: nvidiaDRADriverName,
				ID:   "GPU-new",
			},
			ok: true,
		},
		{
			name: "unknown requested driver",
			resource: kubeletutil.ContainerClaimResource{
				DriverName: "example.com/driver",
				PoolName:   "node-a",
				DeviceName: "gpu-0",
			},
			slices: []*resourcev1.ResourceSlice{
				resourceSlice("slice-1", nvidiaDRADriverName, "node-a", 2, 1,
					device("gpu-0", "GPU-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, ok := resolveDRAResourceFromSlices(test.resource, test.slices)
			require.Equal(t, test.ok, ok)
			require.Equal(t, test.expected, resolved)
		})
	}
}

func resourceSlice(_ string, driver, pool string, generation, count int64, devices ...resourcev1.Device) *resourcev1.ResourceSlice {
	return &resourcev1.ResourceSlice{
		Spec: resourcev1.ResourceSliceSpec{
			Driver: driver,
			Pool: resourcev1.ResourcePool{
				Name:               pool,
				Generation:         generation,
				ResourceSliceCount: count,
			},
			Devices: devices,
		},
	}
}

func device(name, uuid string) resourcev1.Device {
	return deviceWithAttributes(name, map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
		"uuid": {StringValue: ptr(uuid)},
	})
}

func deviceWithAttributes(name string, attributes map[resourcev1.QualifiedName]resourcev1.DeviceAttribute) resourcev1.Device {
	return resourcev1.Device{
		Name:       name,
		Attributes: attributes,
	}
}

func ptr[T any](value T) *T {
	return &value
}
