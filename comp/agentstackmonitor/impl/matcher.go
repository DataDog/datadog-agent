// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentstackmonitorimpl

import (
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// SubjectKind identifies which part of the Datadog agent stack a pod belongs to.
type SubjectKind string

const (
	SubjectKindClusterAgent       SubjectKind = "cluster_agent"
	SubjectKindClusterCheckRunner SubjectKind = "cluster_check_runner"
)

const (
	labelKey                = "app.kubernetes.io/name"
	clusterAgentValue       = "datadog-cluster-agent"
	clusterCheckRunnerValue = "datadog-cluster-checks-runner"
)

func subjectKindFor(pod *workloadmeta.KubernetesPod) (SubjectKind, bool) {
	if pod == nil {
		return "", false
	}
	switch pod.Labels[labelKey] {
	case clusterAgentValue:
		return SubjectKindClusterAgent, true
	case clusterCheckRunnerValue:
		return SubjectKindClusterCheckRunner, true
	}
	return "", false
}

// controllerRef anchors issue identity to the pod's owning workload so that
// reports survive pod restarts within the same Deployment revision.
type controllerRef struct {
	Namespace string
	Kind      string
	Name      string
}
