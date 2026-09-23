// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package util

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// PodWorkloadTarget returns the workload controller owning the pod, resolving
// ReplicaSet -> Deployment (or Argo Rollout) through its owner references.
//
// It is how cluster-scoped facts about a workload, such as the autoscalers
// acting on it, are joined to its pods. The Cluster Agent and the node agent
// both need that join, and must agree on it.
//
// Kubernetes does not give a pod several controllers of the kinds tracked
// here, so the first resolvable owner wins.
func PodWorkloadTarget(pod *workloadmeta.KubernetesPod) (kubernetes.WorkloadTarget, bool) {
	for _, owner := range pod.Owners {
		if target, ok := kubernetes.ResolveWorkloadTarget(pod.Namespace, owner.Kind, owner.Name, pod.Labels); ok {
			return target, true
		}
	}
	return kubernetes.WorkloadTarget{}, false
}

// workloadMetadataResources maps the workload kinds represented in workloadmeta
// by a generic KubernetesMetadata entity to their API resource. Deployments are
// not here: they have their own entity kind, KubernetesDeployment.
var workloadMetadataResources = map[string]schema.GroupVersionResource{
	kubernetes.StatefulSetKind: {Group: "apps", Version: "v1", Resource: "statefulsets"},
	kubernetes.RolloutKind:     {Group: "argoproj.io", Version: "v1alpha1", Resource: "rollouts"},
}

// WorkloadMetadataEntity returns the KubernetesMetadata entity representing a
// workload, carrying only its identity: the ID and GVR the generic metadata
// collector would use for it, so both merge onto the same entity. It reports
// false for workload kinds not represented as KubernetesMetadata.
func WorkloadMetadataEntity(target kubernetes.WorkloadTarget) (*workloadmeta.KubernetesMetadata, bool) {
	gvr, ok := workloadMetadataResources[target.Kind]
	if !ok {
		return nil, false
	}
	return &workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   string(GenerateKubeMetadataEntityID(gvr.Group, gvr.Resource, target.Namespace, target.Name)),
		},
		EntityMeta: workloadmeta.EntityMeta{Name: target.Name, Namespace: target.Namespace},
		GVR:        &gvr,
	}, true
}

// WorkloadTargetFromMetadata is the inverse of WorkloadMetadataEntity. It
// reports false for metadata entities that are not a workload (namespaces,
// arbitrary resources collected for labels as tags, ...).
//
// It relies on the entity ID only, not on EntityMeta or the GVR: an Unset
// event for a deleted resource may carry nothing else.
func WorkloadTargetFromMetadata(metadata *workloadmeta.KubernetesMetadata) (kubernetes.WorkloadTarget, bool) {
	group, resource, namespace, name, err := ParseKubeMetadataEntityID(workloadmeta.KubeMetadataEntityID(metadata.EntityID.ID))
	if err != nil {
		return kubernetes.WorkloadTarget{}, false
	}
	for kind, gvr := range workloadMetadataResources {
		if gvr.Group == group && gvr.Resource == resource {
			return kubernetes.WorkloadTarget{Kind: kind, Namespace: namespace, Name: name}, true
		}
	}
	return kubernetes.WorkloadTarget{}, false
}
