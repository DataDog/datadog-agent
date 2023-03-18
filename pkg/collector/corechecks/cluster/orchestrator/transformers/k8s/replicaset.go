// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"
	appsv1 "k8s.io/api/apps/v1"
)

// ExtractReplicaSet returns the protobuf model corresponding to a Kubernetes
// ReplicaSet resource.
func ExtractReplicaSet(rs *appsv1.ReplicaSet) *model.ReplicaSet {
	replicaSet := model.ReplicaSet{
		Metadata: extractMetadata(&rs.ObjectMeta),
	}
	// spec
	replicaSet.ReplicasDesired = 1 // default
	if rs.Spec.Replicas != nil {
		replicaSet.ReplicasDesired = *rs.Spec.Replicas
	}
	if rs.Spec.Selector != nil {
		replicaSet.Selectors = extractLabelSelector(rs.Spec.Selector)
	}

	// status
	replicaSet.Replicas = rs.Status.Replicas
	replicaSet.FullyLabeledReplicas = rs.Status.FullyLabeledReplicas
	replicaSet.ReadyReplicas = rs.Status.ReadyReplicas
	replicaSet.AvailableReplicas = rs.Status.AvailableReplicas

	replicaSet.ResourceRequirements = ExtractPodTemplateResourceRequirements(rs.Spec.Template)
	replicaSet.Tags = append(replicaSet.Tags, transformers.RetrieveUnifiedServiceTags(rs.ObjectMeta.Labels)...)

	return &replicaSet
}
