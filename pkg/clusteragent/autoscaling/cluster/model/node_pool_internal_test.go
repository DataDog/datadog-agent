// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/stretchr/testify/assert"
)

func TestConvertLabels(t *testing.T) {
	tests := []struct {
		name         string
		domainLabels []*kubeAutoscaling.DomainLabels
		expected     map[string]string
	}{
		{
			name: "basic",
			domainLabels: []*kubeAutoscaling.DomainLabels{
				{
					Key:   "foo",
					Value: "bar",
				},
			},
			expected: map[string]string{"foo": "bar"},
		},
		{
			name: "multiple",
			domainLabels: []*kubeAutoscaling.DomainLabels{
				{
					Key:   "foo",
					Value: "bar",
				},
				{
					Key:   "baz",
					Value: "qux",
				},
			},
			expected: map[string]string{"foo": "bar", "baz": "qux"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertLabels(tt.domainLabels)
			assert.Equal(t, tt.expected, result, "Output of convertLabels does not match expected result")
		})
	}
}

func TestConvertTaints(t *testing.T) {
	tests := []struct {
		name     string
		taints   []*kubeAutoscaling.Taints
		expected []corev1.Taint
	}{
		{
			name: "basic",
			taints: []*kubeAutoscaling.Taints{
				{
					Key:    "foo",
					Value:  "bar",
					Effect: "NoSchedule",
				},
			},
			expected: []corev1.Taint{
				{
					Key:    "foo",
					Value:  "bar",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
		},
		{
			name: "basic",
			taints: []*kubeAutoscaling.Taints{
				{
					Key:    "foo",
					Value:  "bar",
					Effect: "NoSchedule",
				},
				{
					Key:    "baz",
					Value:  "qux",
					Effect: "PreferNoSchedule",
				},
			},
			expected: []corev1.Taint{
				{
					Key:    "foo",
					Value:  "bar",
					Effect: corev1.TaintEffectNoSchedule,
				},
				{
					Key:    "baz",
					Value:  "qux",
					Effect: corev1.TaintEffectPreferNoSchedule,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertTaints(tt.taints)
			assert.Equal(t, tt.expected, result, "Output of convertTaints does not match expected result")
		})
	}
}

func TestBuildNodePoolSpec(t *testing.T) {
	tests := []struct {
		name          string
		minNodePool   NodePoolInternal
		nodeClassName string
		expected      karpenterv1.NodePoolSpec
	}{
		{
			name: "basic",
			minNodePool: NodePoolInternal{
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
		minNodePool NodePoolInternal
		expected    map[string]any
	}{
		{
			name: "basic",
			minNodePool: NodePoolInternal{
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
			expected: map[string]any{
				"metadata": map[string]any{
					"labels": map[string]any{
						datadogModifiedLabelKey: "true",
					},
				},
				"spec": map[string]any{
					"template": map[string]any{
						"metadata": map[string]any{
							"labels": map[string]string{
								kubernetes.AutoscalingLabelKey: "true",
							},
						},
						"spec": map[string]any{
							"requirements": []map[string]any{
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
			result := BuildNodePoolPatch(&tt.nodePool, tt.minNodePool)
			assert.Equal(t, tt.expected, result, "Resulting patch does not match expected patch")
		})
	}
}
