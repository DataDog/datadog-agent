// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package transformers

import (
	model "github.com/DataDog/agent-payload/v5/process"
	appsv1 "k8s.io/api/apps/v1"
)

// ExtractK8sDaemonSet returns the protobuf model corresponding to a Kubernetes
// DaemonSet resource.
func ExtractK8sDaemonSet(ds *appsv1.DaemonSet) *model.DaemonSet {
	daemonSet := model.DaemonSet{
		Metadata: extractMetadata(&ds.ObjectMeta),
		Spec: &model.DaemonSetSpec{
			MinReadySeconds: ds.Spec.MinReadySeconds,
		},
		Status: &model.DaemonSetStatus{
			CurrentNumberScheduled: ds.Status.CurrentNumberScheduled,
			NumberMisscheduled:     ds.Status.NumberMisscheduled,
			DesiredNumberScheduled: ds.Status.DesiredNumberScheduled,
			NumberReady:            ds.Status.NumberReady,
			UpdatedNumberScheduled: ds.Status.UpdatedNumberScheduled,
			NumberAvailable:        ds.Status.NumberAvailable,
			NumberUnavailable:      ds.Status.NumberUnavailable,
		},
	}

	if ds.Spec.RevisionHistoryLimit != nil {
		daemonSet.Spec.RevisionHistoryLimit = *ds.Spec.RevisionHistoryLimit
	}

	daemonSet.Spec.DeploymentStrategy = string(ds.Spec.UpdateStrategy.Type)
	if ds.Spec.UpdateStrategy.Type == "RollingUpdate" && ds.Spec.UpdateStrategy.RollingUpdate != nil {
		if ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable != nil {
			daemonSet.Spec.MaxUnavailable = ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable.StrVal
		}
	}

	if ds.Spec.Selector != nil {
		daemonSet.Spec.Selectors = extractLabelSelector(ds.Spec.Selector)
	}

	return &daemonSet
}
