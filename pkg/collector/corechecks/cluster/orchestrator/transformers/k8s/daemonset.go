// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"
	appsv1 "k8s.io/api/apps/v1"

	model "github.com/DataDog/agent-payload/v5/process"
)

// ExtractDaemonSet returns the protobuf model corresponding to a Kubernetes
// DaemonSet resource.
func ExtractDaemonSet(ds *appsv1.DaemonSet) *model.DaemonSet {
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

	daemonSet.Spec.ResourceRequirements = ExtractPodTemplateResourceRequirements(ds.Spec.Template)
	daemonSet.Tags = append(daemonSet.Tags, transformers.RetrieveUnifiedServiceTags(ds.ObjectMeta.Labels)...)

	return &daemonSet
}
