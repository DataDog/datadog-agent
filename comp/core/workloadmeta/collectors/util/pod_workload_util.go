// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package util

import (
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
