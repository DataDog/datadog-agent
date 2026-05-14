// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"maps"
	"strings"

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

// templateMetadataBlocklistedDomains lists base domains blocked from template_metadata
// labels and annotations.
var templateMetadataBlocklistedDomains = []string{
	"kubernetes.io",
	"k8s.io",
	"karpenter.sh",
}

// filterTemplateMetadataKeys filters out reserved keys and returns a map that can be applied
// to the nodepool template labels or annotations.
func filterTemplateMetadataKeys(kvs []KeyValue) map[string]string {
	out := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		if isBlocklistedTemplateMetadataKey(kv.Key) {
			log.Warnf("Dropping template_metadata key %q", kv.Key)
			continue
		}
		out[kv.Key] = kv.Value
	}
	return out
}

func isBlocklistedTemplateMetadataKey(key string) bool {
	domain := karpenterv1.GetLabelDomain(key)
	if domain == "" {
		return false
	}
	for _, blocked := range templateMetadataBlocklistedDomains {
		if domain == blocked || strings.HasSuffix(domain, "."+blocked) {
			return true
		}
	}
	return false
}

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

	// Handle template metadata labels/annotations
	spec := *kv1.Spec
	if kv1.TemplateMetadata != nil {
		spec.Template.ObjectMeta = karpenterv1.ObjectMeta{
			Labels:      filterTemplateMetadataKeys(kv1.TemplateMetadata.Labels),
			Annotations: filterTemplateMetadataKeys(kv1.TemplateMetadata.Annotations),
		}
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

// BuildReplicaNodePool produces a NodePool for a Datadog-managed replica of an existing target NodePool
// The target is used as the base and RC values are applied on top:
//   - Top-level labels/annotations are always replaced from RC
//   - Other fields are only overwritten if RC explicitly set them; otherwise the target's value is preserved
//   - Template-level labels/annotations from RC are merged on top of the target's
func (n *NodePoolInternal) BuildReplicaNodePool(targetNp *karpenterv1.NodePool) *karpenterv1.NodePool {
	if n.karpenterNodePool == nil || targetNp == nil {
		return nil
	}
	rc := n.karpenterNodePool

	merged := targetNp.DeepCopy()
	merged.TypeMeta = rc.TypeMeta
	merged.Status = karpenterv1.NodePoolStatus{}

	// Construct top-level metadata from RC
	merged.ObjectMeta = metav1.ObjectMeta{
		Name:        rc.Name,
		Labels:      maps.Clone(rc.Labels),
		Annotations: maps.Clone(rc.Annotations),
	}

	// NodePoolSpec fields: use RC values if set, else fallback to target
	if rc.Spec.Weight != nil {
		merged.Spec.Weight = rc.Spec.Weight
	} else {
		merged.Spec.Weight = getNodePoolWeight(targetNp)
	}
	merged.Spec.Replicas = mergePtrs(rc.Spec.Replicas, merged.Spec.Replicas)
	if len(rc.Spec.Limits) > 0 {
		merged.Spec.Limits = rc.Spec.Limits
	}
	if rc.Spec.Disruption.ConsolidationPolicy != "" || rc.Spec.Disruption.ConsolidateAfter.Duration != nil || len(rc.Spec.Disruption.Budgets) > 0 {
		merged.Spec.Disruption = rc.Spec.Disruption
	}

	// Template ObjectMeta: RC keys merged on top of target's
	for k, v := range rc.Spec.Template.ObjectMeta.Labels {
		if merged.Spec.Template.ObjectMeta.Labels == nil {
			merged.Spec.Template.ObjectMeta.Labels = map[string]string{}
		}
		merged.Spec.Template.ObjectMeta.Labels[k] = v
	}
	for k, v := range rc.Spec.Template.ObjectMeta.Annotations {
		if merged.Spec.Template.ObjectMeta.Annotations == nil {
			merged.Spec.Template.ObjectMeta.Annotations = map[string]string{}
		}
		merged.Spec.Template.ObjectMeta.Annotations[k] = v
	}

	// NodeClaimTemplateSpec fields: use RC values if set, else fallback to target
	merged.Spec.Template.Spec.Requirements = mergeSlices(rc.Spec.Template.Spec.Requirements, merged.Spec.Template.Spec.Requirements)
	if rc.Spec.Template.Spec.NodeClassRef != nil {
		merged.Spec.Template.Spec.NodeClassRef = rc.Spec.Template.Spec.NodeClassRef.DeepCopy()
	}
	merged.Spec.Template.Spec.Taints = mergeSlices(rc.Spec.Template.Spec.Taints, merged.Spec.Template.Spec.Taints)
	merged.Spec.Template.Spec.StartupTaints = mergeSlices(rc.Spec.Template.Spec.StartupTaints, merged.Spec.Template.Spec.StartupTaints)
	merged.Spec.Template.Spec.TerminationGracePeriod = mergePtrs(rc.Spec.Template.Spec.TerminationGracePeriod, merged.Spec.Template.Spec.TerminationGracePeriod)
	if rc.Spec.Template.Spec.ExpireAfter.Duration != nil {
		merged.Spec.Template.Spec.ExpireAfter = rc.Spec.Template.Spec.ExpireAfter
	}

	return merged
}

func getNodePoolWeight(replicaNp *karpenterv1.NodePool) *int32 {
	weight := int32(1)
	if replicaNp.Spec.Weight != nil {
		if *replicaNp.Spec.Weight == 100 {
			log.Warnf("Target weight is at the max possible value for target NodePool: %s", replicaNp.Name)
		}
		weight = min(*replicaNp.Spec.Weight+1, 100)
	}
	return &weight
}

// mergePtrs returns rc if non-nil, otherwise target
func mergePtrs[T any](rc, target *T) *T {
	if rc != nil {
		return rc
	}
	return target
}

// mergeSlices returns rc if non-empty, otherwise target
func mergeSlices[T any](rc, target []T) []T {
	if len(rc) > 0 {
		return rc
	}
	return target
}
