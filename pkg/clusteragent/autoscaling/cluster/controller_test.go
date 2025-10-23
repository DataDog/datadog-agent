// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/stretchr/testify/assert"
)

func TestBuildNodePoolSpec(t *testing.T) {
	tests := []struct {
		name          string
		minNodePool   minNodePool
		nodeClassName string
		expected      karpenterv1.NodePoolSpec
	}{
		{
			name: "basic",
			minNodePool: minNodePool{
				name:                     "default",
				recommendedInstanceTypes: []string{"m5.large", "t3.micro"},
				labels:                   map[string]string{"kubernetes.io/arch": "amd64", "kubernetes.io/os": "linux"},
				taints: []corev1.Taint{
					{
						Key:    "node",
						Value:  "test",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
			nodeClassName: "default",
			expected: karpenterv1.NodePoolSpec{
				Template: karpenterv1.NodeClaimTemplate{
					Spec: karpenterv1.NodeClaimTemplateSpec{
						Taints: []corev1.Taint{
							{
								Key:    "node",
								Value:  "test",
								Effect: corev1.TaintEffectNoSchedule,
							},
						},
						Requirements: []karpenterv1.NodeSelectorRequirementWithMinValues{
							{
								NodeSelectorRequirement: corev1.NodeSelectorRequirement{
									Key:      "kubernetes.io/os",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"linux"},
								},
							},
							{
								NodeSelectorRequirement: corev1.NodeSelectorRequirement{
									Key:      "kubernetes.io/arch",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"amd64"},
								},
							},
							{
								NodeSelectorRequirement: corev1.NodeSelectorRequirement{
									Key:      corev1.LabelInstanceTypeStable,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"m5.large", "t3.micro"},
								},
							},
						},
						NodeClassRef: &karpenterv1.NodeClassReference{
							Kind:  "EC2NodeClass",
							Name:  "default",
							Group: "karpenter.k8s.aws",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildNodePoolSpec(tt.minNodePool, tt.nodeClassName)
			assert.Equal(t, tt.expected.Template.Spec.Taints, result.Template.Spec.Taints, "Resulting NodePool does not match expected NodePool")
			assert.Equal(t, tt.expected.Template.Spec.NodeClassRef, result.Template.Spec.NodeClassRef, "Resulting NodePool does not match expected NodePool")
			assert.ElementsMatch(t, tt.expected.Template.Spec.Requirements, result.Template.Spec.Requirements, "Resulting NodePool does not match expected NodePool")
		})
	}

}

func TestBuildNodePoolPatch(t *testing.T) {
	tests := []struct {
		name        string
		nodePool    karpenterv1.NodePool
		minNodePool minNodePool
		expected    map[string]interface{}
	}{
		{
			name: "basic",
			minNodePool: minNodePool{
				name:                     "default",
				recommendedInstanceTypes: []string{"c5.xlarge", "t3.micro"},
				labels:                   map[string]string{"kubernetes.io/arch": "amd64", "kubernetes.io/os": "linux"},
				taints: []corev1.Taint{
					{
						Key:    "node",
						Value:  "test",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
			nodePool: karpenterv1.NodePool{
				Spec: karpenterv1.NodePoolSpec{
					Template: karpenterv1.NodeClaimTemplate{
						Spec: karpenterv1.NodeClaimTemplateSpec{
							Taints: []corev1.Taint{
								{
									Key:    "node",
									Value:  "test",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
							Requirements: []karpenterv1.NodeSelectorRequirementWithMinValues{
								{
									NodeSelectorRequirement: corev1.NodeSelectorRequirement{
										Key:      "kubernetes.io/arch",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"amd64"},
									},
								},
								{
									NodeSelectorRequirement: corev1.NodeSelectorRequirement{
										Key:      "kubernetes.io/os",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"linux"},
									},
								},
								{
									NodeSelectorRequirement: corev1.NodeSelectorRequirement{
										Key:      corev1.LabelInstanceTypeStable,
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"m5.large", "t3.micro"},
									},
								},
							},
							NodeClassRef: &karpenterv1.NodeClassReference{
								Kind:  "EC2NodeClass",
								Name:  "default",
								Group: "karpenter.k8s.aws",
							},
						},
					},
				},
			},
			expected: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						datadogModifiedLabelKey: "true",
					},
				},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"requirements": []map[string]interface{}{
								{
									"key":      "kubernetes.io/arch",
									"operator": "In",
									"values":   []string{"amd64"},
								},
								{
									"key":      "kubernetes.io/os",
									"operator": "In",
									"values":   []string{"linux"},
								},
								{
									"key":      corev1.LabelInstanceTypeStable,
									"operator": "In",
									"values":   []string{"c5.xlarge", "t3.micro"},
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
			result := buildNodePoolPatch(&tt.nodePool, tt.minNodePool)
			assert.Equal(t, tt.expected, result, "Resulting patch does not match expected patch")
		})
	}

}

func TestIsCreatedByDatadog(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected bool
	}{
		{
			name:     "no labels are present",
			labels:   map[string]string{},
			expected: false,
		},
		{
			name: "other labels are present",
			labels: map[string]string{
				"otherLabel": "otherValue",
			},
			expected: false,
		},
		{
			name: "created label is present",
			labels: map[string]string{
				datadogCreatedLabelKey: "true",
			},
			expected: true,
		},
		{
			name: "created and other label is present",
			labels: map[string]string{
				datadogCreatedLabelKey: "true",
				"otherLabel":           "otherValue",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCreatedByDatadog(tt.labels)
			if result != tt.expected {
				t.Errorf("isCreatedByDatadog() = %v, want %v", result, tt.expected)
			}
		})
	}
}
