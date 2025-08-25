// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package builder

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestConvertWorkloadmetaPodToK8sPod(t *testing.T) {
	now := time.Now()

	wmetaPod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "12345",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "test-pod",
			Namespace: "default",
			Annotations: map[string]string{
				"annotation-key": "annotation-value",
			},
			Labels: map[string]string{
				"label-key": "label-value",
			},
		},
		Owners: []workloadmeta.KubernetesPodOwner{
			{
				Kind:       "ReplicaSet",
				Name:       "rs1",
				Controller: ptr.To(true),
			},
		},
		InitContainers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "init-container-id",
				Name: "init-container",
				Image: workloadmeta.ContainerImage{
					Name: "init-image",
				},
			},
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   "main-container-id",
				Name: "main-container",
				Image: workloadmeta.ContainerImage{
					Name: "main-image",
				},
			},
		},
		Phase:         "Running",
		IP:            "10.0.0.1",
		PriorityClass: "high-priority",
		QOSClass:      "BestEffort",
		RuntimeClass:  "runtime-class",

		SecurityContext: &workloadmeta.PodSecurityContext{
			RunAsUser:  1000,
			RunAsGroup: 2000,
			FsGroup:    3000,
		},
		CreationTimestamp: now,
		HostNetwork:       true,
		NodeName:          "node1",
		Volumes: []workloadmeta.KubernetesPodVolume{
			{
				Name: "volume1",
				PersistentVolumeClaim: &workloadmeta.KubernetesPersistentVolumeClaim{
					ClaimName: "some-claim",
					ReadOnly:  true,
				},
			},
		},
		Tolerations: []workloadmeta.KubernetesPodToleration{
			{
				Key:      "key1",
				Operator: "Exists",
				Effect:   "NoSchedule",
			},
		},
		HostIP:    "192.168.1.1",
		StartTime: &now,
		Reason:    "Started",
		Conditions: []workloadmeta.KubernetesPodCondition{
			{
				Type:   "Ready",
				Status: "True",
			},
		},
		InitContainerStatuses: []workloadmeta.KubernetesContainerStatus{
			{
				Name:  "init-container",
				Image: "init-image",
				Ready: true,
			},
		},
		ContainerStatuses: []workloadmeta.KubernetesContainerStatus{
			{
				Name:  "main-container",
				Image: "main-image",
				Ready: true,
				State: workloadmeta.KubernetesContainerState{
					Running: &workloadmeta.KubernetesContainerStateRunning{
						StartedAt: now,
					},
				},
			},
		},

		// Unused in conversion, but added here to make it explicit
		KubeServices:               nil,
		EphemeralContainers:        nil,
		FinishedAt:                 time.Time{},
		GPUVendorList:              nil,
		NamespaceAnnotations:       nil,
		NamespaceLabels:            nil,
		PersistentVolumeClaimNames: nil,
		Ready:                      true,
	}

	wmetaInitContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "init-container-id",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "init-container",
			Namespace: "default",
		},
	}

	wmetaContainer := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "main-container-id",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "main-container",
			Namespace: "default",
		},
		EnvVars: map[string]string{
			"SOME_ENV": "some_env_value",
		},
		Ports: []workloadmeta.ContainerPort{
			{
				Name:     "port",
				Port:     7777,
				Protocol: "TCP",
				HostPort: 8888,
			},
		},
		SecurityContext: &workloadmeta.ContainerSecurityContext{
			Capabilities: &workloadmeta.Capabilities{
				Add:  []string{"CAP_SYS_ADMIN"},
				Drop: []string{"CAP_NET_RAW"},
			},
			Privileged: true,
			SeccompProfile: &workloadmeta.SeccompProfile{
				Type:             workloadmeta.SeccompProfileTypeRuntimeDefault,
				LocalhostProfile: "localhost-profile",
			},
		},
		ReadinessProbe: &workloadmeta.ContainerProbe{
			InitialDelaySeconds: 10,
		},
		Resources: workloadmeta.ContainerResources{
			CPURequest:    ptr.To(10.0),              // 10% = 0.1 cores = 100m
			MemoryRequest: ptr.To(uint64(209715200)), // 200Mi in bytes
			CPULimit:      ptr.To(20.0),              // 20% = 0.2 cores = 200m
			MemoryLimit:   ptr.To(uint64(419430400)), // 400Mi in bytes
		},
	}

	expectedPod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind: "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pod",
			UID:               types.UID("12345"),
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(now),
			Annotations: map[string]string{
				"annotation-key": "annotation-value",
			},
			Labels: map[string]string{
				"label-key": "label-value",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "ReplicaSet",
					Name:       "rs1",
					Controller: ptr.To(true),
				},
			},
		},
		Spec: corev1.PodSpec{
			HostNetwork: true,
			NodeName:    "node1",
			InitContainers: []corev1.Container{
				{
					Name:  "init-container",
					Image: "init-image",
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "main-container",
					Image: "main-image",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 7777,
							HostPort:      8888,
							Name:          "port",
							Protocol:      "TCP",
						},
					},
					ReadinessProbe: &corev1.Probe{
						InitialDelaySeconds: 10,
					},
					Env: []corev1.EnvVar{
						{
							Name:  "SOME_ENV",
							Value: "some_env_value",
						},
					},
					SecurityContext: &corev1.SecurityContext{
						Capabilities: &corev1.Capabilities{
							Add:  []corev1.Capability{"CAP_SYS_ADMIN"},
							Drop: []corev1.Capability{"CAP_NET_RAW"},
						},
						Privileged: ptr.To(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type:             corev1.SeccompProfileTypeRuntimeDefault,
							LocalhostProfile: ptr.To("localhost-profile"),
						},
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"cpu":    resource.MustParse("100m"),
							"memory": resource.MustParse("200Mi"),
						},
						Limits: corev1.ResourceList{
							"cpu":    resource.MustParse("200m"),
							"memory": resource.MustParse("400Mi"),
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "volume1",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "some-claim",
							ReadOnly:  true,
						},
					},
				},
			},
			PriorityClassName: "high-priority",
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser:  ptr.To[int64](1000),
				RunAsGroup: ptr.To[int64](2000),
				FSGroup:    ptr.To[int64](3000),
			},
			RuntimeClassName: ptr.To("runtime-class"),
			Tolerations: []corev1.Toleration{
				{
					Key:      "key1",
					Operator: "Exists",
					Effect:   "NoSchedule",
				},
			},
		},
		Status: corev1.PodStatus{
			Phase:  corev1.PodPhase("Running"),
			HostIP: "192.168.1.1",
			PodIP:  "10.0.0.1",
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "main-container",
					Image: "main-image",
					Ready: true,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{
							StartedAt: metav1.Time{
								Time: now,
							},
						},
					},
				},
			},
			InitContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "init-container",
					Image: "init-image",
					Ready: true,
				},
			},
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodConditionType("Ready"),
					Status: corev1.ConditionStatus("True"),
				},
			},
			QOSClass: corev1.PodQOSClass("BestEffort"),
			StartTime: &metav1.Time{
				Time: now,
			},
			Reason: "Started",
		},
	}

	wmetaMock := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	wmetaEntities := []workloadmeta.Entity{wmetaInitContainer, wmetaContainer, wmetaInitContainer}
	for _, entity := range wmetaEntities {
		wmetaMock.Set(entity)
	}

	actualPod := convertWorkloadmetaPodToK8sPod(wmetaPod, wmetaMock)

	// Compare Resources separately first using Quantity.Equal()
	// assert.Equal doesn't handle kubernetes Quantity well
	actualResources := actualPod.Spec.Containers[0].Resources
	expectedResources := expectedPod.Spec.Containers[0].Resources

	assert.True(t, expectedResources.Requests[corev1.ResourceCPU].Equal(actualResources.Requests[corev1.ResourceCPU]))
	assert.True(t, expectedResources.Requests[corev1.ResourceMemory].Equal(actualResources.Requests[corev1.ResourceMemory]))
	assert.True(t, expectedResources.Limits[corev1.ResourceCPU].Equal(actualResources.Limits[corev1.ResourceCPU]))
	assert.True(t, expectedResources.Limits[corev1.ResourceMemory].Equal(actualResources.Limits[corev1.ResourceMemory]))

	// Compare everything except Resources
	for i := range expectedPod.Spec.Containers {
		expectedPod.Spec.Containers[i].Resources = corev1.ResourceRequirements{}
	}
	for i := range actualPod.Spec.Containers {
		actualPod.Spec.Containers[i].Resources = corev1.ResourceRequirements{}
	}
	assert.Equal(t, expectedPod, actualPod)
}
