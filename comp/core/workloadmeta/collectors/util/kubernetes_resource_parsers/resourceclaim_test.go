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
	tests := []struct {
		name     string
		obj      *unstructured.Unstructured
		expected *workloadmeta.KubernetesResourceClaim
	}{
		{
			// A freshly created claim with no allocation: the workload is
			// waiting for accelerators to be scheduled.
			name: "pending claim (waiting for GPUs)",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "pending-claim",
						"namespace": "team-a",
						"uid":       "uid-pending",
					},
					"spec": map[string]interface{}{
						"devices": map[string]interface{}{
							"requests": []interface{}{
								map[string]interface{}{
									"name": "req-0",
									"exactly": map[string]interface{}{
										"allocationMode":  "ExactCount",
										"count":           int64(4),
										"deviceClassName": "gpu.nvidia.com",
									},
								},
							},
						},
					},
				},
			},
			expected: &workloadmeta.KubernetesResourceClaim{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesResourceClaim,
					ID:   "team-a/pending-claim",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "pending-claim",
					Namespace: "team-a",
					UID:       "uid-pending",
				},
				State: workloadmeta.ResourceClaimPending,
				// Read from spec.devices.requests, so it is available before the
				// scheduler allocates anything.
				RequestedDeviceClasses: []string{"gpu.nvidia.com"},
				// Populated even while pending: once devices are allocated they
				// name this request, which is how they get classified.
				DeviceClassByRequest: map[string]string{"req-0": "gpu.nvidia.com"},
			},
		},
		{
			// The allocated+reserved shape, mirroring the yanmega "scooby"
			// claim: 4 H100s allocated on one node, reserved for the worker pod.
			name: "allocated and reserved claim (training)",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "scooby-claim",
						"namespace": "team-a",
						"uid":       "uid-scooby",
						"ownerReferences": []interface{}{
							map[string]interface{}{
								"kind": "Pod",
								"name": "glm47-worker-zqlhs",
							},
						},
					},
					"status": map[string]interface{}{
						"allocation": map[string]interface{}{
							"devices": map[string]interface{}{
								"results": []interface{}{
									map[string]interface{}{"device": "gpu-2", "driver": "gpu.nvidia.com", "pool": "ip-10-166-67-148", "request": "container-6-request-2"},
									map[string]interface{}{"device": "gpu-3", "driver": "gpu.nvidia.com", "pool": "ip-10-166-67-148", "request": "container-6-request-2"},
									map[string]interface{}{"device": "gpu-5", "driver": "gpu.nvidia.com", "pool": "ip-10-166-67-148", "request": "container-6-request-2"},
									map[string]interface{}{"device": "gpu-7", "driver": "gpu.nvidia.com", "pool": "ip-10-166-67-148", "request": "container-6-request-2"},
								},
							},
							"nodeSelector": map[string]interface{}{
								"nodeSelectorTerms": []interface{}{
									map[string]interface{}{
										"matchFields": []interface{}{
											map[string]interface{}{
												"key":      "metadata.name",
												"operator": "In",
												"values":   []interface{}{"ip-10-166-67-148"},
											},
										},
									},
								},
							},
						},
						"reservedFor": []interface{}{
							map[string]interface{}{"name": "glm47-worker-zqlhs", "resource": "pods"},
						},
					},
				},
			},
			expected: &workloadmeta.KubernetesResourceClaim{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesResourceClaim,
					ID:   "team-a/scooby-claim",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "scooby-claim",
					Namespace: "team-a",
					UID:       "uid-scooby",
				},
				State:    workloadmeta.ResourceClaimReserved,
				NodeName: "ip-10-166-67-148",
				Devices: []workloadmeta.ResourceClaimDevice{
					{Name: "gpu-2", Driver: "gpu.nvidia.com", Pool: "ip-10-166-67-148", Request: "container-6-request-2"},
					{Name: "gpu-3", Driver: "gpu.nvidia.com", Pool: "ip-10-166-67-148", Request: "container-6-request-2"},
					{Name: "gpu-5", Driver: "gpu.nvidia.com", Pool: "ip-10-166-67-148", Request: "container-6-request-2"},
					{Name: "gpu-7", Driver: "gpu.nvidia.com", Pool: "ip-10-166-67-148", Request: "container-6-request-2"},
				},
				ReservedForPods: []string{"glm47-worker-zqlhs"},
				OwnerPod:        "glm47-worker-zqlhs",
			},
		},
		{
			// Devices allocated but not yet reserved for a consumer.
			name: "allocated but not reserved claim",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "alloc-only",
						"namespace": "team-a",
						"uid":       "uid-alloc",
					},
					"status": map[string]interface{}{
						"allocation": map[string]interface{}{
							"devices": map[string]interface{}{
								"results": []interface{}{
									map[string]interface{}{"device": "gpu-0", "driver": "gpu.nvidia.com", "pool": "node-x", "request": "req-0"},
								},
							},
						},
					},
				},
			},
			expected: &workloadmeta.KubernetesResourceClaim{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesResourceClaim,
					ID:   "team-a/alloc-only",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "alloc-only",
					Namespace: "team-a",
					UID:       "uid-alloc",
				},
				State: workloadmeta.ResourceClaimAllocated,
				Devices: []workloadmeta.ResourceClaimDevice{
					{Name: "gpu-0", Driver: "gpu.nvidia.com", Pool: "node-x", Request: "req-0"},
				},
			},
		},
	}

	parser := NewResourceClaimParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity := parser.Parse(tt.obj)
			assert.Equal(t, tt.expected, entity)
		})
	}
}

// A device allocated with adminAccess is monitoring/management access, not
// ordinary consumption -- Kubernetes says such claims ignore ordinary claims to
// the device entirely. The flag must survive parsing so the check can exclude it
// instead of counting it as allocated capacity.
func TestResourceClaimParsesAdminAccessOnAllocatedDevice(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "c", "namespace": "team-a"},
			"status": map[string]interface{}{
				"allocation": map[string]interface{}{
					"devices": map[string]interface{}{
						"results": []interface{}{
							map[string]interface{}{
								"device": "gpu-0", "driver": "gpu.nvidia.com",
								"pool": "node-1", "request": "a",
								"adminAccess": true,
							},
							map[string]interface{}{
								"device": "gpu-1", "driver": "gpu.nvidia.com",
								"pool": "node-1", "request": "b",
							},
						},
					},
				},
			},
		},
	}

	claim := NewResourceClaimParser().Parse(obj).(*workloadmeta.KubernetesResourceClaim)
	assert.Len(t, claim.Devices, 2)
	assert.True(t, claim.Devices[0].AdminAccess)
	// Absent means ordinary consumption, not admin access.
	assert.False(t, claim.Devices[1].AdminAccess)
}

func TestResourceClaimSkipsAllocationResultWithoutDeviceName(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "c", "namespace": "team-a"},
			"status": map[string]interface{}{
				"allocation": map[string]interface{}{
					"devices": map[string]interface{}{
						"results": []interface{}{
							map[string]interface{}{
								"device": "gpu-0", "driver": "gpu.nvidia.com",
								"pool": "node-1", "request": "a",
							},
							map[string]interface{}{
								// Malformed/partial result: no device name. It cannot be
								// joined to a ResourceSlice, so it must be skipped rather
								// than counted positionally and inflating devices.allocated.
								"driver": "gpu.nvidia.com", "pool": "node-1", "request": "b",
							},
						},
					},
				},
			},
		},
	}

	claim := NewResourceClaimParser().Parse(obj).(*workloadmeta.KubernetesResourceClaim)
	assert.Len(t, claim.Devices, 1)
	assert.Equal(t, "gpu-0", claim.Devices[0].Name)
}

// A request marked adminAccess makes the whole claim administrative. Before
// allocation there is no result-level flag, so the request-level flag is the
// only way to keep a pending administrative claim out of the waiting-workload
// count. In v1 the flag lives inside the "exactly" wrapper; in v1beta1 it sits
// directly on the request.
func TestResourceClaimParsesRequestLevelAdminAccess(t *testing.T) {
	// v1 shape: adminAccess inside "exactly".
	adminObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "admin", "namespace": "team-a"},
			"spec": map[string]interface{}{
				"devices": map[string]interface{}{
					"requests": []interface{}{
						map[string]interface{}{
							"name": "req-0",
							"exactly": map[string]interface{}{
								"deviceClassName": "gpu.nvidia.com",
								"adminAccess":     true,
							},
						},
					},
				},
			},
		},
	}
	claim := NewResourceClaimParser().Parse(adminObj).(*workloadmeta.KubernetesResourceClaim)
	assert.True(t, claim.AdminAccess)

	// v1beta1 shape: adminAccess directly on the request.
	betaAdminObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "admin-beta", "namespace": "team-a"},
			"spec": map[string]interface{}{
				"devices": map[string]interface{}{
					"requests": []interface{}{
						map[string]interface{}{
							"name":            "req-0",
							"deviceClassName": "gpu.nvidia.com",
							"adminAccess":     true,
						},
					},
				},
			},
		},
	}
	claim = NewResourceClaimParser().Parse(betaAdminObj).(*workloadmeta.KubernetesResourceClaim)
	assert.True(t, claim.AdminAccess)

	// A request without adminAccess is ordinary consumption.
	plainObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "plain", "namespace": "team-a"},
			"spec": map[string]interface{}{
				"devices": map[string]interface{}{
					"requests": []interface{}{
						map[string]interface{}{
							"name": "req-0",
							"exactly": map[string]interface{}{
								"deviceClassName": "gpu.nvidia.com",
							},
						},
					},
				},
			},
		},
	}
	claim = NewResourceClaimParser().Parse(plainObj).(*workloadmeta.KubernetesResourceClaim)
	assert.False(t, claim.AdminAccess)
}

// resource.k8s.io/v1beta1 puts deviceClassName directly on the request, with no
// "exactly" wrapper. Version selection is by discovery (no version is pinned in
// the GVR), so a cluster whose preferred version is v1beta1 must still parse --
// otherwise every claim silently reports no requested classes and no accelerator
// claim is ever recognised.
func TestResourceClaimRequestedDeviceClassesV1beta1(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "c", "namespace": "team-a"},
			"spec": map[string]interface{}{
				"devices": map[string]interface{}{
					"requests": []interface{}{
						map[string]interface{}{
							"name":            "req-beta",
							"deviceClassName": "gpu.nvidia.com",
						},
					},
				},
			},
		},
	}

	claim := NewResourceClaimParser().Parse(obj).(*workloadmeta.KubernetesResourceClaim)
	assert.Equal(t, []string{"gpu.nvidia.com"}, claim.RequestedDeviceClasses)
	assert.Equal(t, map[string]string{"req-beta": "gpu.nvidia.com"}, claim.DeviceClassByRequest)
}

func TestResourceClaimRequestedDeviceClasses(t *testing.T) {
	// A request either names one class outright or offers alternatives; both
	// shapes must be read, since this is the only accelerator signal available
	// on a claim that has not been allocated yet.
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "c", "namespace": "team-a"},
			"spec": map[string]interface{}{
				"devices": map[string]interface{}{
					"requests": []interface{}{
						map[string]interface{}{
							"name":    "req-exact",
							"exactly": map[string]interface{}{"deviceClassName": "gpu.nvidia.com"},
						},
						map[string]interface{}{
							"name": "req-alternatives",
							"firstAvailable": []interface{}{
								map[string]interface{}{"name": "alt-mig", "deviceClassName": "mig.nvidia.com"},
								// Duplicates collapse.
								map[string]interface{}{"name": "alt-whole", "deviceClassName": "gpu.nvidia.com"},
							},
						},
					},
				},
			},
		},
	}

	claim := NewResourceClaimParser().Parse(obj).(*workloadmeta.KubernetesResourceClaim)
	assert.Equal(t, []string{"gpu.nvidia.com", "mig.nvidia.com"}, claim.RequestedDeviceClasses)

	// The per-request mapping is what lets an allocated device be classified:
	// the device records its request, and the request names the class. A
	// "firstAvailable" subrequest is keyed "<request>/<subrequest>", which is
	// how an allocation result refers to it.
	assert.Equal(t, map[string]string{
		"req-exact":                  "gpu.nvidia.com",
		"req-alternatives/alt-mig":   "mig.nvidia.com",
		"req-alternatives/alt-whole": "gpu.nvidia.com",
	}, claim.DeviceClassByRequest)
}
