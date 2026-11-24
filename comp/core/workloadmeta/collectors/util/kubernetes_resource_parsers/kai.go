// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// kaiPodGroupExcludedAnnotations are dropped rather than streamed to every node:
// the first is copied by KAI from the top owner and holds its whole spec, the
// second changes every time the PodGroup is scheduled again. The Cluster Agent
// reads the times they hold before they are dropped when it has to send events.
var kaiPodGroupExcludedAnnotations = []string{
	"kubectl.kubernetes.io/last-applied-configuration",
	kaiLastStartAnnotation,
}

const (
	kaiLastStartAnnotation    = "kai.scheduler/last-start-timestamp"
	kaiLastEvictionAnnotation = "kai.scheduler/last-eviction-timestamp"
)

type kaiPodGroupParser struct {
	events bool
}

// NewKAIPodGroupParser returns a parser for KAI Scheduler PodGroup resources.
// events fills the start and eviction times and the scheduling conditions that
// the events of the Cluster Agent are built from. They change on every status
// update, so they are left out when nothing needs them, to keep the store from
// notifying its subscribers for them.
func NewKAIPodGroupParser(events bool) ObjectParser {
	return kaiPodGroupParser{events: events}
}

func (p kaiPodGroupParser) Parse(obj interface{}) workloadmeta.Entity {
	u := obj.(*unstructured.Unstructured)
	annotations := u.GetAnnotations()
	// The excluded annotations are dropped from the entity metadata, but the
	// Cluster Agent still needs the times they hold to emit events.
	var lastStartTime, lastEvictionTime time.Time
	var schedulingConditions []workloadmeta.KAISchedulingCondition
	if p.events {
		lastStartTime = parseTime(annotations[kaiLastStartAnnotation])
		lastEvictionTime = parseTime(annotations[kaiLastEvictionAnnotation])
		schedulingConditions = kaiSchedulingConditions(u)
	}
	for _, annotation := range kaiPodGroupExcludedAnnotations {
		delete(annotations, annotation)
	}

	spec, _ := u.Object["spec"].(map[string]interface{})

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
		QueueName:            stringField(spec, "queue"),
		CreationTimestamp:    u.GetCreationTimestamp().UTC(),
		PriorityClassName:    priorityClassName(spec),
		LastStartTime:        lastStartTime,
		LastEvictionTime:     lastEvictionTime,
		SchedulingConditions: schedulingConditions,
	}
}

// kaiSchedulingConditions returns the entries of status.schedulingConditions
// of a PodGroup that have a type.
func kaiSchedulingConditions(u *unstructured.Unstructured) []workloadmeta.KAISchedulingCondition {
	var conditions []workloadmeta.KAISchedulingCondition
	for _, fields := range typedEntries(u.Object, "status", "schedulingConditions") {
		var explanations []workloadmeta.KAISchedulingExplanation
		rawExplanations, _ := fields["reasons"].([]interface{})
		for _, rawExplanation := range rawExplanations {
			if explanationFields, ok := rawExplanation.(map[string]interface{}); ok {
				explanations = append(explanations, workloadmeta.KAISchedulingExplanation{
					Reason:  stringField(explanationFields, "reason"),
					Message: stringField(explanationFields, "message"),
				})
			}
		}

		conditions = append(conditions, workloadmeta.KAISchedulingCondition{
			Type:               stringField(fields, "type"),
			Status:             stringField(fields, "status"),
			NodePool:           stringField(fields, "nodePool"),
			Reason:             stringField(fields, "reason"),
			Message:            stringField(fields, "message"),
			Explanations:       explanations,
			LastTransitionTime: parseTime(stringField(fields, "lastTransitionTime")),
		})
	}
	return conditions
}
