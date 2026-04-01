// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	corev1 "k8s.io/api/core/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

type (
	objectKey struct {
		Kind      string
		Namespace string
		Name      string
	}

	podOwner objectKey // Direct pod owner, e.g. ReplicaSet
	workload objectKey // Workload, e.g. Deployment

	// podGroup combines a pod's direct owner with its top-level workload.
	podGroup struct {
		owner    podOwner
		workload workload
	}
)

func (o objectKey) String() string {
	return o.Kind + " " + o.Namespace + "/" + o.Name
}

func (o podOwner) String() string {
	return objectKey(o).String()
}

func (o workload) String() string {
	return objectKey(o).String()
}

// resolveCoreV1PodOwner resolves the direct owner for a corev1.Pod.
// It returns the ReplicaSet, StatefulSet, or other direct controller as-is.
// Using the direct owner rather than the top-level workload (Deployment) ensures
// that pods from different ReplicaSets during a rolling update are counted independently,
// giving each revision a fresh spot/on-demand ratio calculation.
func resolveCoreV1PodOwner(pod *corev1.Pod) (podOwner, bool) {
	if len(pod.OwnerReferences) == 0 {
		return podOwner{}, false
	}

	ownerRef := pod.OwnerReferences[0]

	// Ignore pods owned directly by a Deployment
	if ownerRef.Kind == kubernetes.DeploymentKind {
		return podOwner{}, false
	}

	return podOwner{Kind: ownerRef.Kind, Namespace: pod.Namespace, Name: ownerRef.Name}, true
}

// resolveWLMPodOwner resolves the direct owner for a workloadmeta KubernetesPod.
// See [resolveCoreV1PodOwner] for the rationale of using the direct owner.
func resolveWLMPodOwner(pod *workloadmeta.KubernetesPod) (podOwner, bool) {
	if len(pod.Owners) == 0 {
		return podOwner{}, false
	}

	owner := pod.Owners[0]

	// Ignore pods owned directly by a Deployment
	if owner.Kind == kubernetes.DeploymentKind {
		return podOwner{}, false
	}

	return podOwner{Kind: owner.Kind, Namespace: pod.Namespace, Name: owner.Name}, true
}

// resolveCoreV1PodGroup resolves both the direct owner and the top-level workload for a corev1.Pod.
func resolveCoreV1PodGroup(pod *corev1.Pod) (podGroup, bool) {
	owner, ok := resolveCoreV1PodOwner(pod)
	if !ok {
		return podGroup{}, false
	}
	w, ok := resolveOwnerWorkload(owner)
	if !ok {
		return podGroup{}, false
	}
	return podGroup{owner: owner, workload: w}, true
}

// resolveWLMPodGroup resolves both the direct owner and the top-level workload for a workloadmeta KubernetesPod.
func resolveWLMPodGroup(pod *workloadmeta.KubernetesPod) (podGroup, bool) {
	owner, ok := resolveWLMPodOwner(pod)
	if !ok {
		return podGroup{}, false
	}
	w, ok := resolveOwnerWorkload(owner)
	if !ok {
		return podGroup{}, false
	}
	return podGroup{owner: owner, workload: w}, true
}

// resolveOwnerWorkload maps a direct pod owner to its top-level workload.
// For ReplicaSets it resolves to the parent Deployment; other kinds map 1:1.
// Returns false if resolution fails.
func resolveOwnerWorkload(owner podOwner) (workload, bool) {
	if owner.Kind == kubernetes.ReplicaSetKind {
		deploymentName := kubernetes.ParseDeploymentForReplicaSet(owner.Name)
		if deploymentName == "" {
			return workload{}, false
		}
		return workload{Kind: kubernetes.DeploymentKind, Namespace: owner.Namespace, Name: deploymentName}, true
	}
	return workload(owner), true
}
