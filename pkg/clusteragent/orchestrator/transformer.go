// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver,orchestrator

package orchestrator

import (
	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/util/orchestrator"

	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func extractDeployment(d *v1.Deployment) *model.Deployment {
	deploy := model.Deployment{
		Metadata: orchestrator.ExtractMetadata(&d.ObjectMeta),
	}
	// spec
	deploy.ReplicasDesired = 1 // default
	if d.Spec.Replicas != nil {
		deploy.ReplicasDesired = *d.Spec.Replicas
	}
	deploy.Paused = d.Spec.Paused
	deploy.DeploymentStrategy = string(d.Spec.Strategy.Type)
	if deploy.DeploymentStrategy == "RollingUpdate" {
		deploy.MaxUnavailable = d.Spec.Strategy.RollingUpdate.MaxUnavailable.StrVal
		deploy.MaxSurge = d.Spec.Strategy.RollingUpdate.MaxSurge.StrVal
	}
	if d.Spec.Selector != nil {
		deploy.Selectors = extractLabelSelector(d.Spec.Selector)
	}

	// status
	deploy.Replicas = d.Status.Replicas
	deploy.UpdatedReplicas = d.Status.UpdatedReplicas
	deploy.ReadyReplicas = d.Status.ReadyReplicas
	deploy.AvailableReplicas = d.Status.AvailableReplicas
	deploy.UnavailableReplicas = d.Status.UnavailableReplicas
	for i, c := range d.Status.Conditions {
		deploy.ConditionMessage += c.Message
		if i != 0 && i != len(d.Status.Conditions)-1 {
			deploy.ConditionMessage += " "
		}
	}

	return &deploy
}

func extractReplicaset(rs *v1.ReplicaSet) *model.ReplicaSet {
	replicaSet := model.ReplicaSet{
		Metadata: orchestrator.ExtractMetadata(&rs.ObjectMeta),
	}
	// spec
	replicaSet.ReplicasDesired = *rs.Spec.Replicas
	if rs.Spec.Selector != nil {
		replicaSet.Selectors = extractLabelSelector(rs.Spec.Selector)
	}

	// status
	replicaSet.Replicas = rs.Status.Replicas
	replicaSet.FullyLabeledReplicas = rs.Status.FullyLabeledReplicas
	replicaSet.ReadyReplicas = rs.Status.ReadyReplicas
	replicaSet.AvailableReplicas = rs.Status.AvailableReplicas

	return &replicaSet
}

func extractLabelSelector(ls *metav1.LabelSelector) []*model.LabelSelectorRequirement {
	labelSelectors := make([]*model.LabelSelectorRequirement, 0, len(ls.MatchLabels)+len(ls.MatchExpressions))
	for k, v := range ls.MatchLabels {
		s := model.LabelSelectorRequirement{
			Key:      k,
			Operator: "In",
			Values:   []string{v},
		}
		labelSelectors = append(labelSelectors, &s)
	}
	for _, s := range ls.MatchExpressions {
		sr := model.LabelSelectorRequirement{
			Key:      s.Key,
			Operator: string(s.Operator),
			Values:   s.Values,
		}
		labelSelectors = append(labelSelectors, &sr)
	}

	return labelSelectors
}
