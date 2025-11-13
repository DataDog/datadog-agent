// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

const (
	DatadogCreatedLabelKey  = "datadoghq.com/cluster-autoscaler.created"
	datadogModifiedLabelKey = "datadoghq.com/cluster-autoscaler.modified"
)

type NodePoolInternal struct {
	// Name matches name of NodePool
	name string `json:"name"`

	// nodePoolHash is hash of NodePoolSpec
	nodePoolHash string `json:"target_hash"` // TODO utilize once this is part of payload

	// recommendedInstanceTypes is list of recommended instance types
	recommendedInstanceTypes []string `json:"recommended_instance_types"`

	// labels is a map of Node labels that correspond to the NodePool
	labels map[string]string `json:"labels"`

	// taints is a list of node taints that correspond to the NodePool
	taints []corev1.Taint `json:"taints"`
}

func NewNodePoolInternal(v *kubeAutoscaling.ClusterAutoscalingValues) NodePoolInternal {
	return NodePoolInternal{
		name:                     v.Name,
		recommendedInstanceTypes: v.RecommendedInstanceTypes,
		labels:                   convertLabels(v.Labels),
		taints:                   convertTaints(v.Taints),
	}
}

func ConvertToKarpenterNodePool(n NodePoolInternal, nodeClassName string) *karpenterv1.NodePool {
	return &karpenterv1.NodePool{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NodePool",
			APIVersion: "karpenter.sh/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   n.name,
			Labels: map[string]string{DatadogCreatedLabelKey: "true"},
		},
		Spec: buildNodePoolSpec(n, nodeClassName),
	}

}

// Getters

// Name returns the name of the NodePoolInternal
func (n *NodePoolInternal) Name() string {
	return n.name
}

// NodePoolHash returns the nodePoolHash of the NodePoolInternal
func (n *NodePoolInternal) NodePoolHash() string {
	return n.nodePoolHash
}

// RecommendedInstanceTypes returns the recommendedInstanceTypes of the NodePoolInternal
func (n *NodePoolInternal) RecommendedInstanceTypes() []string {
	return n.recommendedInstanceTypes
}

// Labels returns the labels of the NodePoolInternal
func (n *NodePoolInternal) Labels() map[string]string {
	return n.labels
}

// Taints returns the taints of the NodePoolInternal
func (n *NodePoolInternal) Taints() []corev1.Taint {
	return n.taints
}

func convertLabels(input []*kubeAutoscaling.DomainLabels) map[string]string {
	output := make(map[string]string)
	for _, elem := range input {
		output[elem.Key] = elem.Value
	}

	return output
}

func convertTaints(input []*kubeAutoscaling.Taints) []corev1.Taint {
	output := []corev1.Taint{}
	for _, elem := range input {
		output = append(output, corev1.Taint{
			Key:    elem.Key,
			Value:  elem.Value,
			Effect: corev1.TaintEffect(elem.Effect),
		})
	}

	return output
}

// buildNodePoolSpec is used for creating new NodePools
func buildNodePoolSpec(n NodePoolInternal, nodeClassName string) karpenterv1.NodePoolSpec {

	// Convert domain labels into requirements
	reqs := []karpenterv1.NodeSelectorRequirementWithMinValues{}
	for k, v := range n.labels {
		reqs = append(reqs, karpenterv1.NodeSelectorRequirementWithMinValues{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      k,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{v},
			},
		})
	}

	// Convert instance types into a requirement
	reqs = append(reqs, karpenterv1.NodeSelectorRequirementWithMinValues{
		NodeSelectorRequirement: corev1.NodeSelectorRequirement{
			Key:      corev1.LabelInstanceTypeStable,
			Operator: corev1.NodeSelectorOpIn,
			Values:   n.recommendedInstanceTypes,
		},
	})

	return karpenterv1.NodePoolSpec{
		Template: karpenterv1.NodeClaimTemplate{
			Spec: karpenterv1.NodeClaimTemplateSpec{
				// Include taints
				Taints:       n.taints,
				Requirements: reqs,
				NodeClassRef: &karpenterv1.NodeClassReference{
					// Only support EC2NodeClass for now
					Kind:  "EC2NodeClass",
					Name:  nodeClassName,
					Group: "karpenter.k8s.aws",
				},
			},
		},
	}
}

// BuildNodePoolPatch is used to construct JSON patch
func BuildNodePoolPatch(np *karpenterv1.NodePool, npi NodePoolInternal) map[string]interface{} {

	// Build requirements patch, only updating values for the instance types
	updatedRequirements := []map[string]interface{}{}
	instanceTypeLabelExists := false
	for _, r := range np.Spec.Template.Spec.Requirements {
		if r.Key == corev1.LabelInstanceTypeStable {
			instanceTypeLabelExists = true
			r.Operator = "In"
			r.Values = npi.recommendedInstanceTypes
		}

		updatedRequirements = append(updatedRequirements, map[string]interface{}{
			"key":      r.Key,
			"operator": string(r.Operator),
			"values":   r.Values,
		})
	}

	if !instanceTypeLabelExists {
		updatedRequirements = append(updatedRequirements, map[string]interface{}{
			"key":      corev1.LabelInstanceTypeStable,
			"operator": "In",
			"values":   npi.recommendedInstanceTypes,
		})
	}

	return map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				datadogModifiedLabelKey: "true",
			},
		},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"requirements": updatedRequirements,
				},
			},
		},
	}
}
