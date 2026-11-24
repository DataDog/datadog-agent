// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	"fmt"
	"maps"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

type kueueQueueParser struct {
	queueType workloadmeta.KueueQueueType
}

type kueueResourceFlavorParser struct{}

type kueueWorkloadParser struct {
	podTemplateTags bool
	events          bool
}

// NewKueueQueueParser returns a parser for Kueue queue resources.
func NewKueueQueueParser(queueType workloadmeta.KueueQueueType) (ObjectParser, error) {
	if err := validateKueueQueueType(queueType); err != nil {
		return nil, err
	}
	return kueueQueueParser{queueType: queueType}, nil
}

// NewKueueResourceFlavorParser returns a parser for Kueue ResourceFlavor resources.
func NewKueueResourceFlavorParser() ObjectParser {
	return kueueResourceFlavorParser{}
}

// NewKueueWorkloadParser returns a parser for Kueue Workload resources.
// podTemplateTags copies labels and annotations shared by every pod set template
// onto the workload entity. events fills the conditions that the events of the
// Cluster Agent are built from. They change on every status update, so they are
// left out when nothing needs them, to keep the store from notifying its
// subscribers for them.
func NewKueueWorkloadParser(podTemplateTags, events bool) ObjectParser {
	return kueueWorkloadParser{podTemplateTags: podTemplateTags, events: events}
}

func (p kueueQueueParser) Parse(obj interface{}) workloadmeta.Entity {
	u := obj.(*unstructured.Unstructured)
	meta := workloadmeta.EntityMeta{
		Name:        u.GetName(),
		Namespace:   u.GetNamespace(),
		Labels:      u.GetLabels(),
		Annotations: u.GetAnnotations(),
		UID:         string(u.GetUID()),
	}

	queue := &workloadmeta.KubernetesKueueQueue{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesKueueQueue,
			ID:   p.entityID(meta.Namespace, meta.Name),
		},
		EntityMeta: meta,
		QueueType:  p.queueType,
	}

	switch p.queueType {
	case workloadmeta.KueueLocalQueue:
		clusterQueue, _, _ := unstructured.NestedString(u.Object, "spec", "clusterQueue")
		queue.ClusterQueueName = clusterQueue
	case workloadmeta.KueueClusterQueue:
		queue.ClusterQueueName = meta.Name
	}

	return queue
}

func (p kueueQueueParser) entityID(namespace, name string) string {
	id, _ := workloadmeta.GenerateKueueQueueEntityID(p.queueType, namespace, name)
	return id
}

func (p kueueResourceFlavorParser) Parse(obj interface{}) workloadmeta.Entity {
	u := obj.(*unstructured.Unstructured)
	meta := workloadmeta.EntityMeta{
		Name:        u.GetName(),
		Namespace:   u.GetNamespace(),
		Labels:      u.GetLabels(),
		Annotations: u.GetAnnotations(),
		UID:         string(u.GetUID()),
	}

	nodeAffinityLabels, _, _ := unstructured.NestedStringMap(u.Object, "spec", "nodeLabels")

	return &workloadmeta.KubernetesKueueResourceFlavor{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesKueueResourceFlavor,
			ID:   workloadmeta.GenerateKueueResourceFlavorEntityID(meta.Name),
		},
		EntityMeta:         meta,
		NodeAffinityLabels: nodeAffinityLabels,
	}
}

func (p kueueWorkloadParser) Parse(obj interface{}) workloadmeta.Entity {
	u := obj.(*unstructured.Unstructured)
	meta := workloadmeta.EntityMeta{
		Name:        u.GetName(),
		Namespace:   u.GetNamespace(),
		Labels:      u.GetLabels(),
		Annotations: u.GetAnnotations(),
		UID:         string(u.GetUID()),
	}

	spec, _ := u.Object["spec"].(map[string]interface{})
	queueName := stringField(spec, "queueName")
	clusterQueueName, _, _ := unstructured.NestedString(u.Object, "status", "admission", "clusterQueue")
	var podTemplateLabels, podTemplateAnnotations map[string]string
	if p.podTemplateTags {
		podTemplateLabels, podTemplateAnnotations = kueuePodTemplateMetadata(u)
	}

	var priority *int32
	if value, found, err := unstructured.NestedInt64(u.Object, "spec", "priority"); found && err == nil {
		narrowed := int32(value)
		priority = &narrowed
	}

	var conditions []workloadmeta.KueueWorkloadCondition
	if p.events {
		conditions = kueueWorkloadConditions(u)
	}

	return &workloadmeta.KubernetesKueueWorkload{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesKueueWorkload,
			ID:   workloadmeta.GenerateKueueWorkloadEntityID(meta.Namespace, meta.Name),
		},
		EntityMeta:             meta,
		QueueName:              queueName,
		ClusterQueueName:       clusterQueueName,
		PodSetAssignments:      kueuePodSetAssignments(u),
		PodTemplateLabels:      podTemplateLabels,
		PodTemplateAnnotations: podTemplateAnnotations,
		CreationTimestamp:      u.GetCreationTimestamp().UTC(),
		Priority:               priority,
		PriorityClassName:      priorityClassName(spec),
		Conditions:             conditions,
	}
}

// kueueWorkloadConditions returns the status conditions of a Workload that
// have a type, in the order Kueue reports them.
func kueueWorkloadConditions(u *unstructured.Unstructured) []workloadmeta.KueueWorkloadCondition {
	var conditions []workloadmeta.KueueWorkloadCondition
	for _, fields := range typedEntries(u.Object, "status", "conditions") {
		conditions = append(conditions, workloadmeta.KueueWorkloadCondition{
			Type:               stringField(fields, "type"),
			Status:             stringField(fields, "status"),
			Reason:             stringField(fields, "reason"),
			Message:            stringField(fields, "message"),
			LastTransitionTime: parseTime(stringField(fields, "lastTransitionTime")),
		})
	}
	return conditions
}

// stringField returns the string at key, or "" when it is missing or not a string.
func stringField(fields map[string]interface{}, key string) string {
	value, _ := fields[key].(string)
	return value
}

// parseTime returns the zero time when the value is empty or malformed.
func parseTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339, value)
	return parsed
}

// priorityClassName reads priorityClassRef.name, the current field, and falls
// back to priorityClassName, the deprecated one.
func priorityClassName(spec map[string]interface{}) string {
	ref, _ := spec["priorityClassRef"].(map[string]interface{})
	if name := stringField(ref, "name"); name != "" {
		return name
	}
	return stringField(spec, "priorityClassName")
}

// typedEntries returns the objects of the list at path that have a type.
func typedEntries(obj map[string]interface{}, path ...string) []map[string]interface{} {
	items, _, _ := unstructured.NestedSlice(obj, path...)
	var entries []map[string]interface{}
	for _, item := range items {
		if fields, ok := item.(map[string]interface{}); ok && stringField(fields, "type") != "" {
			entries = append(entries, fields)
		}
	}
	return entries
}

// kueuePodTemplateMetadata returns labels and annotations that have the same
// value on every pod set template. A key that is missing or different on any
// pod set is dropped, so one pod set cannot overwrite another.
func kueuePodTemplateMetadata(u *unstructured.Unstructured) (map[string]string, map[string]string) {
	podSets, found, _ := unstructured.NestedSlice(u.Object, "spec", "podSets")
	if !found {
		return nil, nil
	}

	var labels, annotations map[string]string
	first := true
	for _, podSet := range podSets {
		podSetMap, ok := podSet.(map[string]interface{})
		if !ok {
			continue
		}

		podSetLabels, _, _ := unstructured.NestedStringMap(podSetMap, "template", "metadata", "labels")
		podSetAnnotations, _, _ := unstructured.NestedStringMap(podSetMap, "template", "metadata", "annotations")
		if first {
			labels = maps.Clone(podSetLabels)
			annotations = maps.Clone(podSetAnnotations)
			first = false
			continue
		}
		labels = matchingStringMap(labels, podSetLabels)
		annotations = matchingStringMap(annotations, podSetAnnotations)
	}

	return labels, annotations
}

func matchingStringMap(current, candidate map[string]string) map[string]string {
	if len(current) == 0 || len(candidate) == 0 {
		return nil
	}

	var matched map[string]string
	for key, value := range current {
		if candidate[key] != value {
			continue
		}
		if matched == nil {
			matched = make(map[string]string, len(current))
		}
		matched[key] = value
	}
	return matched
}

func kueuePodSetAssignments(u *unstructured.Unstructured) []workloadmeta.KueuePodSetAssignment {
	assignments, found, _ := unstructured.NestedSlice(u.Object, "status", "admission", "podSetAssignments")
	if !found {
		return nil
	}

	podSetAssignments := make([]workloadmeta.KueuePodSetAssignment, 0, len(assignments))
	for _, assignment := range assignments {
		assignmentMap, ok := assignment.(map[string]interface{})
		if !ok {
			continue
		}

		name, _, _ := unstructured.NestedString(assignmentMap, "name")
		flavors, _, _ := unstructured.NestedStringMap(assignmentMap, "flavors")
		podSetAssignments = append(podSetAssignments, workloadmeta.KueuePodSetAssignment{
			Name:    name,
			Flavors: flavors,
		})
	}

	return podSetAssignments
}

// GenerateKueueQueueEntityID returns the workloadmeta entity ID for a Kueue queue.
func GenerateKueueQueueEntityID(queueType workloadmeta.KueueQueueType, namespace, name string) (string, error) {
	return workloadmeta.GenerateKueueQueueEntityID(queueType, namespace, name)
}

// QueueTypeForKueueResource returns the workloadmeta queue type for a Kueue resource name.
func QueueTypeForKueueResource(resource string) (workloadmeta.KueueQueueType, error) {
	switch resource {
	case kubernetes.KueueLocalQueueResourceName:
		return workloadmeta.KueueLocalQueue, nil
	case kubernetes.KueueClusterQueueResourceName:
		return workloadmeta.KueueClusterQueue, nil
	default:
		return "", fmt.Errorf("unsupported Kueue resource %q", resource)
	}
}

func validateKueueQueueType(queueType workloadmeta.KueueQueueType) error {
	switch queueType {
	case workloadmeta.KueueLocalQueue, workloadmeta.KueueClusterQueue:
		return nil
	default:
		return fmt.Errorf("unsupported Kueue queue type %q", queueType)
	}
}
