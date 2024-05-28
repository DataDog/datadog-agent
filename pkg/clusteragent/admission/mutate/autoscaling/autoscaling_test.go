// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test && kubeapiserver

package autoscaling

import (
	"fmt"
	"reflect"
	"testing"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

type fakeRecommender struct {
	recID           string
	recommendations map[string][]datadoghq.DatadogPodAutoscalerContainerResources
	err             error
}

// GetRecommendations returns recommendations
func (f *fakeRecommender) GetRecommendations(_ string, ownerRef metav1.OwnerReference) (string, []datadoghq.DatadogPodAutoscalerContainerResources, error) {
	if recs, ok := f.recommendations[ownerRef.Name]; ok {
		return f.recID, recs, nil
	}

	return "", nil, f.err
}

func TestUpdateResources(t *testing.T) {
	tests := []struct {
		name         string
		wh           *Webhook
		pod          corev1.Pod
		wantInjected bool
		wantErr      bool
		wantPod      corev1.Pod
	}{
		{
			name: "update resources when recommendations differ",
			wh: &Webhook{
				recommender: &fakeRecommender{
					recID: "version1",
					recommendations: map[string][]datadoghq.DatadogPodAutoscalerContainerResources{
						"test-deployment": {
							{Name: "container1", Limits: corev1.ResourceList{"cpu": resource.MustParse("500m")}, Requests: corev1.ResourceList{"memory": resource.MustParse("256Mi")}},
						},
					},
				},
			},
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{{
						Kind: "ReplicaSet",
						Name: "test-deployment-968f49d86",
					}},
					Annotations: map[string]string{recommendationIDAnotation: "version0"},
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
					OwnerReferences: []metav1.OwnerReference{{
						Kind: "ReplicaSet",
						Name: "test-deployment-968f49d86",
					}},
					Annotations: map[string]string{recommendationIDAnotation: "version1"},
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
			name: "update resources when there are none",
			wh: &Webhook{
				recommender: &fakeRecommender{
					recID: "version0",
					recommendations: map[string][]datadoghq.DatadogPodAutoscalerContainerResources{
						"test-deployment": {
							{Name: "container1", Limits: corev1.ResourceList{"cpu": resource.MustParse("500m")}, Requests: corev1.ResourceList{"memory": resource.MustParse("256Mi")}},
						},
					},
				},
			},
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{{
						Kind: "ReplicaSet",
						Name: "test-deployment-968f49d86",
					}},
					Annotations: map[string]string{recommendationIDAnotation: "version0"},
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
					OwnerReferences: []metav1.OwnerReference{{
						Kind: "ReplicaSet",
						Name: "test-deployment-968f49d86",
					}},
					Annotations: map[string]string{recommendationIDAnotation: "version0"},
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
			wh: &Webhook{
				recommender: &fakeRecommender{
					recID: "version0",
					recommendations: map[string][]datadoghq.DatadogPodAutoscalerContainerResources{
						"test-deployment": {
							{Name: "container1", Limits: corev1.ResourceList{"cpu": resource.MustParse("200m")}, Requests: corev1.ResourceList{"memory": resource.MustParse("128Mi")}},
						},
					},
				},
			},
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{{
						Kind: "ReplicaSet",
						Name: "test-deployment-968f49d86",
					}},
					Annotations: map[string]string{recommendationIDAnotation: "version0"},
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
			wantPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{{
						Kind: "ReplicaSet",
						Name: "test-deployment-968f49d86",
					}},
					Annotations: map[string]string{recommendationIDAnotation: "version0"},
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
			name: "no update when pod has no owner",
			wh:   &Webhook{},
			pod: corev1.Pod{
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
			wantPod: corev1.Pod{
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
			wantErr: true,
		},
		{
			name: "no update when deployment can't be parsed",
			wh:   &Webhook{},
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{
					Kind: "ReplicaSet",
					Name: "test-deployment-notahash",
				}}},
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
			wantPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{
					Kind: "ReplicaSet",
					Name: "test-deployment-notahash",
				}}},
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
			wantErr: true,
		},
		{
			name: "no update on errors",
			wh: &Webhook{
				recommender: &fakeRecommender{
					err: fmt.Errorf("error"),
				},
			},
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{
					Kind: "ReplicaSet",
					Name: "test-deployment-968f49d86",
				}}},
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
			wantErr: true,
			wantPod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{
					Kind: "ReplicaSet",
					Name: "test-deployment-968f49d86",
				}}},
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
			name: "no update on empty recommendations",
			wh: &Webhook{
				recommender: &fakeRecommender{},
			},
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{
					Kind: "ReplicaSet",
					Name: "test-deployment-968f49d86",
				}}},
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
				ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{
					Kind: "ReplicaSet",
					Name: "test-deployment-968f49d86",
				}}},
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
			injected, err := tt.wh.updateResources(&tt.pod, tt.pod.Namespace, dynamic.Interface(nil))
			if (err != nil) != tt.wantErr {
				t.Errorf("updateResources() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.wantInjected, injected)
			if !reflect.DeepEqual(tt.pod, tt.wantPod) {
				t.Errorf("updateResources() pod = %v, wantPod %v", tt.pod, tt.wantPod)
			}
		})
	}
}
