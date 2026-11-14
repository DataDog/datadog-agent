// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package kueue provides helpers for Kueue metadata on workloadmeta entities.
package kueue

import (
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// WorkloadName returns an explicit Kueue Workload reference from pod
// metadata. Admission annotations take precedence over prebuilt Workload and
// pod-group references. Annotation forms take precedence over label forms.
func WorkloadName(pod *workloadmeta.KubernetesPod) string {
	for _, name := range []string{
		pod.Annotations[kubernetes.KueueWorkloadAnnotationKey],
		pod.Annotations[kubernetes.KueuePrebuiltWorkloadNameKey],
		pod.Labels[kubernetes.KueuePrebuiltWorkloadNameKey],
		pod.Annotations[kubernetes.KueuePodGroupNameLabelKey],
		pod.Labels[kubernetes.KueuePodGroupNameLabelKey],
	} {
		if name != "" {
			return name
		}
	}
	return ""
}

// IsManagedPod reports whether Kueue's plain Pod integration manages this pod.
// Controller-created pods can reference a Kueue Workload without this label.
func IsManagedPod(pod *workloadmeta.KubernetesPod) bool {
	return pod.Labels[kubernetes.KueueManagedLabelKey] == "true"
}

// WorkloadMatchesPod reports whether the pod refers to this Workload.
// Explicit names take precedence. Otherwise, the plain Pod integration creates
// a Workload owned by the pod and records the pod UID in its job-uid label,
// even when the pod itself is owned by a controller such as RayCluster.
func WorkloadMatchesPod(workload *workloadmeta.KubernetesKueueWorkload, pod *workloadmeta.KubernetesPod) bool {
	if workload.Namespace != pod.Namespace {
		return false
	}
	if name := WorkloadName(pod); name != "" {
		return workload.Name == name
	}
	return IsManagedPod(pod) && pod.ID != "" && workload.Labels[kubernetes.KueueJobUIDLabelKey] == pod.ID
}
