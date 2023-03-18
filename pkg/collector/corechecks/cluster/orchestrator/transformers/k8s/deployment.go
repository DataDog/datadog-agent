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
	corev1 "k8s.io/api/core/v1"
)

// ExtractDeployment returns the protobuf model corresponding to a Kubernetes
// Deployment resource.
func ExtractDeployment(d *appsv1.Deployment) *model.Deployment {
	deploy := model.Deployment{
		Metadata: extractMetadata(&d.ObjectMeta),
	}
	// spec
	deploy.ReplicasDesired = 1 // default
	if d.Spec.Replicas != nil {
		deploy.ReplicasDesired = *d.Spec.Replicas
	}
	deploy.Paused = d.Spec.Paused
	deploy.DeploymentStrategy = string(d.Spec.Strategy.Type)
	if deploy.DeploymentStrategy == "RollingUpdate" && d.Spec.Strategy.RollingUpdate != nil {
		if d.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
			deploy.MaxUnavailable = d.Spec.Strategy.RollingUpdate.MaxUnavailable.StrVal
		}
		if d.Spec.Strategy.RollingUpdate.MaxSurge != nil {
			deploy.MaxSurge = d.Spec.Strategy.RollingUpdate.MaxSurge.StrVal
		}
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
	deploy.ConditionMessage = extractDeploymentConditionMessage(d.Status.Conditions)

	deploy.ResourceRequirements = ExtractPodTemplateResourceRequirements(d.Spec.Template)
	deploy.Tags = append(deploy.Tags, transformers.RetrieveUnifiedServiceTags(d.ObjectMeta.Labels)...)

	return &deploy
}

func extractDeploymentConditionMessage(conditions []appsv1.DeploymentCondition) string {
	messageMap := make(map[appsv1.DeploymentConditionType]string)

	// from https://github.com/kubernetes/kubernetes/blob/0b678bbb51a83e47df912f1205907418e354b281/staging/src/k8s.io/api/apps/appsv1/types.go#L417-L430
	// update if new ones appear
	chronologicalConditions := []appsv1.DeploymentConditionType{
		appsv1.DeploymentReplicaFailure,
		appsv1.DeploymentProgressing,
		appsv1.DeploymentAvailable,
	}

	// populate messageMap with messages for non-passing conditions
	for _, c := range conditions {
		if c.Status == corev1.ConditionFalse && c.Message != "" {
			messageMap[c.Type] = c.Message
		}
	}

	// return the message of the first one that failed
	for _, c := range chronologicalConditions {
		if m := messageMap[c]; m != "" {
			return m
		}
	}
	return ""
}
