// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && test

package kubelet

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestPodParser(t *testing.T) {
	creationTimestamp := time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)
	startTime := creationTimestamp.Add(time.Minute)

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
				CreationTimestamp: creationTimestamp,
			},
			Spec: kubelet.Spec{
				NodeName:          "test-node",
				HostNetwork:       true,
				PriorityClassName: "priorityClass",
				Volumes: []kubelet.VolumeSpec{
					{
						Name: "pvcVol",
						PersistentVolumeClaim: &kubelet.PersistentVolumeClaimSpec{
							ClaimName: "pvcName",
							ReadOnly:  false,
						},
					},
				},
				Tolerations: []kubelet.Toleration{
					{
						Key:               "node.kubernetes.io/not-ready",
						Operator:          "Exists",
						Effect:            "NoExecute",
						TolerationSeconds: pointer.Ptr(int64(300)),
					},
				},
				InitContainers: []kubelet.ContainerSpec{
					{
						Name:  "init-container-name",
						Image: "busybox:latest",
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
						Env: []kubelet.EnvVar{
							{
								Name:  "DD_ENV",
								Value: "prod",
							},
							{
								Name:  "OTEL_SERVICE_NAME",
								Value: "$(DD_ENV)-$(DD_SERVICE)",
							},
							{
								Name:      "DD_SERVICE",
								Value:     "",
								ValueFrom: &struct{}{},
							},
						},
					},
				},
				EphemeralContainers: []kubelet.ContainerSpec{
					{
						Name:  "ephemeral-container",
						Image: "busybox:latest",
					},
				},
			},
			Status: kubelet.Status{
				Phase:     string(corev1.PodRunning),
				StartTime: startTime,
				HostIP:    "192.168.1.10",
				Reason:    "SomeReason",
				Conditions: []kubelet.Conditions{
					{
						Type:   string(corev1.PodReady),
						Status: string(corev1.ConditionTrue),
					},
				},
				PodIP:    "127.0.0.1",
				QOSClass: string(corev1.PodQOSGuaranteed),
				InitContainers: []kubelet.ContainerStatus{
					{
						Name:         "init-container-name",
						ImageID:      "sha256:abcd1234",
						Image:        "busybox:latest",
						ID:           "docker://init-containerID",
						RestartCount: 0,
					},
				},
				Containers: []kubelet.ContainerStatus{
					{
						Name:         "nginx-container",
						ImageID:      "5dbe7e1b6b9c",
						Image:        "nginx:1.25.2",
						ID:           "docker://containerID",
						Ready:        true,
						RestartCount: 1,
					},
				},
				EphemeralContainers: []kubelet.ContainerStatus{
					{
						Name:    "ephemeral-container",
						ImageID: "12345",
						Image:   "busybox:latest",
						ID:      "docker://ephemeral-container-id",
						Ready:   false,
					},
				},
			},
		},
	}

	events := parsePods(referencePod, true)
	parsedEntities := make([]workloadmeta.Entity, 0, len(events))
	for _, event := range events {
		parsedEntities = append(parsedEntities, event.Entity)
	}

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
		Ports: []workloadmeta.ContainerPort{},
		EnvVars: map[string]string{
			"DD_ENV": "prod",
		},
		State: workloadmeta.ContainerState{
			Health: "healthy",
		},
	}

	expectedEphemeralContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "ephemeral-container-id",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "ephemeral-container",
			Labels: map[string]string{
				kubernetes.CriContainerNamespaceLabel: "namespace",
			},
		},
		Image: workloadmeta.ContainerImage{
			ID:        "12345",
			Name:      "busybox",
			ShortName: "busybox",
			Tag:       "latest",
			RawName:   "busybox:latest",
		},
		Runtime: "docker",
		Owner: &workloadmeta.EntityID{
			Kind: "kubernetes_pod",
			ID:   "uniqueIdentifier",
		},
		Ports:   []workloadmeta.ContainerPort{},
		EnvVars: map[string]string{},
		State: workloadmeta.ContainerState{
			Health: "unhealthy", // Ephemeral containers are not ready
		},
	}

	expectedInitContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "init-containerID",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "init-container-name",
			Labels: map[string]string{
				kubernetes.CriContainerNamespaceLabel: "namespace",
			},
		},
		Image: workloadmeta.ContainerImage{
			ID:        "sha256:abcd1234",
			Name:      "busybox",
			ShortName: "busybox",
			Tag:       "latest",
			RawName:   "busybox:latest",
		},
		Runtime: "docker",
		Owner: &workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "uniqueIdentifier",
		},
		Ports:   []workloadmeta.ContainerPort{},
		EnvVars: map[string]string{},
		State: workloadmeta.ContainerState{
			Health: "unhealthy",
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
			},
		},
		EphemeralContainers: []workloadmeta.OrchestratorContainer{
			{
				Name: "ephemeral-container",
				ID:   "ephemeral-container-id",
				Image: workloadmeta.ContainerImage{
					ID:        "12345",
					Name:      "busybox",
					ShortName: "busybox",
					Tag:       "latest",
					RawName:   "busybox:latest",
				},
			},
		},
		InitContainers: []workloadmeta.OrchestratorContainer{
			{
				Name: "init-container-name",
				ID:   "init-containerID",
				Image: workloadmeta.ContainerImage{
					ID:        "sha256:abcd1234",
					Name:      "busybox",
					ShortName: "busybox",
					Tag:       "latest",
					RawName:   "busybox:latest",
				},
			},
		},
		PersistentVolumeClaimNames: []string{"pvcName"},
		Ready:                      true,
		IP:                         "127.0.0.1",
		PriorityClass:              "priorityClass",
		GPUVendorList:              []string{"nvidia"},
		QOSClass:                   "Guaranteed",
		CreationTimestamp:          creationTimestamp,
		StartTime:                  &startTime,
		NodeName:                   "test-node",
		HostIP:                     "192.168.1.10",
		HostNetwork:                true,
		InitContainerStatuses: []workloadmeta.KubernetesContainerStatus{
			{
				ContainerID:  "docker://init-containerID",
				Name:         "init-container-name",
				Image:        "busybox:latest",
				ImageID:      "sha256:abcd1234",
				Ready:        false,
				RestartCount: 0,
			},
		},
		ContainerStatuses: []workloadmeta.KubernetesContainerStatus{
			{
				ContainerID:  "docker://containerID",
				Name:         "nginx-container",
				Image:        "nginx:1.25.2",
				ImageID:      "5dbe7e1b6b9c",
				Ready:        true,
				RestartCount: 1,
			},
		},
		Conditions: []workloadmeta.KubernetesPodCondition{
			{
				Type:   "Ready",
				Status: "True",
			},
		},
		Volumes: []workloadmeta.KubernetesPodVolume{
			{
				Name: "pvcVol",
				PersistentVolumeClaim: &workloadmeta.KubernetesPersistentVolumeClaim{
					ClaimName: "pvcName",
					ReadOnly:  false,
				},
			},
		},
		Tolerations: []workloadmeta.KubernetesPodToleration{
			{
				Key:               "node.kubernetes.io/not-ready",
				Operator:          "Exists",
				Effect:            "NoExecute",
				TolerationSeconds: pointer.Ptr(int64(300)),
			},
		},
		Reason: "SomeReason",
	}

	expectedEntities := []workloadmeta.Entity{
		expectedInitContainer,
		expectedContainer,
		expectedEphemeralContainer,
		expectedPod,
	}

	assert.ElementsMatch(t, expectedEntities, parsedEntities)
}
