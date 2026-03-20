// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	// AWS Karpenter provider registers some variables in shared packages
	_ "github.com/aws/karpenter-provider-aws/pkg/apis/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

const (
	// DatadogCreatedLabelKey is used on NodePools to indicate they were created by Datadog
	DatadogCreatedLabelKey = "autoscaling.datadoghq.com/created"

	// DatadogModifiedLabelKey is used on NodePools to indicate they were modified by Datadog
	DatadogModifiedLabelKey = "autoscaling.datadoghq.com/modified"

	// DatadogReplicaAnnotationKey stores the name of the user NodePool this Datadog-managed NodePool was derived from
	DatadogReplicaAnnotationKey = "autoscaling.datadoghq.com/target-nodepool"

	// KarpenterNodePoolHashAnnotationKey is the annotation key that tracks the Karpenter NodePool template hash
	KarpenterNodePoolHashAnnotationKey = "karpenter.sh/nodepool-hash"
)

type NodePoolInternal struct {
	// name matches name of NodePool
	name string

	// targetName is the user-created NodePool the Datadog-managed NodePool is derived from
	targetName string

	// targetHash is hash of the user-created NodePoolSpec
	targetHash string

	// karpenterNodePool is the fully-formed Karpenter NodePool from the manifest
	karpenterNodePool *karpenterv1.NodePool
}

// NewNodePoolInternal creates a NodePoolInternal from a ClusterAutoscalingValues schema object
func NewNodePoolInternal(v ClusterAutoscalingValues) NodePoolInternal {
	npi := NodePoolInternal{
		targetName: v.TargetName,
		targetHash: v.TargetHash,
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

// Name returns the name of the NodePoolInternal
func (n *NodePoolInternal) Name() string { return n.name }

// TargetName returns the targetName of the NodePoolInternal
func (n *NodePoolInternal) TargetName() string { return n.targetName }

// TargetHash returns the targetHash of the NodePoolInternal
func (n *NodePoolInternal) TargetHash() string { return n.targetHash }

// KarpenterNodePool returns the karpenterNodePool of the NodePoolInternal
func (n *NodePoolInternal) KarpenterNodePool() *karpenterv1.NodePool { return n.karpenterNodePool }

func (n *NodePoolInternal) RecommendedInstanceTypes() []string {
	if n.karpenterNodePool == nil {
		return []string{}
	}
	for _, req := range n.karpenterNodePool.Spec.Template.Spec.Requirements {
		if req.Key == corev1.LabelInstanceTypeStable {
			return req.Values
		}
	}
	return []string{}
}
