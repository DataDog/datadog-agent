// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	// AWS Karpenter provider registers some variables in shared packages
	_ "github.com/aws/karpenter-provider-aws/pkg/apis/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

const (
	// For use on NodePools
	DatadogCreatedLabelKey      = "autoscaling.datadoghq.com/created"
	DatadogReplicaAnnotationKey = "autoscaling.datadoghq.com/target-nodepool"

	// KarpenterNodePoolHashAnnotationKey is the annotation key that that tracks the Karpenter NodePool template hash
	KarpenterNodePoolHashAnnotationKey = "karpenter.sh/nodepool-hash"
)

type NodePoolInternal struct {
	// targetName is the user-created NodePool the Datadog-managed NodePool is derived from
	targetName string

	// targetHash is hash of the user-created NodePoolSpec
	targetHash string

	// karpenterNodePool is the fully-formed Karpenter NodePool from the manifest
	karpenterNodePool *karpenterv1.NodePool
}

func NewNodePoolInternal(v ClusterAutoscalingValues) NodePoolInternal {
	npi := NodePoolInternal{
		targetName: v.TargetName,
		targetHash: v.TargetHash,
	}
	if v.Type == TypeKarpenterV1 && v.Manifest.KarpenterV1 != nil {
		npi.karpenterNodePool = buildKarpenterNodePoolFromManifest(v.Manifest.KarpenterV1)
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
	// Merge top-level metadata into template metadata. Top-level acts as defaults;
	// any keys already set in spec.Template.ObjectMeta take precedence.
	spec := *kv1.Spec
	mergedTemplateLabels := make(map[string]string, len(labels))
	for k, v := range labels {
		mergedTemplateLabels[k] = v
	}
	for k, v := range spec.Template.ObjectMeta.Labels {
		mergedTemplateLabels[k] = v
	}
	mergedTemplateAnnotations := make(map[string]string, len(annotations))
	for k, v := range annotations {
		mergedTemplateAnnotations[k] = v
	}
	for k, v := range spec.Template.ObjectMeta.Annotations {
		mergedTemplateAnnotations[k] = v
	}
	spec.Template.ObjectMeta = karpenterv1.ObjectMeta{
		Labels:      mergedTemplateLabels,
		Annotations: mergedTemplateAnnotations,
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

// Getters

// Name returns the name of the NodePoolInternal
func (n *NodePoolInternal) Name() string {
	if n.karpenterNodePool == nil {
		return ""
	}
	return n.karpenterNodePool.Name
}

// TargetName returns the targetName of the NodePoolInternal
func (n *NodePoolInternal) TargetName() string {
	return n.targetName
}

// TargetHash returns the targetHash of the NodePoolInternal
func (n *NodePoolInternal) TargetHash() string {
	return n.targetHash
}

// KarpenterNodePool returns the fully-formed NodePool from the manifest, or nil if the manifest was absent or invalid.
func (n *NodePoolInternal) KarpenterNodePool() *karpenterv1.NodePool {
	return n.karpenterNodePool
}

func GetNodePoolWeight(replicaNp *karpenterv1.NodePool) *int32 {
	weight := int32(1)
	if replicaNp.Spec.Weight != nil {
		if *replicaNp.Spec.Weight == 100 {
			log.Warnf("Target weight is at the max possible value for target NodePool: %s", replicaNp.Name)
		}
		weight = min(*replicaNp.Spec.Weight+1, 100)
	}
	return &weight
}
