// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func TestIsCreatedByDatadog(t *testing.T) {
	tests := []struct {
		name     string
		nodePool karpenterv1.NodePool
		expected bool
	}{
		{
			name: "no labels are present",
			nodePool: karpenterv1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "TestNodePool",
					Labels: map[string]string{},
				},
				Spec: karpenterv1.NodePoolSpec{
					Template: karpenterv1.NodeClaimTemplate{
						Spec: karpenterv1.NodeClaimTemplateSpec{
							Requirements: []karpenterv1.NodeSelectorRequirementWithMinValues{
								{
									NodeSelectorRequirement: corev1.NodeSelectorRequirement{
										Key:      "kubernetes.io/arch",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"amd64"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCreatedByDatadog(tt.nodePool)
			if result != tt.expected {
				t.Errorf("isCreatedByDatadog() = %v, want %v", result, tt.expected)
			}
		})
	}
}
