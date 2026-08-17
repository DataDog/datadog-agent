// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package kubernetesresourceparsers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestResourceSliceParser(t *testing.T) {
	tests := []struct {
		name     string
		obj      *unstructured.Unstructured
		expected *workloadmeta.KubernetesResourceSlice
	}{
		{
			// The exact attribute shape published by nvidia-dra-driver-gpu
			// 25.12.0 for a Tesla T4, captured from a live cluster. The uuid is
			// the value NVML reports for the same card.
			name: "physical gpu device",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "ip-192-168-54-39.ec2.internal-gpu.nvidia.com-ld2fr",
						"uid":  "uid-slice",
					},
					"spec": map[string]interface{}{
						"driver":   "gpu.nvidia.com",
						"nodeName": "ip-192-168-54-39.ec2.internal",
						"pool": map[string]interface{}{
							"name": "ip-192-168-54-39.ec2.internal",
						},
						"devices": []interface{}{
							map[string]interface{}{
								"name": "gpu-0",
								"attributes": map[string]interface{}{
									"uuid":                            map[string]interface{}{"string": "GPU-0c09cfd9-1e4c-ae5d-d372-0017b029daa8"},
									"productName":                     map[string]interface{}{"string": "Tesla T4"},
									"architecture":                    map[string]interface{}{"string": "Turing"},
									"resource.kubernetes.io/pcieRoot": map[string]interface{}{"string": "pci0000:00"},
									// A non-string attribute must not be read as a string.
									"cudaComputeCapability": map[string]interface{}{"version": "7.5.0"},
								},
								"capacity": map[string]interface{}{
									"memory": map[string]interface{}{"value": "15360Mi"},
								},
							},
						},
					},
				},
			},
			expected: &workloadmeta.KubernetesResourceSlice{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesResourceSlice,
					ID:   "ip-192-168-54-39.ec2.internal-gpu.nvidia.com-ld2fr",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: "ip-192-168-54-39.ec2.internal-gpu.nvidia.com-ld2fr",
					UID:  "uid-slice",
				},
				NodeName: "ip-192-168-54-39.ec2.internal",
				Driver:   "gpu.nvidia.com",
				Pool:     "ip-192-168-54-39.ec2.internal",
				Devices: []workloadmeta.ResourceSliceDevice{
					{
						Name:        "gpu-0",
						UUID:        "GPU-0c09cfd9-1e4c-ae5d-d372-0017b029daa8",
						ProductName: "Tesla T4",
						PCIeRoot:    "pci0000:00",
						MemoryBytes: 15360 * 1024 * 1024,
					},
				},
			},
		},
		{
			// MIG devices carry parentUUID + profile and no uuid of their own,
			// and their memory capacity differs from the physical card's.
			name: "mig device",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "h100-slice", "uid": "uid-mig"},
					"spec": map[string]interface{}{
						"driver":   "gpu.nvidia.com",
						"nodeName": "node-h100",
						"pool":     map[string]interface{}{"name": "node-h100"},
						"devices": []interface{}{
							map[string]interface{}{
								"name": "gpu-0-mig-1g10gb-19-0",
								"attributes": map[string]interface{}{
									"type":        map[string]interface{}{"string": "mig"},
									"profile":     map[string]interface{}{"string": "1g.10gb"},
									"parentUUID":  map[string]interface{}{"string": "GPU-69748a45-1ba8-35d1-3cc6-0a635023cf7b"},
									"productName": map[string]interface{}{"string": "NVIDIA H100 80GB HBM3"},
								},
								"capacity": map[string]interface{}{
									"memory": map[string]interface{}{"value": "9984Mi"},
								},
							},
						},
					},
				},
			},
			expected: &workloadmeta.KubernetesResourceSlice{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesResourceSlice,
					ID:   "h100-slice",
				},
				EntityMeta: workloadmeta.EntityMeta{Name: "h100-slice", UID: "uid-mig"},
				NodeName:   "node-h100",
				Driver:     "gpu.nvidia.com",
				Pool:       "node-h100",
				Devices: []workloadmeta.ResourceSliceDevice{
					{
						Name:        "gpu-0-mig-1g10gb-19-0",
						ProductName: "NVIDIA H100 80GB HBM3",
						ParentUUID:  "GPU-69748a45-1ba8-35d1-3cc6-0a635023cf7b",
						Profile:     "1g.10gb",
						MemoryBytes: 9984 * 1024 * 1024,
					},
				},
			},
		},
		{
			// The first slice of a pool carries sharedCounters and no devices.
			name: "slice with no devices",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "counters-only", "uid": "uid-empty"},
					"spec": map[string]interface{}{
						"driver":   "gpu.nvidia.com",
						"nodeName": "node-h100",
						"pool":     map[string]interface{}{"name": "node-h100"},
					},
				},
			},
			expected: &workloadmeta.KubernetesResourceSlice{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesResourceSlice,
					ID:   "counters-only",
				},
				EntityMeta: workloadmeta.EntityMeta{Name: "counters-only", UID: "uid-empty"},
				NodeName:   "node-h100",
				Driver:     "gpu.nvidia.com",
				Pool:       "node-h100",
			},
		},
	}

	parser := NewResourceSliceParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parser.Parse(tt.obj))
		})
	}
}

func TestResourceSliceDeviceCapacityUnparseable(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "s"},
			"spec": map[string]interface{}{
				"devices": []interface{}{
					map[string]interface{}{
						"name":     "gpu-0",
						"capacity": map[string]interface{}{"memory": map[string]interface{}{"value": "not-a-quantity"}},
					},
					// A device without a name cannot be joined to anything, so
					// it is dropped rather than stored with an empty key.
					map[string]interface{}{"name": ""},
				},
			},
		},
	}

	slice := parserResourceSlice(t, obj)
	assert.Len(t, slice.Devices, 1)
	assert.Equal(t, "gpu-0", slice.Devices[0].Name)
	assert.Zero(t, slice.Devices[0].MemoryBytes)
}

// resource.k8s.io/v1beta1 nests a device's attributes and capacity under
// "basic" instead of putting them on the device. Version selection is by
// discovery, so a v1beta1 cluster must still yield UUIDs, parent UUIDs and
// capacities -- without them the MIG collapse in the gpu_allocation check has
// nothing to deduplicate on and silently degrades to positional counting.
func TestResourceSliceParserV1beta1BasicDevice(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "s-beta"},
			"spec": map[string]interface{}{
				"nodeName": "node-1",
				"driver":   "gpu.nvidia.com",
				"pool":     map[string]interface{}{"name": "node-1"},
				"devices": []interface{}{
					map[string]interface{}{
						"name": "mig-0",
						"basic": map[string]interface{}{
							"attributes": map[string]interface{}{
								"parentUUID":  map[string]interface{}{"string": "GPU-aaa"},
								"profile":     map[string]interface{}{"string": "1g.10gb"},
								"productName": map[string]interface{}{"string": "NVIDIA H100"},
							},
							"capacity": map[string]interface{}{
								"memory": map[string]interface{}{"value": "10Gi"},
							},
						},
					},
				},
			},
		},
	}

	slice := parserResourceSlice(t, obj)
	assert.Len(t, slice.Devices, 1)
	assert.Equal(t, "mig-0", slice.Devices[0].Name)
	assert.Equal(t, "GPU-aaa", slice.Devices[0].ParentUUID)
	assert.Equal(t, "1g.10gb", slice.Devices[0].Profile)
	assert.Equal(t, "NVIDIA H100", slice.Devices[0].ProductName)
	assert.Equal(t, int64(10*1024*1024*1024), slice.Devices[0].MemoryBytes)
}

func parserResourceSlice(t *testing.T, obj *unstructured.Unstructured) *workloadmeta.KubernetesResourceSlice {
	t.Helper()
	entity := NewResourceSliceParser().Parse(obj)
	slice, ok := entity.(*workloadmeta.KubernetesResourceSlice)
	assert.True(t, ok)
	return slice
}
