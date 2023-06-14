// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	appsv1 "k8s.io/api/apps/v1"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"
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

	if len(rs.Status.Conditions) > 0 {
		rsConditions, conditionTags := extractReplicaSetConditions(rs)
		replicaSet.Conditions = rsConditions
		replicaSet.Tags = append(replicaSet.Tags, conditionTags...)
	}

	replicaSet.ResourceRequirements = ExtractPodTemplateResourceRequirements(rs.Spec.Template)
	replicaSet.Tags = append(replicaSet.Tags, transformers.RetrieveUnifiedServiceTags(rs.ObjectMeta.Labels)...)

	return &replicaSet
}

// extractReplicaSetConditions iterates over replicaset conditions and returns:
// - the payload representation of those conditions
// - the list of tags that will enable pod filtering by condition
func extractReplicaSetConditions(p *appsv1.ReplicaSet) ([]*model.ReplicaSetCondition, []string) {
	conditions := make([]*model.ReplicaSetCondition, 0, len(p.Status.Conditions))
	conditionTags := make([]string, 0, len(p.Status.Conditions))

	for _, condition := range p.Status.Conditions {
		c := &model.ReplicaSetCondition{
			Message: condition.Message,
			Reason:  condition.Reason,
			Status:  string(condition.Status),
			Type:    string(condition.Type),
		}
		if !condition.LastTransitionTime.IsZero() {
			c.LastTransitionTime = condition.LastTransitionTime.Unix()
		}

		conditions = append(conditions, c)

		conditionTag := createConditionTag(string(condition.Type), string(condition.Status))
		conditionTags = append(conditionTags, conditionTag)
	}

	return conditions, conditionTags
}
