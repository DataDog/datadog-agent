// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

func patcherTestStoreWithData() *store {
	store := autoscaling.NewStore[model.PodAutoscalerInternal]()

	// ns1/autoscaler1 targets "test-deployment" and has vertical recommendations for 2 containers and from automatic source
	store.Set("ns1/autoscaler1", model.FakePodAutoscalerInternal{
		Namespace: "ns1",
		Name:      "autoscaler1",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
				Name:       "test-deployment",
			},
		},
		ScalingValues: model.ScalingValues{
			VerticalError: errors.New("error on dd side"),
			Vertical: &model.VerticalScalingValues{
				Source:        datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				ResourcesHash: "version1",
				ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
					{Name: "container1", Limits: corev1.ResourceList{"cpu": resource.MustParse("500m")}, Requests: corev1.ResourceList{"memory": resource.MustParse("256Mi")}},
					{Name: "container2", Limits: corev1.ResourceList{"cpu": resource.MustParse("600m")}, Requests: corev1.ResourceList{"memory": resource.MustParse("512Mi")}},
				},
			},
		},
	}.Build(), "")
	// ns1/autoscaler2 has a custom owner reference and no vertical recommendations
	store.Set("ns1/autoscaler2", model.FakePodAutoscalerInternal{
		Namespace: "ns1",
		Name:      "autoscaler1",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "Custom",
				APIVersion: "foo.com/v1",
				Name:       "test",
			},
		},
	}.Build(), "")

	// ns1/autoscaler3 targets "test-sidecar-deployment" and has vertical recommendations for init sidecar container
	store.Set("ns1/autoscaler3", model.FakePodAutoscalerInternal{
		Namespace: "ns1",
		Name:      "autoscaler3",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
				Name:       "test-sidecar-deployment",
			},
		},
		ScalingValues: model.ScalingValues{
			Vertical: &model.VerticalScalingValues{
				Source:        datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				ResourcesHash: "sidecar-version1",
				ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
					{Name: "init-sidecar-container", Limits: corev1.ResourceList{"cpu": resource.MustParse("300m")}, Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")}},
				},
			},
		},
	}.Build(), "")

	// ns1/autoscaler4 targets "test-mixed-deployment" and has vertical recommendations for both sidecar and main containers
	store.Set("ns1/autoscaler4", model.FakePodAutoscalerInternal{
		Namespace: "ns1",
		Name:      "autoscaler4",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
				Name:       "test-mixed-deployment",
			},
		},
		ScalingValues: model.ScalingValues{
			Vertical: &model.VerticalScalingValues{
				Source:        datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				ResourcesHash: "mixed-version1",
				ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
					{Name: "init-sidecar-container", Limits: corev1.ResourceList{"cpu": resource.MustParse("400m")}, Requests: corev1.ResourceList{"memory": resource.MustParse("256Mi")}},
					{Name: "main-container", Limits: corev1.ResourceList{"cpu": resource.MustParse("800m")}, Requests: corev1.ResourceList{"memory": resource.MustParse("512Mi")}},
				},
			},
		},
	}.Build(), "")

	// In ns2, autoscaler1 and autoscaler2 target the same RS "duplicate-target"
	store.Set("ns2/autoscaler1", model.FakePodAutoscalerInternal{
		Namespace: "ns2",
		Name:      "autoscaler1",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "ReplicaSet",
				APIVersion: "apps/v1",
				Name:       "duplicate-target",
			},
		},
	}.Build(), "")
	store.Set("ns2/autoscaler2", model.FakePodAutoscalerInternal{
		Namespace: "ns2",
		Name:      "autoscaler2",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "ReplicaSet",
				APIVersion: "apps/v1",
				Name:       "duplicate-target",
			},
		},
	}.Build(), "")

	// In ns3, autoscaler1 targets the rollout "my-rollout"
	store.Set("ns3/autoscaler1", model.FakePodAutoscalerInternal{
		Namespace: "ns3",
		Name:      "autoscaler1",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "Rollout",
				APIVersion: "argoproj.io/v1alpha1",
				Name:       "my-rollout",
			},
		},
	}.Build(), "")

	// In ns4, autoscaler1 targets the statefulset "my-statefulset"
	store.Set("ns4/autoscaler1", model.FakePodAutoscalerInternal{
		Namespace: "ns4",
		Name:      "autoscaler1",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       kubernetes.StatefulSetKind,
				APIVersion: "apps/v1",
				Name:       "my-statefulset",
			},
		},
	}.Build(), "")

	return store
}

func TestPatcherApplyRecommendations(t *testing.T) {
	tests := []struct {
		name         string
		pod          corev1.Pod
		wantInjected bool
		wantErr      bool
		wantPod      corev1.Pod
	}{
		{
			name: "update resources when recommendations differ",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "ReplicaSet",
						Name:       "test-deployment-968f49d86",
						APIVersion: "apps/v1",
					}},
					Annotations: map[string]string{
						model.RecommendationIDAnnotation: "version0",
						model.AutoscalerIDAnnotation:     "ns1/autoscaler1",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "container1",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
						},
					}},
				},
			},
			wantInjected: true,
			wantPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "ReplicaSet",
						Name:       "test-deployment-968f49d86",
						APIVersion: "apps/v1",
					}},
					Annotations: map[string]string{
						model.RecommendationIDAnnotation: "version1",
						model.AutoscalerIDAnnotation:     "ns1/autoscaler1",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "container1",
							Resources: corev1.ResourceRequirements{
								Limits:   corev1.ResourceList{"cpu": resource.MustParse("500m")},
								Requests: corev1.ResourceList{"memory": resource.MustParse("256Mi")},
							},
						},
					},
				},
			},
		},
		{
			name: "update resources when there are none",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "ReplicaSet",
						Name:       "test-deployment-968f49d86",
						APIVersion: "apps/v1",
					}},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "container1",
					}},
				},
			},
			wantInjected: true,
			wantPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "ReplicaSet",
						Name:       "test-deployment-968f49d86",
						APIVersion: "apps/v1",
					}},
					Annotations: map[string]string{
						model.RecommendationIDAnnotation: "version1",
						model.AutoscalerIDAnnotation:     "ns1/autoscaler1",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "container1",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("500m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("256Mi")},
						},
					}},
				},
			},
		},
		{
			name: "no update when recommendations match",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "ReplicaSet",
						Name:       "test-deployment-968f49d86",
						APIVersion: "apps/v1",
					}},
					Annotations: map[string]string{
						model.RecommendationIDAnnotation: "version1",
						model.AutoscalerIDAnnotation:     "ns1/autoscaler1",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "container1",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("500m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("256Mi")},
						},
					}},
				},
			},
			wantInjected: false,
			wantPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "ReplicaSet",
						Name:       "test-deployment-968f49d86",
						APIVersion: "apps/v1",
					}},
					Annotations: map[string]string{
						model.RecommendationIDAnnotation: "version1",
						model.AutoscalerIDAnnotation:     "ns1/autoscaler1",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "container1",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("500m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("256Mi")},
						},
					}},
				},
			},
		},
		{
			name: "only autoscaler id on empty recommendations",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "Custom",
						APIVersion: "foo.com/v1",
						Name:       "test",
					}},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "container1",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
						},
					}},
				},
			},
			wantErr:      false,
			wantInjected: true,
			wantPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "Custom",
						APIVersion: "foo.com/v1",
						Name:       "test",
					}},
					Annotations: map[string]string{
						model.AutoscalerIDAnnotation: "ns1/autoscaler1",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "container1",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
						},
					}},
				},
			},
		},
		{
			name: "update init sidecar resources when recommendations differ",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "ReplicaSet",
						Name:       "test-sidecar-deployment-968f49d86",
						APIVersion: "apps/v1",
					}},
					Annotations: map[string]string{
						model.RecommendationIDAnnotation: "sidecar-version0",
						model.AutoscalerIDAnnotation:     "ns1/autoscaler3",
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name:          "init-sidecar-container",
						RestartPolicy: pointer.Ptr(corev1.ContainerRestartPolicyAlways),
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("100m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("64Mi")},
						},
					}},
					Containers: []corev1.Container{{
						Name: "main-container",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
						},
					}},
				},
			},
			wantInjected: true,
			wantPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "ReplicaSet",
						Name:       "test-sidecar-deployment-968f49d86",
						APIVersion: "apps/v1",
					}},
					Annotations: map[string]string{
						model.RecommendationIDAnnotation: "sidecar-version1",
						model.AutoscalerIDAnnotation:     "ns1/autoscaler3",
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:          "init-sidecar-container",
							RestartPolicy: pointer.Ptr(corev1.ContainerRestartPolicyAlways),
							Resources: corev1.ResourceRequirements{
								Limits:   corev1.ResourceList{"cpu": resource.MustParse("300m")},
								Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
							},
						},
					},
					Containers: []corev1.Container{{
						Name: "main-container",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
						},
					}},
				},
			},
		},
		{
			name: "update init sidecar resources when there are none",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "ReplicaSet",
						Name:       "test-sidecar-deployment-968f49d86",
						APIVersion: "apps/v1",
					}},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name:          "init-sidecar-container",
						RestartPolicy: pointer.Ptr(corev1.ContainerRestartPolicyAlways),
					}},
					Containers: []corev1.Container{{
						Name: "main-container",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
						},
					}},
				},
			},
			wantInjected: true,
			wantPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "ReplicaSet",
						Name:       "test-sidecar-deployment-968f49d86",
						APIVersion: "apps/v1",
					}},
					Annotations: map[string]string{
						model.RecommendationIDAnnotation: "sidecar-version1",
						model.AutoscalerIDAnnotation:     "ns1/autoscaler3",
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name:          "init-sidecar-container",
						RestartPolicy: pointer.Ptr(corev1.ContainerRestartPolicyAlways),
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("300m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
						},
					}},
					Containers: []corev1.Container{{
						Name: "main-container",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
						},
					}},
				},
			},
		},
		{
			name: "no update when init sidecar recommendations match",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "ReplicaSet",
						Name:       "test-sidecar-deployment-968f49d86",
						APIVersion: "apps/v1",
					}},
					Annotations: map[string]string{
						model.RecommendationIDAnnotation: "sidecar-version1",
						model.AutoscalerIDAnnotation:     "ns1/autoscaler3",
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name:          "init-sidecar-container",
						RestartPolicy: pointer.Ptr(corev1.ContainerRestartPolicyAlways),
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("300m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
						},
					}},
					Containers: []corev1.Container{{
						Name: "main-container",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
						},
					}},
				},
			},
			wantInjected: false,
			wantPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "ReplicaSet",
						Name:       "test-sidecar-deployment-968f49d86",
						APIVersion: "apps/v1",
					}},
					Annotations: map[string]string{
						model.RecommendationIDAnnotation: "sidecar-version1",
						model.AutoscalerIDAnnotation:     "ns1/autoscaler3",
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name:          "init-sidecar-container",
						RestartPolicy: pointer.Ptr(corev1.ContainerRestartPolicyAlways),
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("300m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
						},
					}},
					Containers: []corev1.Container{{
						Name: "main-container",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
						},
					}},
				},
			},
		},
		{
			name: "regular init containers ignored",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "ReplicaSet",
						Name:       "test-sidecar-deployment-968f49d86",
						APIVersion: "apps/v1",
					}},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name: "init-sidecar-container",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("100m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("64Mi")},
						},
					}},
					Containers: []corev1.Container{{
						Name: "main-container",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
						},
					}},
				},
			},
			wantInjected: true,
			wantPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "ReplicaSet",
						Name:       "test-sidecar-deployment-968f49d86",
						APIVersion: "apps/v1",
					}},
					Annotations: map[string]string{
						model.RecommendationIDAnnotation: "sidecar-version1",
						model.AutoscalerIDAnnotation:     "ns1/autoscaler3",
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name: "init-sidecar-container",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("100m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("64Mi")},
						},
					}},
					Containers: []corev1.Container{{
						Name: "main-container",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
						},
					}},
				},
			},
		},
		{
			name: "update both init sidecar and main container resources when recommendations differ",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "ReplicaSet",
						Name:       "test-mixed-deployment-968f49d86",
						APIVersion: "apps/v1",
					}},
					Annotations: map[string]string{
						model.RecommendationIDAnnotation: "mixed-version0",
						model.AutoscalerIDAnnotation:     "ns1/autoscaler4",
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name:          "init-sidecar-container",
						RestartPolicy: pointer.Ptr(corev1.ContainerRestartPolicyAlways),
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
						},
					}},
					Containers: []corev1.Container{{
						Name: "main-container",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("500m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("256Mi")},
						},
					}},
				},
			},
			wantInjected: true,
			wantPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{{
						Kind:       "ReplicaSet",
						Name:       "test-mixed-deployment-968f49d86",
						APIVersion: "apps/v1",
					}},
					Annotations: map[string]string{
						model.RecommendationIDAnnotation: "mixed-version1",
						model.AutoscalerIDAnnotation:     "ns1/autoscaler4",
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name:          "init-sidecar-container",
						RestartPolicy: pointer.Ptr(corev1.ContainerRestartPolicyAlways),
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("400m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("256Mi")},
						},
					}},
					Containers: []corev1.Container{{
						Name: "main-container",
						Resources: corev1.ResourceRequirements{
							Limits:   corev1.ResourceList{"cpu": resource.MustParse("800m")},
							Requests: corev1.ResourceList{"memory": resource.MustParse("512Mi")},
						},
					}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := patcherTestStoreWithData()
			patcherAdapter := NewPodPatcher(store, nil, nil, nil)

			injected, err := patcherAdapter.ApplyRecommendations(&tt.pod)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateResources() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.wantInjected, injected)
			assert.Empty(t, cmp.Diff(tt.pod, tt.wantPod), "Difference between expected POD and actual POD")
		})
	}
}

func TestFindAutoscaler(t *testing.T) {
	tests := []struct {
		name                 string
		pod                  *corev1.Pod
		expectedAutoscalerID string
		expectedError        error
	}{
		{
			name:                 "Pod without owner should return nil",
			pod:                  &corev1.Pod{},
			expectedAutoscalerID: "",
			expectedError:        nil,
		},
		{
			name: "Pod with directly deployment owner should return nil",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind:       "Deployment",
							Name:       "deployment",
							APIVersion: "apps/v1",
						},
					},
				},
			},
			expectedAutoscalerID: "",
			expectedError:        errDeploymentNotValidOwner,
		},
		{
			name: "Pod with owner but no matching autoscaler should return nil",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind:       "ReplicaSet",
							Name:       "rs1",
							APIVersion: "apps/v1",
						},
					},
				},
			},
			expectedAutoscalerID: "",
			expectedError:        nil,
		},
		{
			name: "Pod with owner and matching autoscaler should return the autoscaler",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns1",
					Name:      "pod1",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind:       "ReplicaSet",
							Name:       "test-deployment-968f49d86",
							APIVersion: "apps/v1",
						},
					},
				},
			},
			expectedAutoscalerID: "ns1/autoscaler1",
			expectedError:        nil,
		},
		{
			name: "Pod with owner and multiple matching autoscalers should return an error",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns2",
					Name:      "pod2",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind:       "ReplicaSet",
							Name:       "duplicate-target",
							APIVersion: "apps/v1",
						},
					},
				},
			},
			expectedAutoscalerID: "",
			expectedError:        errors.New("Multiple autoscaler found for POD ns2/pod2, ownerRef: ReplicaSet/duplicate-target, cannot update POD"),
		},
		{
			name: "Pod without owner",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns2",
					Name:      "pod2",
				},
			},
			expectedAutoscalerID: "",
			expectedError:        nil,
		},
		{
			name: "Pod owned by replicaset managed by rollout",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns3",
					Name:      "pod3",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind:       "ReplicaSet",
							Name:       "my-rollout-9b8dc4bd6",
							APIVersion: "apps/v1",
						},
					},
					Labels: map[string]string{
						kubernetes.ArgoRolloutLabelKey: "9b8dc4bd6",
					},
				},
			},
			expectedAutoscalerID: "ns3/autoscaler1",
			expectedError:        nil,
		},
		{
			name: "Pod owned by statefulset",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns4",
					Name:      "my-statefulset-0",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind:       kubernetes.StatefulSetKind,
							Name:       "my-statefulset",
							APIVersion: "apps/v1",
						},
					},
				},
			},
			expectedAutoscalerID: "ns4/autoscaler1",
			expectedError:        nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := patcherTestStoreWithData()
			patcherAdapter := podPatcher{store: store}

			result, err := patcherAdapter.findAutoscaler(tt.pod)
			if tt.expectedAutoscalerID == "" {
				if result != nil {
					t.Errorf("Expected no Autoscaler to be found, but found one with ID: %s", result.ID())
				}
			} else {
				require.NotNil(t, result, "Expected Autoscaler with id: %s to be found, but found none", tt.expectedAutoscalerID)
				assert.Equal(t, tt.expectedAutoscalerID, result.ID())
			}
			assert.Equal(t, tt.expectedError, err)
		})
	}
}

func TestPatchContainerResources(t *testing.T) {
	tests := []struct {
		name             string
		recommendation   datadoghqcommon.DatadogPodAutoscalerContainerResources
		container        *corev1.Container
		expectedPatched  bool
		expectedLimits   corev1.ResourceList
		expectedRequests corev1.ResourceList
	}{
		{
			name: "container with no existing resources gets recommendations applied",
			recommendation: datadoghqcommon.DatadogPodAutoscalerContainerResources{
				Name:     "test-container",
				Limits:   corev1.ResourceList{"cpu": resource.MustParse("500m"), "memory": resource.MustParse("512Mi")},
				Requests: corev1.ResourceList{"cpu": resource.MustParse("250m"), "memory": resource.MustParse("256Mi")},
			},
			container: &corev1.Container{
				Name:      "test-container",
				Resources: corev1.ResourceRequirements{},
			},
			expectedPatched:  true,
			expectedLimits:   corev1.ResourceList{"cpu": resource.MustParse("500m"), "memory": resource.MustParse("512Mi")},
			expectedRequests: corev1.ResourceList{"cpu": resource.MustParse("250m"), "memory": resource.MustParse("256Mi")},
		},
		{
			name: "container with different resources gets updated",
			recommendation: datadoghqcommon.DatadogPodAutoscalerContainerResources{
				Name:     "test-container",
				Limits:   corev1.ResourceList{"cpu": resource.MustParse("800m")},
				Requests: corev1.ResourceList{"memory": resource.MustParse("512Mi")},
			},
			container: &corev1.Container{
				Name: "test-container",
				Resources: corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{"cpu": resource.MustParse("500m")},
					Requests: corev1.ResourceList{"memory": resource.MustParse("256Mi")},
				},
			},
			expectedPatched:  true,
			expectedLimits:   corev1.ResourceList{"cpu": resource.MustParse("800m")},
			expectedRequests: corev1.ResourceList{"memory": resource.MustParse("512Mi")},
		},
		{
			name: "container with same resources as recommendation returns no patch",
			recommendation: datadoghqcommon.DatadogPodAutoscalerContainerResources{
				Name:     "test-container",
				Limits:   corev1.ResourceList{"cpu": resource.MustParse("500m")},
				Requests: corev1.ResourceList{"memory": resource.MustParse("256Mi")},
			},
			container: &corev1.Container{
				Name: "test-container",
				Resources: corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{"cpu": resource.MustParse("500m")},
					Requests: corev1.ResourceList{"memory": resource.MustParse("256Mi")},
				},
			},
			expectedPatched:  false,
			expectedLimits:   corev1.ResourceList{"cpu": resource.MustParse("500m")},
			expectedRequests: corev1.ResourceList{"memory": resource.MustParse("256Mi")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			containerCopy := tt.container.DeepCopy()

			patched := patchContainerResources(tt.recommendation, containerCopy)

			assert.Equal(t, tt.expectedPatched, patched, "patchContainerResources should return expected patch status")
			assert.Equal(t, tt.expectedLimits, containerCopy.Resources.Limits, "Container limits should match expected values")
			assert.Equal(t, tt.expectedRequests, containerCopy.Resources.Requests, "Container requests should match expected values")
		})
	}
}

func TestPatchPod(t *testing.T) {
	tests := []struct {
		name             string
		recommendation   datadoghqcommon.DatadogPodAutoscalerContainerResources
		pod              *corev1.Pod
		expectedPatched  bool
		expectedLimits   corev1.ResourceList
		expectedRequests corev1.ResourceList
	}{
		{
			name: "patch regular container with matching name",
			recommendation: datadoghqcommon.DatadogPodAutoscalerContainerResources{
				Name:     "app-container",
				Limits:   corev1.ResourceList{"cpu": resource.MustParse("500m")},
				Requests: corev1.ResourceList{"memory": resource.MustParse("256Mi")},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app-container",
							Resources: corev1.ResourceRequirements{
								Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
								Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
							},
						},
						{
							Name: "sidecar-container",
							Resources: corev1.ResourceRequirements{
								Limits:   corev1.ResourceList{"cpu": resource.MustParse("100m")},
								Requests: corev1.ResourceList{"memory": resource.MustParse("64Mi")},
							},
						},
					},
				},
			},
			expectedPatched:  true,
			expectedLimits:   corev1.ResourceList{"cpu": resource.MustParse("500m")},
			expectedRequests: corev1.ResourceList{"memory": resource.MustParse("256Mi")},
		},
		{
			name: "patch init sidecar container with restartPolicy Always",
			recommendation: datadoghqcommon.DatadogPodAutoscalerContainerResources{
				Name:     "init-sidecar",
				Limits:   corev1.ResourceList{"cpu": resource.MustParse("300m")},
				Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app-container",
							Resources: corev1.ResourceRequirements{
								Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
								Requests: corev1.ResourceList{"memory": resource.MustParse("64Mi")},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:          "init-sidecar",
							RestartPolicy: pointer.Ptr(corev1.ContainerRestartPolicyAlways),
							Resources: corev1.ResourceRequirements{
								Limits:   corev1.ResourceList{"cpu": resource.MustParse("100m")},
								Requests: corev1.ResourceList{"memory": resource.MustParse("64Mi")},
							},
						},
					},
				},
			},
			expectedPatched:  true,
			expectedLimits:   corev1.ResourceList{"cpu": resource.MustParse("300m")},
			expectedRequests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
		},
		{
			name: "regular init container is ignored",
			recommendation: datadoghqcommon.DatadogPodAutoscalerContainerResources{
				Name:     "init-container",
				Limits:   corev1.ResourceList{"cpu": resource.MustParse("300m")},
				Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app-container",
							Resources: corev1.ResourceRequirements{
								Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
								Requests: corev1.ResourceList{"memory": resource.MustParse("64Mi")},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name: "init-container",
							Resources: corev1.ResourceRequirements{
								Limits:   corev1.ResourceList{"cpu": resource.MustParse("100m")},
								Requests: corev1.ResourceList{"memory": resource.MustParse("64Mi")},
							},
						},
					},
				},
			},
			expectedPatched: false,
		},
		{
			name: "no matching container name returns false",
			recommendation: datadoghqcommon.DatadogPodAutoscalerContainerResources{
				Name:     "non-existent-container",
				Limits:   corev1.ResourceList{"cpu": resource.MustParse("500m")},
				Requests: corev1.ResourceList{"memory": resource.MustParse("256Mi")},
			},
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app-container",
							Resources: corev1.ResourceRequirements{
								Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
								Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")},
							},
						},
					},
				},
			},
			expectedPatched: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podCopy := tt.pod.DeepCopy()

			patched := patchPod(tt.recommendation, podCopy)

			assert.Equal(t, tt.expectedPatched, patched, "patchPod should return expected patch status")

			if tt.expectedPatched {
				var foundContainer *corev1.Container
				for i := range podCopy.Spec.Containers {
					if podCopy.Spec.Containers[i].Name == tt.recommendation.Name {
						foundContainer = &podCopy.Spec.Containers[i]
						break
					}
				}

				if foundContainer == nil {
					for i := range podCopy.Spec.InitContainers {
						cont := &podCopy.Spec.InitContainers[i]
						isInitSidecarContainer := cont.RestartPolicy != nil && *cont.RestartPolicy == corev1.ContainerRestartPolicyAlways
						if cont.Name == tt.recommendation.Name && isInitSidecarContainer {
							foundContainer = cont
							break
						}
					}
				}

				require.NotNil(t, foundContainer, "Should have found the patched container")
				assert.Equal(t, tt.expectedLimits, foundContainer.Resources.Limits, "Container limits should match expected values")
				assert.Equal(t, tt.expectedRequests, foundContainer.Resources.Requests, "Container requests should match expected values")
			}
		})
	}
}
