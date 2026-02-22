// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestValidateAutoscalerSpec(t *testing.T) {
	tests := map[string]struct {
		spec    datadoghq.DatadogPodAutoscalerSpec
		wantErr string
	}{
		// Objective type-to-payload checks
		"custom query objective missing payload": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
					{Type: datadoghqcommon.DatadogPodAutoscalerCustomQueryObjectiveType},
				},
			},
			wantErr: "Autoscaler objective type is custom query but customQueryObjective is nil",
		},
		"pod resource type without resource": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
					{Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType},
				},
			},
			wantErr: "autoscaler objective type is PodResource but podResource is nil",
		},
		"container resource type without resource": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
					{Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType},
				},
			},
			wantErr: "autoscaler objective type is ContainerResource but containerResource is nil",
		},
		"custom query objective with pod resource also set": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
					{
						Type:        datadoghqcommon.DatadogPodAutoscalerCustomQueryObjectiveType,
						CustomQuery: &datadoghqcommon.DatadogPodAutoscalerCustomQueryObjective{},
						PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{},
					},
				},
			},
		},
		"pod resource type with custom query also set": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
					{
						Type:        datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
						PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{},
						CustomQuery: &datadoghqcommon.DatadogPodAutoscalerCustomQueryObjective{},
					},
				},
			},
		},
		"container resource type with custom query also set": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
					{
						Type:              datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
						ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{},
						CustomQuery:       &datadoghqcommon.DatadogPodAutoscalerCustomQueryObjective{},
					},
				},
			},
		},
		"valid pod resource objective": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
					{
						Type:        datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
						PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{},
					},
				},
			},
		},

		// Fallback objective checks
		"fallback objective custom query not allowed": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Fallback: &datadoghq.DatadogFallbackPolicy{
					Horizontal: datadoghq.DatadogPodAutoscalerHorizontalFallbackPolicy{
						Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
							{Type: datadoghqcommon.DatadogPodAutoscalerCustomQueryObjectiveType},
						},
					},
				},
			},
			wantErr: "Autoscaler fallback cannot be based on custom query objective",
		},
		"fallback objective cpu allowed": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Fallback: &datadoghq.DatadogFallbackPolicy{
					Horizontal: datadoghq.DatadogPodAutoscalerHorizontalFallbackPolicy{
						Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
							{Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
								ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{}},
						},
					},
				},
			},
		},
		"fallback objective pod resource type without resource": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Fallback: &datadoghq.DatadogFallbackPolicy{
					Horizontal: datadoghq.DatadogPodAutoscalerHorizontalFallbackPolicy{
						Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
							{Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType},
						},
					},
				},
			},
			wantErr: "fallback: autoscaler objective type is PodResource but podResource is nil",
		},
		"fallback objective container resource type without resource": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Fallback: &datadoghq.DatadogFallbackPolicy{
					Horizontal: datadoghq.DatadogPodAutoscalerHorizontalFallbackPolicy{
						Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
							{Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType},
						},
					},
				},
			},
			wantErr: "fallback: autoscaler objective type is ContainerResource but containerResource is nil",
		},

		// Constraints: minReplicas / maxReplicas
		"maxReplicas less than minReplicas": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
					MinReplicas: pointer.Ptr[int32](5),
					MaxReplicas: pointer.Ptr[int32](2),
				},
			},
			wantErr: "constraints.maxReplicas (2) must be greater than or equal to constraints.minReplicas (5)",
		},
		"maxReplicas equals minReplicas": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
					MinReplicas: pointer.Ptr[int32](3),
					MaxReplicas: pointer.Ptr[int32](3),
				},
			},
		},
		"only minReplicas set": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
					MinReplicas: pointer.Ptr[int32](5),
				},
			},
		},
		"only maxReplicas set": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
					MaxReplicas: pointer.Ptr[int32](10),
				},
			},
		},

		// Constraints: duplicate container names
		"duplicate container name in constraints": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
					Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
						{Name: "app"},
						{Name: "sidecar"},
						{Name: "app"},
					},
				},
			},
			wantErr: `duplicate container name "app" in constraints`,
		},
		"unique container names in constraints": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
					Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
						{Name: "app"},
						{Name: "sidecar"},
					},
				},
			},
		},

		// Constraints: minAllowed > maxAllowed (top-level)
		"container cpu minAllowed greater than maxAllowed": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
					Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
						{
							Name:       "app",
							MinAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
							MaxAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
						},
					},
				},
			},
			wantErr: `container "app": minAllowed cpu (500m) must be less than or equal to maxAllowed (100m)`,
		},
		"container memory minAllowed greater than maxAllowed": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
					Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
						{
							Name:       "app",
							MinAllowed: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
							MaxAllowed: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("256Mi")},
						},
					},
				},
			},
			wantErr: `container "app": minAllowed memory (1Gi) must be less than or equal to maxAllowed (256Mi)`,
		},
		"container minAllowed equals maxAllowed": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
					Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
						{
							Name:       "app",
							MinAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m")},
							MaxAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m")},
						},
					},
				},
			},
		},
		"container only minAllowed set": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
					Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
						{
							Name:       "app",
							MinAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
						},
					},
				},
			},
		},
		"container only maxAllowed set": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
					Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
						{
							Name:       "app",
							MaxAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
						},
					},
				},
			},
		},
		"container cpu valid memory invalid": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
					Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
						{
							Name: "app",
							MinAllowed: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
							MaxAllowed: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
						},
					},
				},
			},
			wantErr: `container "app": minAllowed memory (1Gi) must be less than or equal to maxAllowed (256Mi)`,
		},

		// Constraints: legacy Requests field
		"legacy requests minAllowed greater than maxAllowed": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
					Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
						{
							Name: "app",
							Requests: &datadoghqcommon.DatadogPodAutoscalerContainerResourceConstraints{
								MinAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
								MaxAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
							},
						},
					},
				},
			},
			wantErr: `container "app": minAllowed cpu (500m) must be less than or equal to maxAllowed (100m)`,
		},
		"legacy requests valid bounds": {
			spec: datadoghq.DatadogPodAutoscalerSpec{
				Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
					Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
						{
							Name: "app",
							Requests: &datadoghqcommon.DatadogPodAutoscalerContainerResourceConstraints{
								MinAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
								MaxAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
							},
						},
					},
				},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := ValidateAutoscalerSpec(&tt.spec)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}
