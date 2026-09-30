// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build nodefilter

package nodefilter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestParsePod(t *testing.T) {
	creationTimestamp := time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)
	conditionTransitionTime := creationTimestamp.Add(30 * time.Second)
	controller := true

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pod",
			Namespace:         "test-namespace",
			UID:               types.UID("pod-uid"),
			CreationTimestamp: metav1.NewTime(creationTimestamp),
			Annotations:       map[string]string{"annotationKey": "annotationValue"},
			Labels:            map[string]string{"labelKey": "labelValue"},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "deployment-hashrs",
					UID:        types.UID("owner-uid"),
					Controller: &controller,
				},
			},
		},
		Spec: corev1.PodSpec{
			NodeName:          "test-node",
			HostNetwork:       true,
			PriorityClassName: "priorityClass",
			Volumes: []corev1.Volume{
				{
					Name: "pvcVol",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "pvcName",
						},
					},
				},
			},
			InitContainers: []corev1.Container{
				{Name: "init-container-name", Image: "busybox:latest"},
			},
			Containers: []corev1.Container{
				{
					Name:  "nginx-container",
					Image: "nginx:1.25.2",
					Env: []corev1.EnvVar{
						{Name: "DD_ENV", Value: "prod"},
						{Name: "DD_SERVICE", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("1"),
						},
					},
					ResizePolicy: []corev1.ContainerResizePolicy{
						{ResourceName: corev1.ResourceCPU, RestartPolicy: corev1.NotRequired},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase:    corev1.PodRunning,
			HostIP:   "192.168.1.10",
			PodIP:    "127.0.0.1",
			QOSClass: corev1.PodQOSGuaranteed,
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodReady,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(conditionTransitionTime),
				},
			},
			InitContainerStatuses: []corev1.ContainerStatus{
				{
					Name:        "init-container-name",
					Image:       "busybox:latest",
					ImageID:     "sha256:abcd1234",
					ContainerID: "docker://init-containerID",
					State:       corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}},
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:        "nginx-container",
					Image:       "nginx:1.25.2",
					ImageID:     "5dbe7e1b6b9c",
					ContainerID: "docker://containerID",
					Ready:       true,
					State:       corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				},
			},
		},
	}

	events := parsePod(pod)

	entities := make(map[workloadmeta.Kind][]workloadmeta.Entity)
	for _, event := range events {
		assert.Equal(t, workloadmeta.EventTypeSet, event.Type)
		assert.Equal(t, workloadmeta.SourceNodeOrchestrator, event.Source)
		entities[event.Entity.GetID().Kind] = append(entities[event.Entity.GetID().Kind], event.Entity)
	}

	require.Len(t, entities[workloadmeta.KindContainer], 2)
	require.Len(t, entities[workloadmeta.KindKubernetesPod], 1)

	podEntity := entities[workloadmeta.KindKubernetesPod][0].(*workloadmeta.KubernetesPod)
	podEntityID := workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-uid"}

	assert.Equal(t, "pod-uid", podEntity.ID)
	assert.Equal(t, "test-pod", podEntity.Name)
	assert.Equal(t, "test-namespace", podEntity.Namespace)
	assert.Equal(t, map[string]string{"annotationKey": "annotationValue"}, podEntity.Annotations)
	assert.Equal(t, map[string]string{"labelKey": "labelValue"}, podEntity.Labels)
	assert.True(t, podEntity.Ready)
	require.NotNil(t, podEntity.ReadyTimestamp)
	assert.Equal(t, conditionTransitionTime, *podEntity.ReadyTimestamp)
	assert.Nil(t, podEntity.DeletionTimestamp)
	assert.Equal(t, "Running", podEntity.Phase)
	assert.Equal(t, "127.0.0.1", podEntity.IP)
	assert.Equal(t, "192.168.1.10", podEntity.HostIP)
	assert.True(t, podEntity.HostNetwork)
	assert.Equal(t, "priorityClass", podEntity.PriorityClass)
	assert.Equal(t, "Guaranteed", podEntity.QOSClass)
	assert.Equal(t, "test-node", podEntity.NodeName)
	assert.Equal(t, []string{"nvidia"}, podEntity.GPUVendorList)
	assert.Equal(t, []string{"pvcName"}, podEntity.PersistentVolumeClaimNames)
	assert.Equal(t, creationTimestamp, podEntity.CreationTimestamp)
	assert.Equal(t, []workloadmeta.KubernetesPodOwner{
		{Kind: "ReplicaSet", Name: "deployment-hashrs", ID: "owner-uid", Group: "apps", Controller: &controller},
	}, podEntity.Owners)

	require.Len(t, podEntity.Containers, 1)
	assert.Equal(t, "containerID", podEntity.Containers[0].ID)
	assert.Equal(t, workloadmeta.ContainerResources{GPUVendorList: []string{"nvidia"}}, podEntity.Containers[0].Resources)
	assert.Equal(t, workloadmeta.ContainerResizePolicy{CPURestartPolicy: string(corev1.NotRequired)}, podEntity.Containers[0].ResizePolicy)

	require.Len(t, podEntity.InitContainers, 1)
	assert.Equal(t, "init-containerID", podEntity.InitContainers[0].ID)

	var container *workloadmeta.Container
	for _, e := range entities[workloadmeta.KindContainer] {
		c := e.(*workloadmeta.Container)
		if c.ID == "containerID" {
			container = c
		}
	}
	require.NotNil(t, container)
	assert.Equal(t, "nginx-container", container.Name)
	assert.Equal(t, workloadmeta.ContainerRuntime("docker"), container.Runtime)
	assert.Equal(t, map[string]string{"DD_ENV": "prod"}, container.EnvVars)
	assert.True(t, container.State.Running)
	assert.Equal(t, workloadmeta.ContainerStatusRunning, container.State.Status)
	assert.Equal(t, workloadmeta.ContainerHealthHealthy, container.State.Health)
	assert.Equal(t, &podEntityID, container.Owner)
	assert.Equal(t, workloadmeta.ContainerImage{
		ID:        "5dbe7e1b6b9c",
		RawName:   "nginx:1.25.2",
		Name:      "nginx",
		ShortName: "nginx",
		Tag:       "1.25.2",
	}, container.Image)
}

func TestParsePod_SkipsContainerWithoutID(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", UID: types.UID("pod-uid")},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "not-yet-created"}},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "not-yet-created", ContainerID: ""},
			},
		},
	}

	events := parsePod(pod)

	for _, event := range events {
		assert.NotEqual(t, workloadmeta.KindContainer, event.Entity.GetID().Kind)
	}
}

func TestParsePod_DeletionTimestamp(t *testing.T) {
	deletionTime := metav1.NewTime(time.Date(2025, time.January, 2, 0, 0, 0, 0, time.UTC))
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pod",
			UID:               types.UID("pod-uid"),
			DeletionTimestamp: &deletionTime,
		},
	}

	events := parsePod(pod)

	require.Len(t, events, 1)
	podEntity := events[0].Entity.(*workloadmeta.KubernetesPod)
	require.NotNil(t, podEntity.DeletionTimestamp)
	assert.Equal(t, deletionTime.Time, *podEntity.DeletionTimestamp)
}

func TestExtractResizePolicy(t *testing.T) {
	spec := &corev1.Container{
		ResizePolicy: []corev1.ContainerResizePolicy{
			{ResourceName: corev1.ResourceCPU, RestartPolicy: corev1.RestartContainer},
			{ResourceName: corev1.ResourceMemory, RestartPolicy: corev1.NotRequired},
		},
	}

	assert.Equal(t, workloadmeta.ContainerResizePolicy{
		CPURestartPolicy:    string(corev1.RestartContainer),
		MemoryRestartPolicy: string(corev1.NotRequired),
	}, extractResizePolicy(spec))
}

func TestGpuVendorsFromLimits(t *testing.T) {
	limits := corev1.ResourceList{
		"nvidia.com/gpu": resource.MustParse("1"),
		"amd.com/gpu":    resource.MustParse("1"),
		"cpu":            resource.MustParse("100m"),
	}

	vendors := gpuVendorsFromLimits(limits)
	assert.ElementsMatch(t, []string{"nvidia", "amd"}, vendors)
}
