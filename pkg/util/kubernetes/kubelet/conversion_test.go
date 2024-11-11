// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && kubeapiserver

package kubelet

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

func TestConvertKubeletPodToK8sPod(t *testing.T) {
	now := time.Now()

	pod := &Pod{
		Metadata: PodMetadata{
			Name:              "test-pod",
			UID:               "12345",
			Namespace:         "default",
			CreationTimestamp: now,
			Annotations: map[string]string{
				"annotation-key": "annotation-value",
			},
			Labels: map[string]string{
				"label-key": "label-value",
			},
			Owners: []PodOwner{
				{
					Kind:       "ReplicaSet",
					Name:       "rs1",
					Controller: ptr.To(true),
				},
			},
		},
		Spec: Spec{
			HostNetwork: true,
			NodeName:    "node1",
			InitContainers: []ContainerSpec{
				{
					Name:  "init-container",
					Image: "init-image",
				},
			},
			Containers: []ContainerSpec{
				{
					Name:  "main-container",
					Image: "main-image",
					Ports: []ContainerPortSpec{
						{
							ContainerPort: 7777,
							HostPort:      8888,
							Name:          "port",
							Protocol:      "TCP",
						},
					},
					ReadinessProbe: &ContainerProbe{
						InitialDelaySeconds: 10,
					},
					Env: []EnvVar{
						{
							Name:  "SOME_ENV",
							Value: "some_env_value",
						},
					},
					SecurityContext: &ContainerSecurityContextSpec{
						Capabilities: &CapabilitiesSpec{
							Add:  []string{"CAP_SYS_ADMIN"},
							Drop: []string{"CAP_NET_RAW"},
						},
						Privileged: ptr.To(true),
						SeccompProfile: &SeccompProfileSpec{
							Type:             SeccompProfileTypeRuntimeDefault,
							LocalhostProfile: ptr.To("localhost-profile"),
						},
					},
					Resources: &ContainerResourcesSpec{
						Requests: ResourceList{
							"cpu":    resource.MustParse("100m"),
							"memory": resource.MustParse("200Mi"),
						},
						Limits: ResourceList{
							"cpu":    resource.MustParse("200m"),
							"memory": resource.MustParse("400Mi"),
						},
					},
				},
			},
			Volumes: []VolumeSpec{
				{
					Name: "volume1",
					PersistentVolumeClaim: &PersistentVolumeClaimSpec{
						ClaimName: "some-claim",
						ReadOnly:  true,
					},
				},
			},
			PriorityClassName: "high-priority",
			SecurityContext: &PodSecurityContextSpec{
				RunAsUser:  1000,
				RunAsGroup: 2000,
				FsGroup:    3000,
			},
			RuntimeClassName: ptr.To("runtime-class"),
			Tolerations: []Toleration{
				{
					Key:      "key1",
					Operator: "Exists",
					Effect:   "NoSchedule",
				},
			},
		},
		Status: Status{
			Phase:  "Running",
			HostIP: "192.168.1.1",
			PodIP:  "10.0.0.1",
			Containers: []ContainerStatus{
				{
					Name:  "main-container",
					Image: "main-image",
					Ready: true,
					State: ContainerState{
						Running: &ContainerStateRunning{
							StartedAt: now,
						},
					},
				},
			},
			InitContainers: []ContainerStatus{
				{
					Name:  "init-container",
					Image: "init-image",
					Ready: true,
				},
			},
			Conditions: []Conditions{
				{
					Type:   "Ready",
					Status: "True",
				},
			},
			QOSClass:  "BestEffort",
			StartTime: now,
			Reason:    "Started",
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

	assert.Equal(t, expectedPod, ConvertKubeletPodToK8sPod(pod))
}
