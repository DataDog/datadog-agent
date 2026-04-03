// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertLabels(t *testing.T) {
	tests := []struct {
		name     string
		input    []KeyValue
		expected map[string]string
	}{
		{
			name:     "basic",
			input:    []KeyValue{{Key: "foo", Value: "bar"}},
			expected: map[string]string{"foo": "bar"},
		},
		{
			name: "multiple",
			input: []KeyValue{
				{Key: "foo", Value: "bar"},
				{Key: "baz", Value: "qux"},
			},
			expected: map[string]string{"foo": "bar", "baz": "qux"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertLabels(tt.input)
			assert.Equal(t, tt.expected, result, "Output of convertLabels does not match expected result")
		})
	}
}

func TestConvertTaints(t *testing.T) {
	tests := []struct {
		name     string
		input    []Taint
		expected []corev1.Taint
	}{
		{
			name:  "basic",
			input: []Taint{{Key: "foo", Value: "bar", Effect: "NoSchedule"}},
			expected: []corev1.Taint{
				{Key: "foo", Value: "bar", Effect: corev1.TaintEffectNoSchedule},
			},
		},
		{
			name: "multiple",
			input: []Taint{
				{Key: "foo", Value: "bar", Effect: "NoSchedule"},
				{Key: "baz", Value: "qux", Effect: "PreferNoSchedule"},
			},
			expected: []corev1.Taint{
				{Key: "foo", Value: "bar", Effect: corev1.TaintEffectNoSchedule},
				{Key: "baz", Value: "qux", Effect: corev1.TaintEffectPreferNoSchedule},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertTaints(tt.input)
			assert.Equal(t, tt.expected, result, "Output of convertTaints does not match expected result")
		})
	}
}

func TestBuildNodePoolSpec(t *testing.T) {
	tests := []struct {
		name         string
		minNodePool  NodePoolInternal
		nodeClassRef *karpenterv1.NodeClassReference
		expected     karpenterv1.NodePoolSpec
	}{
		{
			name: "manual karpenter ec2nodeclass",
			minNodePool: NodePoolInternal{
				name:                     "default",
				recommendedInstanceTypes: []string{"t3.micro", "m5.large"},
				labels:                   map[string]string{"kubernetes.io/arch": "amd64", "kubernetes.io/os": "linux"},
				taints: []corev1.Taint{
					{
						Key:    "node",
						Value:  "test",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
			nodeClassRef: &karpenterv1.NodeClassReference{
				Kind:  "EC2NodeClass",
				Name:  "default",
				Group: "karpenter.k8s.aws",
			},
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
		{
			name: "eks auto mode nodeclass",
			minNodePool: NodePoolInternal{
				name:                     "default",
				recommendedInstanceTypes: []string{"t3.micro", "m5.large"},
				labels:                   map[string]string{"kubernetes.io/arch": "amd64", "kubernetes.io/os": "linux"},
				taints: []corev1.Taint{
					{
						Key:    "node",
						Value:  "test",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
			},
			nodeClassRef: &karpenterv1.NodeClassReference{
				Kind:  "NodeClass",
				Name:  "default",
				Group: "eks.amazonaws.com",
			},
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
							Kind:  "NodeClass",
							Name:  "default",
							Group: "eks.amazonaws.com",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildNodePoolSpec(tt.minNodePool, tt.nodeClassRef)
			assert.Equal(t, tt.expected.Template.Spec.Taints, result.Template.Spec.Taints, "Resulting NodePool does not match expected NodePool")
			assert.Equal(t, tt.expected.Template.Spec.NodeClassRef, result.Template.Spec.NodeClassRef, "Resulting NodePool does not match expected NodePool")
			assert.ElementsMatch(t, tt.expected.Template.Spec.Requirements, result.Template.Spec.Requirements, "Resulting NodePool does not match expected NodePool")
		})
	}
}

func TestBuildReplicaNodePool(t *testing.T) {
	tests := []struct {
		name        string
		nodePool    karpenterv1.NodePool
		minNodePool NodePoolInternal
	}{
		{
			name: "nil template labels, no instance type requirement",
			minNodePool: NodePoolInternal{
				name:                     "dd-linux-amd64-test",
				recommendedInstanceTypes: []string{"m5.large"},
				targetName:               "default",
			},
			nodePool: karpenterv1.NodePool{
				Spec: karpenterv1.NodePoolSpec{
					Template: karpenterv1.NodeClaimTemplate{
						Spec: karpenterv1.NodeClaimTemplateSpec{
							Requirements: []karpenterv1.NodeSelectorRequirementWithMinValues{},
						},
					},
				},
			},
		},
		{
			name: "existing instance type requirement",
			minNodePool: NodePoolInternal{
				name:                     "dd-linux-amd64-test",
				recommendedInstanceTypes: []string{"m5.large"},
				targetName:               "default",
			},
			nodePool: karpenterv1.NodePool{
				Spec: karpenterv1.NodePoolSpec{
					Template: karpenterv1.NodeClaimTemplate{
						ObjectMeta: karpenterv1.ObjectMeta{
							Labels: map[string]string{"existing": "label"},
						},
						Spec: karpenterv1.NodeClaimTemplateSpec{
							Requirements: []karpenterv1.NodeSelectorRequirementWithMinValues{
								{
									NodeSelectorRequirement: corev1.NodeSelectorRequirement{
										Key:      corev1.LabelInstanceTypeStable,
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"t3.micro"},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "existing requirements, no instance type requirement",
			minNodePool: NodePoolInternal{
				name:                     "dd-linux-amd64-test",
				recommendedInstanceTypes: []string{"m5.large"},
				targetName:               "default",
			},
			nodePool: karpenterv1.NodePool{
				Spec: karpenterv1.NodePoolSpec{
					Template: karpenterv1.NodeClaimTemplate{
						ObjectMeta: karpenterv1.ObjectMeta{
							Labels: map[string]string{"existing": "label"},
						},
						Spec: karpenterv1.NodeClaimTemplateSpec{
							Requirements: []karpenterv1.NodeSelectorRequirementWithMinValues{
								{
									NodeSelectorRequirement: corev1.NodeSelectorRequirement{
										Key:      "karpenter.k8s.aws/instance-family",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"t3"},
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
			np := BuildReplicaNodePool(&tt.nodePool, tt.minNodePool)

			assert.Equal(t, "true", np.Spec.Template.ObjectMeta.Labels[kubernetes.AutoscalingLabelKey])
			assert.Equal(t, "true", np.ObjectMeta.Labels[DatadogCreatedLabelKey])
			foundInstanceType := false
			for _, r := range np.Spec.Template.Spec.Requirements {
				if r.Key == corev1.LabelInstanceTypeStable {
					assert.Equal(t, tt.minNodePool.RecommendedInstanceTypes(), r.Values)
					foundInstanceType = true
					break
				}
			}
			assert.True(t, foundInstanceType, "instance type requirement should be present")
		})
	}
}

func TestUpdateNodePoolObject(t *testing.T) {
	weightOne := int32(1)

	tests := []struct {
		name             string
		targetNodePool   karpenterv1.NodePool
		ddNodePool       karpenterv1.NodePool
		minNodePool      NodePoolInternal
		expectedNodePool karpenterv1.NodePool
	}{
		{
			name: "instance type requirements have changed",
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
			targetNodePool: karpenterv1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "targetNp",
				},
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
			ddNodePool: karpenterv1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "replicaNp",
				},
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
			expectedNodePool: karpenterv1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "replicaNp",
					Labels: map[string]string{
						"autoscaling.datadoghq.com/modified": "true",
					},
				},
				Spec: karpenterv1.NodePoolSpec{
					Weight: &weightOne,
					Template: karpenterv1.NodeClaimTemplate{
						ObjectMeta: karpenterv1.ObjectMeta{
							Labels: map[string]string{
								"autoscaling.datadoghq.com/managed": "true",
							},
						},
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
										Values:   []string{"c5.xlarge", "t3.micro"},
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
		},
		{
			name: "instance type requirements operator has changed",
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
			targetNodePool: karpenterv1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "targetNp",
				},
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
										Operator: corev1.NodeSelectorOpNotIn,
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
			ddNodePool: karpenterv1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "replicaNp",
				},
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
										Operator: corev1.NodeSelectorOpNotIn,
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
			expectedNodePool: karpenterv1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "replicaNp",
					Labels: map[string]string{
						"autoscaling.datadoghq.com/modified": "true",
					},
				},
				Spec: karpenterv1.NodePoolSpec{
					Weight: &weightOne,
					Template: karpenterv1.NodeClaimTemplate{
						ObjectMeta: karpenterv1.ObjectMeta{
							Labels: map[string]string{
								"autoscaling.datadoghq.com/managed": "true",
							},
						},
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
		},
		{
			name: "instance type requirements have not changed",
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
			targetNodePool: karpenterv1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "targetNp",
				},
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
			ddNodePool: karpenterv1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "replicaNp",
				},
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
			expectedNodePool: karpenterv1.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "replicaNp",
					Labels: map[string]string{
						"autoscaling.datadoghq.com/modified": "true",
					},
				},
				Spec: karpenterv1.NodePoolSpec{
					Weight: &weightOne,
					Template: karpenterv1.NodeClaimTemplate{
						ObjectMeta: karpenterv1.ObjectMeta{
							Labels: map[string]string{
								"autoscaling.datadoghq.com/managed": "true",
							},
						},
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			np := UpdateNodePoolObject(&tt.targetNodePool, &tt.ddNodePool, tt.minNodePool)
			assert.Equal(t, tt.expectedNodePool, *np, "Resulting NodePool does not match expected NodePool")
		})
	}
}

// --- New manifest-based path tests ---

func TestNewNodePoolInternal_ManifestPath(t *testing.T) {
	weight := int32(10)
	values := ClusterAutoscalingValues{
		TargetName: "my-target",
		TargetHash: "abc123",
		Type:       TypeKarpenterV1,
		Manifest: Manifest{
			KarpenterV1: &KarpenterV1NodePool{
				Metadata: Metadata{
					Name: "dd-my-pool",
					Labels: Labels{
						{Key: "env", Value: "prod"},
					},
					Annotations: Annotations{
						{Key: "note", Value: "managed"},
					},
				},
				Spec: &karpenterv1.NodePoolSpec{
					Weight: &weight,
				},
			},
		},
	}

	npi := NewNodePoolInternal(values)

	assert.Equal(t, "dd-my-pool", npi.Name())
	assert.Equal(t, "my-target", npi.TargetName())
	assert.Equal(t, "abc123", npi.TargetHash())

	knp := npi.KarpenterNodePool()
	require.NotNil(t, knp)
	assert.Equal(t, "dd-my-pool", knp.Name)
	assert.Equal(t, "NodePool", knp.TypeMeta.Kind)
	assert.Equal(t, "karpenter.sh/v1", knp.TypeMeta.APIVersion)
	assert.Equal(t, map[string]string{"env": "prod"}, knp.Labels)
	assert.Equal(t, map[string]string{"note": "managed"}, knp.Annotations)
	assert.Equal(t, &weight, knp.Spec.Weight)
	assert.Equal(t, map[string]string{"env": "prod"}, knp.Spec.Template.ObjectMeta.Labels)
	assert.Equal(t, map[string]string{"note": "managed"}, knp.Spec.Template.ObjectMeta.Annotations)
}

func TestNewNodePoolInternal_ManifestNilUsesDeprecatedFields(t *testing.T) {
	values := ClusterAutoscalingValues{
		Name:                     "old-pool",
		RecommendedInstanceTypes: []string{"m5.large"},
		Labels:                   []KeyValue{{Key: "kubernetes.io/os", Value: "linux"}},
		Taints:                   []Taint{{Key: "node", Value: "test", Effect: "NoSchedule"}},
		TargetName:               "old-target",
		TargetHash:               "oldhash",
	}

	npi := NewNodePoolInternal(values)

	assert.Nil(t, npi.KarpenterNodePool())
	assert.Equal(t, "old-pool", npi.Name())
	assert.Equal(t, []string{"m5.large"}, npi.RecommendedInstanceTypes())
	assert.Equal(t, map[string]string{"kubernetes.io/os": "linux"}, npi.Labels())
	assert.Equal(t, []corev1.Taint{{Key: "node", Value: "test", Effect: corev1.TaintEffectNoSchedule}}, npi.Taints())
}

func TestNewNodePoolInternal_RecommendedInstanceTypesFromManifest(t *testing.T) {
	values := ClusterAutoscalingValues{
		Type: TypeKarpenterV1,
		Manifest: Manifest{
			KarpenterV1: &KarpenterV1NodePool{
				Metadata: Metadata{Name: "test-pool"},
				Spec: &karpenterv1.NodePoolSpec{
					Template: karpenterv1.NodeClaimTemplate{
						Spec: karpenterv1.NodeClaimTemplateSpec{
							Requirements: []karpenterv1.NodeSelectorRequirementWithMinValues{
								{
									NodeSelectorRequirement: corev1.NodeSelectorRequirement{
										Key:      corev1.LabelInstanceTypeStable,
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"t3.medium", "t3.large"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	npi := NewNodePoolInternal(values)
	require.NotNil(t, npi.KarpenterNodePool())
	assert.Equal(t, []string{"t3.medium", "t3.large"}, npi.RecommendedInstanceTypes())
}

func TestBuildKarpenterNodePoolFromManifest(t *testing.T) {
	weight := int32(5)
	kv1 := &KarpenterV1NodePool{
		Metadata: Metadata{
			Name:        "test-pool",
			Labels:      Labels{{Key: "foo", Value: "bar"}},
			Annotations: Annotations{{Key: "ann-key", Value: "ann-val"}},
		},
		Spec: &karpenterv1.NodePoolSpec{Weight: &weight},
	}

	knp := buildKarpenterNodePoolFromManifest(kv1)

	require.NotNil(t, knp)
	assert.Equal(t, metav1.TypeMeta{Kind: "NodePool", APIVersion: "karpenter.sh/v1"}, knp.TypeMeta)
	assert.Equal(t, "test-pool", knp.Name)
	assert.Equal(t, map[string]string{"foo": "bar"}, knp.Labels)
	assert.Equal(t, map[string]string{"ann-key": "ann-val"}, knp.Annotations)
	assert.Equal(t, &weight, knp.Spec.Weight)
	assert.Equal(t, map[string]string{"foo": "bar"}, knp.Spec.Template.ObjectMeta.Labels)
	assert.Equal(t, map[string]string{"ann-key": "ann-val"}, knp.Spec.Template.ObjectMeta.Annotations)
	assert.NotContains(t, knp.Labels, DatadogCreatedLabelKey)
}

func TestBuildKarpenterNodePoolFromManifest_NilSpec(t *testing.T) {
	kv1 := &KarpenterV1NodePool{
		Metadata: Metadata{Name: "test-pool"},
		Spec:     nil,
	}
	assert.Nil(t, buildKarpenterNodePoolFromManifest(kv1))
}
