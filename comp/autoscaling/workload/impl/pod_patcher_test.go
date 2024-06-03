// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package impl

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-agent/comp/autoscaling/workload/impl/model"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
)

func patcherTestStoreWithData() *store {
	store := autoscaling.NewStore[model.PodAutoscalerInternal]()

	// ns1/autoscaler1 targets "test-deployment" and has vertical recommendations for 2 containers and from automatic source
	store.Set("ns1/autoscaler1", model.PodAutoscalerInternal{
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
				Source:        datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
				ResourcesHash: "version1",
				ContainerResources: []datadoghq.DatadogPodAutoscalerContainerResources{
					{Name: "container1", Limits: corev1.ResourceList{"cpu": resource.MustParse("500m")}, Requests: corev1.ResourceList{"memory": resource.MustParse("256Mi")}},
					{Name: "container2", Limits: corev1.ResourceList{"cpu": resource.MustParse("600m")}, Requests: corev1.ResourceList{"memory": resource.MustParse("512Mi")}},
				},
			},
		},
	}, "")
	// ns1/autoscaler2 has a custom owner reference and no vertical recommendations
	store.Set("ns1/autoscaler2", model.PodAutoscalerInternal{
		Namespace: "ns1",
		Name:      "autoscaler1",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "Custom",
				APIVersion: "foo.com/v1",
				Name:       "test",
			},
		},
	}, "")

	// In ns2, autoscaler1 and autoscaler2 target the same RS "duplicate-target"
	store.Set("ns2/autoscaler1", model.PodAutoscalerInternal{
		Namespace: "ns2",
		Name:      "autoscaler1",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "ReplicaSet",
				APIVersion: "apps/v1",
				Name:       "duplicate-target",
			},
		},
	}, "")
	store.Set("ns2/autoscaler2", model.PodAutoscalerInternal{
		Namespace: "ns2",
		Name:      "autoscaler2",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "ReplicaSet",
				APIVersion: "apps/v1",
				Name:       "duplicate-target",
			},
		},
	}, "")

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
			name: "no update on empty recommendations",
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
			wantErr: false,
			wantPod: corev1.Pod{
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := patcherTestStoreWithData()
			patcherAdapter := newPodPatcher(store, nil, nil, nil)

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
