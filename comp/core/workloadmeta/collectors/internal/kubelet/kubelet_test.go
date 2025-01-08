// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && test

package kubelet

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestPodParser(t *testing.T) {

	referencePod := []*kubelet.Pod{
		{
			Metadata: kubelet.PodMetadata{
				Name:      "TestPod",
				UID:       "uniqueIdentifier",
				Namespace: "namespace",
				Owners: []kubelet.PodOwner{
					{
						Kind: "ReplicaSet",
						Name: "deployment-hashrs",
						ID:   "ownerUID",
					},
				},
				Annotations: map[string]string{
					"annotationKey": "annotationValue",
				},
				Labels: map[string]string{
					"labelKey": "labelValue",
				},
			},
			Spec: kubelet.Spec{
				PriorityClassName: "priorityClass",
				Volumes: []kubelet.VolumeSpec{
					{
						Name: "pvcVol",
						PersistentVolumeClaim: &kubelet.PersistentVolumeClaimSpec{
							ClaimName: "pvcName",
						},
					},
				},
				Containers: []kubelet.ContainerSpec{
					{
						Name:  "nginx-container",
						Image: "nginx:1.25.2",
						Resources: &kubelet.ContainerResourcesSpec{
							Requests: kubelet.ResourceList{
								"nvidia.com/gpu": resource.Quantity{
									Format: "1",
								},
								"cpu": resource.MustParse("100m"),
							},
						},
					},
				},
			},
			Status: kubelet.Status{
				Phase: string(corev1.PodRunning),
				Conditions: []kubelet.Conditions{
					{
						Type:   string(corev1.PodReady),
						Status: string(corev1.ConditionTrue),
					},
				},
				PodIP:    "127.0.0.1",
				QOSClass: string(corev1.PodQOSGuaranteed),
				Containers: []kubelet.ContainerStatus{
					{
						Name:    "nginx-container",
						ImageID: "5dbe7e1b6b9c",
						Image:   "nginx:1.25.2",
						ID:      "docker://containerID",
						Ready:   true,
					},
				},
			},
		},
	}

	events := parsePods(referencePod)
	containerEvent, podEvent := events[0], events[1]

	expectedContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "containerID",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "nginx-container",
			Labels: map[string]string{
				kubernetes.CriContainerNamespaceLabel: "namespace",
			},
		},
		Image: workloadmeta.ContainerImage{
			ID:        "5dbe7e1b6b9c",
			Name:      "nginx",
			ShortName: "nginx",
			Tag:       "1.25.2",
			RawName:   "nginx:1.25.2",
		},
		Runtime: "docker",
		Resources: workloadmeta.ContainerResources{
			GPUVendorList: []string{"nvidia"},
			CPURequest:    pointer.Ptr(10.0),
		},
		Owner: &workloadmeta.EntityID{
			Kind: "kubernetes_pod",
			ID:   "uniqueIdentifier",
		},
		Ports:   []workloadmeta.ContainerPort{},
		EnvVars: map[string]string{},
		State: workloadmeta.ContainerState{
			Health: "healthy",
		},
	}
	expectedPod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "uniqueIdentifier",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "TestPod",
			Namespace: "namespace",
			Annotations: map[string]string{
				"annotationKey": "annotationValue",
			},
			Labels: map[string]string{
				"labelKey": "labelValue",
			},
		},
		Phase: "Running",
		Owners: []workloadmeta.KubernetesPodOwner{
			{
				Kind: "ReplicaSet",
				Name: "deployment-hashrs",
				ID:   "ownerUID",
			},
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				Name: "nginx-container",
				ID:   "containerID",
				Image: workloadmeta.ContainerImage{
					ID:        "5dbe7e1b6b9c",
					Name:      "nginx",
					ShortName: "nginx",
					Tag:       "1.25.2",
					RawName:   "nginx:1.25.2",
				},
				Resources: workloadmeta.ContainerResources{
					GPUVendorList: []string{"nvidia"},
					CPURequest:    pointer.Ptr(10.0),
				},
			},
		},
		InitContainers:             []workloadmeta.OrchestratorContainer{},
		PersistentVolumeClaimNames: []string{"pvcName"},
		Ready:                      true,
		IP:                         "127.0.0.1",
		PriorityClass:              "priorityClass",
		GPUVendorList:              []string{"nvidia"},
		QOSClass:                   "Guaranteed",
	}

	assert.Equal(t, expectedPod, podEvent.Entity)

	assert.Equal(t, expectedContainer, containerEvent.Entity)
}
