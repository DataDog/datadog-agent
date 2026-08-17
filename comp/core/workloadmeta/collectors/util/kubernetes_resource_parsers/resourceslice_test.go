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
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "s"},
		"spec": map[string]interface{}{
			"driver":   "gpu.nvidia.com",
			"nodeName": "node-1",
			"pool":     map[string]interface{}{"name": "node-1", "generation": int64(1)},
			"devices": []interface{}{
				map[string]interface{}{
					"name": "gpu-0",
					"attributes": map[string]interface{}{
						"uuid": map[string]interface{}{"string": "GPU-abc"},
					},
				},
			},
		},
	}}

	slice := NewResourceSliceParser().Parse(obj).(*workloadmeta.KubernetesResourceSlice)
	assert.Equal(t, "gpu.nvidia.com", slice.Driver)
	assert.Equal(t, "node-1", slice.NodeName)
	assert.Equal(t, int64(1), slice.PoolGeneration)
	assert.Len(t, slice.Devices, 1)
	assert.Equal(t, "GPU-abc", slice.Devices[0].UUID)
}

func TestResourceSliceParserV1beta1BasicUnwrap(t *testing.T) {
	// v1beta1 nests attributes and capacity under "basic". Without the unwrap,
	// a v1beta1 cluster silently yields devices with no attributes.
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "s"},
		"spec": map[string]interface{}{
			"driver":   "gpu.nvidia.com",
			"nodeName": "node-1",
			"pool":     map[string]interface{}{"name": "node-1", "generation": int64(1)},
			"devices": []interface{}{
				map[string]interface{}{
					"name": "gpu-0",
					"basic": map[string]interface{}{
						"attributes": map[string]interface{}{
							"uuid": map[string]interface{}{"string": "GPU-basic"},
						},
						"capacity": map[string]interface{}{
							"memory": map[string]interface{}{"value": "81152Mi"},
						},
					},
				},
			},
		},
	}}

	slice := NewResourceSliceParser().Parse(obj).(*workloadmeta.KubernetesResourceSlice)
	assert.Len(t, slice.Devices, 1)
	assert.Equal(t, "GPU-basic", slice.Devices[0].UUID)
	assert.Equal(t, int64(85094039552), slice.Devices[0].MemoryBytes)
}

func TestResourceSliceParserMIGShape(t *testing.T) {
	// A MIG device carries parentUUID + profile and no uuid of its own.
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "s"},
		"spec": map[string]interface{}{
			"driver":   "gpu.nvidia.com",
			"nodeName": "node-1",
			"pool":     map[string]interface{}{"name": "node-1", "generation": int64(1)},
			"devices": []interface{}{
				map[string]interface{}{
					"name": "gpu-0-mig-1g10gb-19-0",
					"attributes": map[string]interface{}{
						"parentUUID": map[string]interface{}{"string": "GPU-parent"},
						"profile":    map[string]interface{}{"string": "1g.10gb"},
					},
					"capacity": map[string]interface{}{
						"memory": map[string]interface{}{"value": "9984Mi"},
					},
				},
			},
		},
	}}

	slice := NewResourceSliceParser().Parse(obj).(*workloadmeta.KubernetesResourceSlice)
	assert.Len(t, slice.Devices, 1)
	assert.Empty(t, slice.Devices[0].UUID)
	assert.Equal(t, "GPU-parent", slice.Devices[0].ParentUUID)
	assert.Equal(t, "1g.10gb", slice.Devices[0].Profile)
	assert.Equal(t, int64(10468982784), slice.Devices[0].MemoryBytes)
}

func TestResourceSliceCapacityQuantityParsing(t *testing.T) {
	// Capacities are Kubernetes quantities, not plain integers.
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "s"},
		"spec": map[string]interface{}{
			"driver":   "gpu.nvidia.com",
			"nodeName": "node-1",
			"pool":     map[string]interface{}{"name": "node-1", "generation": int64(1)},
			"devices": []interface{}{
				map[string]interface{}{
					"name": "gpu-0",
					"capacity": map[string]interface{}{
						"memory": map[string]interface{}{"value": "81152Mi"},
					},
				},
			},
		},
	}}

	slice := NewResourceSliceParser().Parse(obj).(*workloadmeta.KubernetesResourceSlice)
	assert.Equal(t, int64(85094039552), slice.Devices[0].MemoryBytes)
}
