// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	_ "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

const (
	// For use on NodePools
	DatadogCreatedLabelKey  = "autoscaling.datadoghq.com/created"
	datadogModifiedLabelKey = "autoscaling.datadoghq.com/modified"
)

type NodePoolInternal struct {
	// Name matches name of NodePool
	name string `json:"name"`

	// recommendedInstanceTypes is list of recommended instance types
	recommendedInstanceTypes []string `json:"recommended_instance_types"`

	// labels is a map of Node labels that correspond to the NodePool
	labels map[string]string `json:"labels"`

	// taints is a list of node taints that correspond to the NodePool
	taints []corev1.Taint `json:"taints"`

	// targetName is the user-created NodePool the Datadog-managed NodePool is derived from
	targetName string `json:"target_name"`

	// targetHash is hash of the user-created NodePoolSpec
	targetHash string `json:"target_hash"` // TODO utilize once this is part of payload

	// targetWeight is weight of the user-created NodePoolSpec
	targetWeight *int32 `json:"target_weight"`
}

func NewNodePoolInternal(v *kubeAutoscaling.ClusterAutoscalingValues) NodePoolInternal {
	return NodePoolInternal{
		name:                     v.Name,
		recommendedInstanceTypes: v.RecommendedInstanceTypes,
		labels:                   convertLabels(v.Labels),
		taints:                   convertTaints(v.Taints),
		targetName:               v.TargetName,
		targetHash:               v.TargetHash,
		targetWeight:             v.TargetWeight,
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

// TargetName returns the targetName of the NodePoolInternal
func (n *NodePoolInternal) TargetName() string {
	return n.targetName
}

// TargetHash returns the targetHash of the NodePoolInternal
func (n *NodePoolInternal) TargetHash() string {
	return n.targetHash
}

// TargetWeight returns the targetWeight of the NodePoolInternal
func (n *NodePoolInternal) TargetWeight() int32 {
	return *n.targetWeight
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

var deprecatedLabels = sets.New[string]("beta.kubernetes.io/arch", "beta.kubernetes.io/os")

// buildNodePoolSpec is used for creating new NodePools
func buildNodePoolSpec(n NodePoolInternal, nodeClassName string) karpenterv1.NodePoolSpec {

	wellKnownLabels := karpenterv1.WellKnownLabels

	metadataLabels := map[string]string{}

	// Convert domain labels into requirements
	reqs := make([]karpenterv1.NodeSelectorRequirementWithMinValues, 0, len(n.labels)+1)
	for k, v := range n.labels {

		// Don't include long-deprecated labels
		if deprecatedLabels.Has(k) {
			continue
		}

		// If it is a well-known label, use Operator In
		if wellKnownLabels.Has(k) {
			reqs = append(reqs, karpenterv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: corev1.NodeSelectorRequirement{
					Key:      k,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{v},
				},
			})
			// If it is not a well-known label, use Operator Exists and include the label in the metadata
		} else {
			reqs = append(reqs, karpenterv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: corev1.NodeSelectorRequirement{
					Key:      k,
					Operator: corev1.NodeSelectorOpExists,
				},
			})
			metadataLabels[k] = v
		}
	}

	// Convert instance types into a requirement
	reqs = append(reqs, karpenterv1.NodeSelectorRequirementWithMinValues{
		NodeSelectorRequirement: corev1.NodeSelectorRequirement{
			Key:      corev1.LabelInstanceTypeStable,
			Operator: corev1.NodeSelectorOpIn,
			Values:   n.recommendedInstanceTypes,
		},
	})

	// Add autoscaling label
	metadataLabels[kubernetes.AutoscalingLabelKey] = "true"

	npSpec := karpenterv1.NodePoolSpec{
		Template: karpenterv1.NodeClaimTemplate{
			ObjectMeta: karpenterv1.ObjectMeta{
				Labels: metadataLabels,
			},
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

	if n.TargetWeight != nil {
		if n.TargetWeight() >= 0 && n.TargetWeight() < 100 {
			weight := n.TargetWeight() + 1
			npSpec.Weight = &weight
		} else {
			log.Warnf("TargetWeight is invalid: %v for Target NodePool: %s", n.TargetWeight(), n.TargetName())
		}
	}

	return npSpec
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
				"metadata": map[string]interface{}{
					"labels": map[string]string{
						kubernetes.AutoscalingLabelKey: "true",
					},
				},
				"spec": map[string]interface{}{
					"requirements": updatedRequirements,
				},
			},
		},
	}
}
