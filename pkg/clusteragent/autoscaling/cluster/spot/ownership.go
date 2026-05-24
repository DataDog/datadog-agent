// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// objectRef identifies a Kubernetes object by group, kind, namespace, and name.
type objectRef struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
}

func (o objectRef) String() string {
	if o.Group != "" {
		return o.Group + "/" + o.Kind + " " + o.Namespace + "/" + o.Name
	}
	return o.Kind + " " + o.Namespace + "/" + o.Name
}

// podOwnership combines a pod's direct owner with its top-level workload controller.
type podOwnership struct {
	directOwner   objectRef
	topLevelOwner objectRef
}

// resolveCoreV1PodOwner resolves the direct owner for a corev1.Pod.
// It returns the ReplicaSet, StatefulSet, or other direct controller as-is.
// Using the direct owner rather than the top-level workload (Deployment) ensures
// that pods from different ReplicaSets during a rolling update are counted independently,
// giving each revision a fresh spot/on-demand ratio calculation.
func resolveCoreV1PodOwner(pod *corev1.Pod) (objectRef, bool) {
	if len(pod.OwnerReferences) == 0 {
		return objectRef{}, false
	}

	ownerRef := pod.OwnerReferences[0]

	// Ignore pods owned directly by a Deployment
	if ownerRef.Kind == kubernetes.DeploymentKind {
		return objectRef{}, false
	}

	gv, _ := schema.ParseGroupVersion(ownerRef.APIVersion)
	return objectRef{Group: gv.Group, Kind: ownerRef.Kind, Namespace: pod.Namespace, Name: ownerRef.Name}, true
}

// resolveWLMPodOwner resolves the direct owner for a workloadmeta KubernetesPod.
// See [resolveCoreV1PodOwner] for the rationale of using the direct owner.
func resolveWLMPodOwner(pod *workloadmeta.KubernetesPod) (objectRef, bool) {
	if len(pod.Owners) == 0 {
		return objectRef{}, false
	}

	owner := pod.Owners[0]

	// Ignore pods owned directly by a Deployment
	if owner.Kind == kubernetes.DeploymentKind {
		return objectRef{}, false
	}

	return objectRef{Group: owner.Group, Kind: owner.Kind, Namespace: pod.Namespace, Name: owner.Name}, true
}

// resolveCoreV1PodOwnership resolves both the direct owner and the top-level workload for a corev1.Pod.
func resolveCoreV1PodOwnership(pod *corev1.Pod) (podOwnership, bool) {
	owner, ok := resolveCoreV1PodOwner(pod)
	if !ok {
		return podOwnership{}, false
	}
	w, ok := resolveTopLevelOwner(owner)
	if !ok {
		return podOwnership{}, false
	}
	return podOwnership{directOwner: owner, topLevelOwner: w}, true
}

// resolveWLMPodOwnership resolves both the direct owner and the top-level workload for a workloadmeta KubernetesPod.
func resolveWLMPodOwnership(pod *workloadmeta.KubernetesPod) (podOwnership, bool) {
	owner, ok := resolveWLMPodOwner(pod)
	if !ok {
		return podOwnership{}, false
	}
	w, ok := resolveTopLevelOwner(owner)
	if !ok {
		return podOwnership{}, false
	}
	return podOwnership{directOwner: owner, topLevelOwner: w}, true
}

// resolveTopLevelOwner maps a direct pod owner to its top-level workload.
// For ReplicaSets it resolves to the parent Deployment; other kinds map 1:1.
// Returns false if resolution fails.
//
// TODO: Argo Rollouts also use ReplicaSets; their pods will resolve to a Deployment
// that does not exist. Add Argo Rollout detection when support is added.
func resolveTopLevelOwner(owner objectRef) (objectRef, bool) {
	if owner.Kind == kubernetes.ReplicaSetKind {
		deploymentName := kubernetes.ParseDeploymentForReplicaSet(owner.Name)
		// TODO: add support for ArgoRollout
		if deploymentName == "" {
			return objectRef{}, false
		}
		return objectRef{Group: owner.Group, Kind: kubernetes.DeploymentKind, Namespace: owner.Namespace, Name: deploymentName}, true
	}
	return owner, true
}
