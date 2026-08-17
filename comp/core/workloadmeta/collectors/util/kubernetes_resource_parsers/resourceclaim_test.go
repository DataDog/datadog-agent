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

func TestResourceClaimParser(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "c", "namespace": "team-a"},
		"spec": map[string]interface{}{
			"devices": map[string]interface{}{
				"requests": []interface{}{
					map[string]interface{}{
						"name": "gpu",
						"exactly": map[string]interface{}{
							"deviceClassName": "gpu.nvidia.com",
						},
					},
				},
			},
		},
	}}

	claim := NewResourceClaimParser().Parse(obj).(*workloadmeta.KubernetesResourceClaim)
	assert.Equal(t, "gpu.nvidia.com", claim.RequestedDeviceClasses[0])
	assert.Equal(t, workloadmeta.ResourceClaimPending, claim.State)
}

func TestResourceClaimStateFromAllocationPresence(t *testing.T) {
	// status.allocation is present but its result has no device name, so it is
	// skipped by allocatedDevices. The claim must still read as allocated, not
	// pending: state derives from the presence of an allocation, not from how
	// many devices happened to parse.
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "c", "namespace": "team-a"},
		"spec": map[string]interface{}{
			"devices": map[string]interface{}{
				"requests": []interface{}{
					map[string]interface{}{
						"name": "gpu",
						"exactly": map[string]interface{}{
							"deviceClassName": "gpu.nvidia.com",
						},
					},
				},
			},
		},
		"status": map[string]interface{}{
			"allocation": map[string]interface{}{
				"devices": map[string]interface{}{
					"results": []interface{}{
						map[string]interface{}{"driver": "gpu.nvidia.com", "pool": "node-1"},
					},
				},
			},
		},
	}}

	claim := NewResourceClaimParser().Parse(obj).(*workloadmeta.KubernetesResourceClaim)
	assert.Empty(t, claim.Devices)
	assert.Equal(t, workloadmeta.ResourceClaimAllocated, claim.State)
}

func TestResourceClaimAdminAccessBothShapes(t *testing.T) {
	// v1: adminAccess lives inside the "exactly" wrapper.
	v1Obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "c", "namespace": "team-a"},
		"spec": map[string]interface{}{
			"devices": map[string]interface{}{
				"requests": []interface{}{
					map[string]interface{}{
						"name": "gpu",
						"exactly": map[string]interface{}{
							"deviceClassName": "gpu.nvidia.com",
							"adminAccess":     true,
						},
					},
				},
			},
		},
	}}
	assert.True(t, NewResourceClaimParser().Parse(v1Obj).(*workloadmeta.KubernetesResourceClaim).AdminAccess)

	// v1beta1: adminAccess sits directly on the request.
	v1beta1Obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "c", "namespace": "team-a"},
		"spec": map[string]interface{}{
			"devices": map[string]interface{}{
				"requests": []interface{}{
					map[string]interface{}{
						"name":            "gpu",
						"deviceClassName": "gpu.nvidia.com",
						"adminAccess":     true,
					},
				},
			},
		},
	}}
	assert.True(t, NewResourceClaimParser().Parse(v1beta1Obj).(*workloadmeta.KubernetesResourceClaim).AdminAccess)
}

func TestResourceClaimFirstAvailableSubrequestKeying(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "c", "namespace": "team-a"},
		"spec": map[string]interface{}{
			"devices": map[string]interface{}{
				"requests": []interface{}{
					map[string]interface{}{
						"name": "gpu",
						"firstAvailable": []interface{}{
							map[string]interface{}{
								"name":            "mig",
								"deviceClassName": "mig.nvidia.com",
							},
							map[string]interface{}{
								"name":            "whole",
								"deviceClassName": "gpu.nvidia.com",
							},
						},
					},
				},
			},
		},
	}}

	claim := NewResourceClaimParser().Parse(obj).(*workloadmeta.KubernetesResourceClaim)
	assert.Equal(t, []string{"mig.nvidia.com", "gpu.nvidia.com"}, claim.RequestedDeviceClasses)
	// An allocation result names a subrequest as "<request>/<subrequest>".
	assert.Equal(t, "mig.nvidia.com", claim.DeviceClassByRequest["gpu/mig"])
	assert.Equal(t, "gpu.nvidia.com", claim.DeviceClassByRequest["gpu/whole"])
}

func TestResourceClaimV1beta1BareDeviceClassName(t *testing.T) {
	// v1beta1 puts deviceClassName directly on the request, no "exactly" wrapper.
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "c", "namespace": "team-a"},
		"spec": map[string]interface{}{
			"devices": map[string]interface{}{
				"requests": []interface{}{
					map[string]interface{}{
						"name":            "gpu",
						"deviceClassName": "gpu.nvidia.com",
					},
				},
			},
		},
	}}

	claim := NewResourceClaimParser().Parse(obj).(*workloadmeta.KubernetesResourceClaim)
	assert.Equal(t, []string{"gpu.nvidia.com"}, claim.RequestedDeviceClasses)
	assert.Equal(t, "gpu.nvidia.com", claim.DeviceClassByRequest["gpu"])
}

func TestResourceClaimReservedForPodsAndNodeName(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "c", "namespace": "team-a"},
		"spec": map[string]interface{}{
			"devices": map[string]interface{}{
				"requests": []interface{}{
					map[string]interface{}{
						"name": "gpu",
						"exactly": map[string]interface{}{
							"deviceClassName": "gpu.nvidia.com",
						},
					},
				},
			},
		},
		"status": map[string]interface{}{
			"allocation": map[string]interface{}{
				"devices": map[string]interface{}{
					"results": []interface{}{
						map[string]interface{}{
							"device": "gpu-0", "driver": "gpu.nvidia.com", "pool": "node-1", "request": "gpu",
						},
					},
				},
				"nodeSelector": map[string]interface{}{
					"nodeSelectorTerms": []interface{}{
						map[string]interface{}{
							"matchFields": []interface{}{
								map[string]interface{}{
									"key": "metadata.name", "operator": "In", "values": []interface{}{"node-1"},
								},
							},
						},
					},
				},
			},
			"reservedFor": []interface{}{
				map[string]interface{}{"resource": "pods", "name": "pod-1"},
				map[string]interface{}{"resource": "other", "name": "ignored"},
			},
		},
	}}

	claim := NewResourceClaimParser().Parse(obj).(*workloadmeta.KubernetesResourceClaim)
	assert.Equal(t, "node-1", claim.NodeName)
	assert.Equal(t, []string{"pod-1"}, claim.ReservedForPods)
	assert.Equal(t, workloadmeta.ResourceClaimReserved, claim.State)
	assert.Len(t, claim.Devices, 1)
	assert.Equal(t, "gpu-0", claim.Devices[0].Name)
}
