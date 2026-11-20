// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// kaiPodGroupExcludedAnnotations are dropped rather than streamed to every node:
// the first is copied by KAI from the top owner and holds its whole spec, the
// second changes every time the PodGroup is scheduled again.
var kaiPodGroupExcludedAnnotations = []string{
	"kubectl.kubernetes.io/last-applied-configuration",
	kaiLastStartAnnotation,
}

const (
	kaiLastStartAnnotation    = "kai.scheduler/last-start-timestamp"
	kaiLastEvictionAnnotation = "kai.scheduler/last-eviction-timestamp"
)

type kaiPodGroupParser struct{}

// NewKAIPodGroupParser returns a parser for KAI Scheduler PodGroup resources.
func NewKAIPodGroupParser() ObjectParser {
	return kaiPodGroupParser{}
}

func (p kaiPodGroupParser) Parse(obj interface{}) workloadmeta.Entity {
	u := obj.(*unstructured.Unstructured)
	annotations := u.GetAnnotations()
	// The excluded annotations are dropped from the entity metadata, but the
	// Cluster Agent still needs the times they hold to emit events.
	lastStartTime := parseRFC3339(annotations[kaiLastStartAnnotation])
	lastEvictionTime := parseRFC3339(annotations[kaiLastEvictionAnnotation])
	for _, annotation := range kaiPodGroupExcludedAnnotations {
		delete(annotations, annotation)
	}

	queueName, _, _ := unstructured.NestedString(u.Object, "spec", "queue")
	priorityClassName, _, _ := unstructured.NestedString(u.Object, "spec", "priorityClassName")

	return &workloadmeta.KubernetesKAIPodGroup{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesKAIPodGroup,
			ID:   workloadmeta.GenerateKAIPodGroupEntityID(u.GetNamespace(), u.GetName()),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        u.GetName(),
			Namespace:   u.GetNamespace(),
			Labels:      u.GetLabels(),
			Annotations: annotations,
			UID:         string(u.GetUID()),
		},
		QueueName:            queueName,
		CreationTimestamp:    u.GetCreationTimestamp().UTC(),
		PriorityClassName:    priorityClassName,
		LastStartTime:        lastStartTime,
		LastEvictionTime:     lastEvictionTime,
		SchedulingConditions: kaiSchedulingConditions(u),
	}
}

// kaiSchedulingConditions returns the entries of status.schedulingConditions
// of a PodGroup that have a type.
func kaiSchedulingConditions(u *unstructured.Unstructured) []workloadmeta.KAISchedulingCondition {
	items, _, _ := unstructured.NestedSlice(u.Object, "status", "schedulingConditions")

	var conditions []workloadmeta.KAISchedulingCondition
	for _, item := range items {
		fields, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		conditionType, _, _ := unstructured.NestedString(fields, "type")
		if conditionType == "" {
			continue
		}
		status, _, _ := unstructured.NestedString(fields, "status")
		nodePool, _, _ := unstructured.NestedString(fields, "nodePool")
		reason, _, _ := unstructured.NestedString(fields, "reason")
		message, _, _ := unstructured.NestedString(fields, "message")
		transitionTime, _, _ := unstructured.NestedString(fields, "lastTransitionTime")

		var explanations []workloadmeta.KAISchedulingExplanation
		rawExplanations, _, _ := unstructured.NestedSlice(fields, "reasons")
		for _, rawExplanation := range rawExplanations {
			explanationFields, ok := rawExplanation.(map[string]interface{})
			if !ok {
				continue
			}
			explanationReason, _, _ := unstructured.NestedString(explanationFields, "reason")
			explanationMessage, _, _ := unstructured.NestedString(explanationFields, "message")
			explanations = append(explanations, workloadmeta.KAISchedulingExplanation{
				Reason:  explanationReason,
				Message: explanationMessage,
			})
		}

		conditions = append(conditions, workloadmeta.KAISchedulingCondition{
			Type:               conditionType,
			Status:             status,
			NodePool:           nodePool,
			Reason:             reason,
			Message:            message,
			Explanations:       explanations,
			LastTransitionTime: parseRFC3339(transitionTime),
		})
	}
	return conditions
}
