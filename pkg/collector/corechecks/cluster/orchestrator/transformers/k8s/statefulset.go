// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"

	v1 "k8s.io/api/apps/v1"
)

// ExtractStatefulSet returns the protobuf model corresponding to a
// Kubernetes StatefulSet resource.
func ExtractStatefulSet(sts *v1.StatefulSet) *model.StatefulSet {
	statefulSet := model.StatefulSet{
		Metadata: extractMetadata(&sts.ObjectMeta),
		Spec: &model.StatefulSetSpec{
			ServiceName:         sts.Spec.ServiceName,
			PodManagementPolicy: string(sts.Spec.PodManagementPolicy),
			UpdateStrategy:      string(sts.Spec.UpdateStrategy.Type),
		},
		Status: &model.StatefulSetStatus{
			Replicas:        sts.Status.Replicas,
			ReadyReplicas:   sts.Status.ReadyReplicas,
			CurrentReplicas: sts.Status.CurrentReplicas,
			UpdatedReplicas: sts.Status.UpdatedReplicas,
		},
	}

	if sts.Spec.UpdateStrategy.Type == "RollingUpdate" && sts.Spec.UpdateStrategy.RollingUpdate != nil {
		if sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
			statefulSet.Spec.Partition = *sts.Spec.UpdateStrategy.RollingUpdate.Partition
		}
	}

	if sts.Spec.Replicas != nil {
		statefulSet.Spec.DesiredReplicas = *sts.Spec.Replicas
	}

	if sts.Spec.Selector != nil {
		statefulSet.Spec.Selectors = extractLabelSelector(sts.Spec.Selector)
	}

	statefulSet.Spec.ResourceRequirements = ExtractPodTemplateResourceRequirements(sts.Spec.Template)
	statefulSet.Tags = append(statefulSet.Tags, transformers.RetrieveUnifiedServiceTags(sts.ObjectMeta.Labels)...)

	return &statefulSet
}
