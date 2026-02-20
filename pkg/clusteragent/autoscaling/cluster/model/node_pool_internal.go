// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"slices"

	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	// AWS Karpenter provider registers some variables in shared packages
	_ "github.com/aws/karpenter-provider-aws/pkg/apis/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

const (
	// For use on NodePools
	DatadogCreatedLabelKey      = "autoscaling.datadoghq.com/created"
	datadogModifiedLabelKey     = "autoscaling.datadoghq.com/modified"
	datadogReplicaAnnotationKey = "autoscaling.datadoghq.com/target-nodepool"

	// KarpenterNodePoolHashAnnotationKey is the annotation key that that tracks the Karpenter NodePool template hash
	KarpenterNodePoolHashAnnotationKey = "karpenter.sh/nodepool-hash"
)

type NodePoolInternal struct {
	// Name matches name of NodePool
	name string

	// recommendedInstanceTypes is list of recommended instance types
	recommendedInstanceTypes []string

	// labels is a map of Node labels that correspond to the NodePool
	labels map[string]string

	// taints is a list of node taints that correspond to the NodePool
	taints []corev1.Taint

	// targetName is the user-created NodePool the Datadog-managed NodePool is derived from
	targetName string

	// targetHash is hash of the user-created NodePoolSpec
	targetHash string
}

func NewNodePoolInternal(v *kubeAutoscaling.ClusterAutoscalingValues) NodePoolInternal {
	return NodePoolInternal{
		name:                     v.Name,
		recommendedInstanceTypes: v.RecommendedInstanceTypes,
		labels:                   convertLabels(v.Labels),
		taints:                   convertTaints(v.Taints),
		targetName:               v.TargetName,
		targetHash:               v.TargetHash,
	}
}

func ConvertToKarpenterNodePool(n NodePoolInternal, nodeClassName string) *karpenterv1.NodePool {
	knp := &karpenterv1.NodePool{
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

	return knp
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

var deprecatedLabels = sets.New("beta.kubernetes.io/arch", "beta.kubernetes.io/os")

// buildNodePoolSpec is used for creating new NodePools from scratch
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
	// sort the instance types first for readability
	instanceTypes := n.RecommendedInstanceTypes()
	slices.Sort(instanceTypes)
	reqs = append(reqs, karpenterv1.NodeSelectorRequirementWithMinValues{
		NodeSelectorRequirement: corev1.NodeSelectorRequirement{
			Key:      corev1.LabelInstanceTypeStable,
			Operator: corev1.NodeSelectorOpIn,
			Values:   instanceTypes,
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

	return npSpec
}

// BuildReplicaNodePool copies the target NodePool spec and updates it to create a replica NodePool
func BuildReplicaNodePool(targetNp *karpenterv1.NodePool, npi NodePoolInternal) *karpenterv1.NodePool {

	replicaNp := targetNp.DeepCopy()

	modifyNodePoolSpec(replicaNp, npi)

	modifyReplicaNodePool(replicaNp, npi, true)

	return replicaNp
}

// UpdateNodePoolObject updates a copy of a NodePool object with the recommended instance types and object metadata.
func UpdateNodePoolObject(targetNp, datadogNp *karpenterv1.NodePool, npi NodePoolInternal) *karpenterv1.NodePool {
	var npCopy *karpenterv1.NodePool
	if targetNp != nil {
		// Base replica NodePool on target NodePool spec
		npCopy = targetNp.DeepCopy()
		// Preserve ObjectMeta from Datadog-created NodePool
		npCopy.ObjectMeta = datadogNp.ObjectMeta
		// Allow label updates to propagate from target to replica
		npCopy.ObjectMeta.Labels = targetNp.GetLabels()

		modifyReplicaNodePool(npCopy, npi, false)
	} else {
		npCopy = datadogNp.DeepCopy()
	}

	modifyNodePoolSpec(npCopy, npi)

	return npCopy
}

// TODO: Add logic for any existing requirements that could be incompatible with recommendations
func modifyNodePoolSpec(np *karpenterv1.NodePool, npi NodePoolInternal) {
	instanceTypes := npi.RecommendedInstanceTypes()
	slices.Sort(instanceTypes)

	// Update instance type requirements
	instanceTypeLabelFound := false
	for i := range np.Spec.Template.Spec.Requirements {
		r := &np.Spec.Template.Spec.Requirements[i]
		if r.Key == corev1.LabelInstanceTypeStable {
			instanceTypeLabelFound = true
			if r.Operator != corev1.NodeSelectorOpIn {
				r.Operator = corev1.NodeSelectorOpIn
			}

			if !slices.Equal(r.Values, instanceTypes) {
				r.Values = instanceTypes
			}
			break
		}
	}

	if !instanceTypeLabelFound {
		np.Spec.Template.Spec.Requirements = append(np.Spec.Template.Spec.Requirements,
			karpenterv1.NodeSelectorRequirementWithMinValues{
				NodeSelectorRequirement: corev1.NodeSelectorRequirement{
					Key:      corev1.LabelInstanceTypeStable,
					Operator: corev1.NodeSelectorOpIn,
					Values:   instanceTypes,
				},
			},
		)
	}

	// Update Template ObjectMeta labels
	if np.Spec.Template.ObjectMeta.Labels == nil {
		np.Spec.Template.ObjectMeta.Labels = make(map[string]string)
	}
	np.Spec.Template.ObjectMeta.Labels[kubernetes.AutoscalingLabelKey] = "true"
}

func modifyReplicaNodePool(replicaNp *karpenterv1.NodePool, npi NodePoolInternal, isNew bool) {
	if isNew {
		// Reset the TypeMeta
		replicaNp.TypeMeta = metav1.TypeMeta{
			Kind:       "NodePool",
			APIVersion: "karpenter.sh/v1",
		}

		// Reset the ObjectMeta
		replicaNp.ObjectMeta = metav1.ObjectMeta{
			Name:        npi.Name(), // Ensure replica name is used in lieu of TargetName
			Labels:      map[string]string{DatadogCreatedLabelKey: "true"},
			Annotations: map[string]string{datadogReplicaAnnotationKey: npi.TargetName()},
		}
		// Reset the Status
		replicaNp.Status = karpenterv1.NodePoolStatus{}
	} else {
		if replicaNp.ObjectMeta.Labels == nil {
			replicaNp.ObjectMeta.Labels = make(map[string]string)
		}
		replicaNp.ObjectMeta.Labels[datadogModifiedLabelKey] = "true"
	}

	// Update the weight
	weight := int32(1)
	if replicaNp.Spec.Weight != nil {
		if *replicaNp.Spec.Weight == 100 {
			log.Warnf("Target weight is at the max possible value for target NodePool: %s", npi.TargetName())
		}
		weight = min(*replicaNp.Spec.Weight+1, 100)
	}
	replicaNp.Spec.Weight = &weight
}
