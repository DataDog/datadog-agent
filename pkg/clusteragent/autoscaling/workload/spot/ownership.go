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

type objectKey struct {
	Namespace string
	Kind      string
	Name      string
}

func (o objectKey) String() string {
	return o.Kind + " " + o.Namespace + "/" + o.Name
}

// resolveCoreV1PodOwner resolves the direct owner for a corev1.Pod.
// It returns the ReplicaSet, StatefulSet, or other direct controller as-is.
// Using the direct owner rather than the top-level workload (Deployment) ensures
// that pods from different ReplicaSets during a rolling update are counted independently,
// giving each revision a fresh spot/on-demand ratio calculation.
func resolveCoreV1PodOwner(pod *corev1.Pod) (objectKey, bool) {
	if len(pod.OwnerReferences) == 0 {
		return objectKey{}, false
	}

	ownerRef := pod.OwnerReferences[0]

	// Ignore pods owned directly by a Deployment
	if ownerRef.Kind == kubernetes.DeploymentKind {
		return objectKey{}, false
	}

	return objectKey{Namespace: pod.Namespace, Kind: ownerRef.Kind, Name: ownerRef.Name}, true
}

// resolveWLMPodOwner resolves the direct owner for a workloadmeta KubernetesPod.
// See [resolvePodOwner] for the rationale of using the direct owner.
func resolveWLMPodOwner(pod *workloadmeta.KubernetesPod) (objectKey, bool) {
	if len(pod.Owners) == 0 {
		return objectKey{}, false
	}

	owner := pod.Owners[0]

	// Ignore pods owned directly by a Deployment
	if owner.Kind == kubernetes.DeploymentKind {
		return objectKey{}, false
	}

	return objectKey{Namespace: pod.Namespace, Kind: owner.Kind, Name: owner.Name}, true
}
