// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"maps"
	"slices"

	// AWS Karpenter provider registers some variables in shared packages
	_ "github.com/aws/karpenter-provider-aws/pkg/apis/v1"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

const (
	// For use on NodePools
	DatadogCreatedLabelKey      = "autoscaling.datadoghq.com/created"
	DatadogModifiedLabelKey     = "autoscaling.datadoghq.com/modified"
	DatadogReplicaAnnotationKey = "autoscaling.datadoghq.com/target-nodepool"

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

	// karpenterNodePool is the fully-formed Karpenter NodePool from the manifest (new path only)
	karpenterNodePool *karpenterv1.NodePool
}

func NewNodePoolInternal(v ClusterAutoscalingValues) NodePoolInternal {
	npi := NodePoolInternal{
		name:                     v.Name,
		recommendedInstanceTypes: v.RecommendedInstanceTypes,
		labels:                   convertLabels(v.Labels),
		taints:                   convertTaints(v.Taints),
		targetName:               v.TargetName,
		targetHash:               v.TargetHash,
	}
	if v.Type == TypeKarpenterV1 && v.Manifest.KarpenterV1 != nil {
		knp := buildKarpenterNodePoolFromManifest(v.Manifest.KarpenterV1)
		npi.karpenterNodePool = knp
		if knp != nil {
			npi.name = knp.Name
		}
	}
	return npi
}

func buildKarpenterNodePoolFromManifest(kv1 *KarpenterV1NodePool) *karpenterv1.NodePool {
	if kv1.Spec == nil {
		log.Debugf("KarpenterV1NodePool %q has nil spec, skipping manifest path", kv1.Metadata.Name)
		return nil
	}
	labels := make(map[string]string, len(kv1.Metadata.Labels))
	for _, kv := range kv1.Metadata.Labels {
		labels[kv.Key] = kv.Value
	}
	annotations := make(map[string]string, len(kv1.Metadata.Annotations))
	for _, kv := range kv1.Metadata.Annotations {
		annotations[kv.Key] = kv.Value
	}
	templateLabels := make(map[string]string, len(labels))
	for k, v := range labels {
		templateLabels[k] = v
	}
	templateAnnotations := make(map[string]string, len(annotations))
	for k, v := range annotations {
		templateAnnotations[k] = v
	}
	spec := *kv1.Spec
	spec.Template.ObjectMeta = karpenterv1.ObjectMeta{
		Labels:      templateLabels,
		Annotations: templateAnnotations,
	}
	return &karpenterv1.NodePool{
		TypeMeta: metav1.TypeMeta{Kind: "NodePool", APIVersion: "karpenter.sh/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        kv1.Metadata.Name,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: spec,
	}
}

func ConvertToKarpenterNodePool(n NodePoolInternal, nodeClassRef *karpenterv1.NodeClassReference) *karpenterv1.NodePool {
	knp := &karpenterv1.NodePool{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NodePool",
			APIVersion: "karpenter.sh/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   n.name,
			Labels: map[string]string{DatadogCreatedLabelKey: "true"},
		},
		Spec: buildNodePoolSpec(n, nodeClassRef),
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
	if n.karpenterNodePool != nil {
		for _, req := range n.karpenterNodePool.Spec.Template.Spec.Requirements {
			if req.Key == corev1.LabelInstanceTypeStable {
				return req.Values
			}
		}
		return []string{}
	}
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

// KarpenterNodePool returns the fully-formed NodePool from the manifest, or nil if the old schema was used.
func (n *NodePoolInternal) KarpenterNodePool() *karpenterv1.NodePool {
	return n.karpenterNodePool
}

func convertLabels(input []KeyValue) map[string]string {
	output := make(map[string]string)
	for _, kv := range input {
		output[kv.Key] = kv.Value
	}
	return output
}

func convertTaints(input []Taint) []corev1.Taint {
	output := []corev1.Taint{}
	for _, t := range input {
		output = append(output, corev1.Taint{
			Key:    t.Key,
			Value:  t.Value,
			Effect: corev1.TaintEffect(t.Effect),
		})
	}
	return output
}

var deprecatedLabels = sets.New("beta.kubernetes.io/arch", "beta.kubernetes.io/os")

// buildNodePoolSpec is used for creating new NodePools from scratch
func buildNodePoolSpec(n NodePoolInternal, nodeClassRef *karpenterv1.NodeClassReference) karpenterv1.NodePoolSpec {
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
				NodeClassRef: nodeClassRef,
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
		npCopy.ObjectMeta.Labels = maps.Clone(targetNp.GetLabels())

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
			Annotations: map[string]string{DatadogReplicaAnnotationKey: npi.TargetName()},
		}
		// Reset the Status
		replicaNp.Status = karpenterv1.NodePoolStatus{}
	} else {
		if replicaNp.ObjectMeta.Labels == nil {
			replicaNp.ObjectMeta.Labels = make(map[string]string)
		}
		replicaNp.ObjectMeta.Labels[DatadogModifiedLabelKey] = "true"
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
