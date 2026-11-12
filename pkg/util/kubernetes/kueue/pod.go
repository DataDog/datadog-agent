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
