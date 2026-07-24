// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

type kueueQueueParser struct {
	queueType workloadmeta.KueueQueueType
}

type kueueResourceFlavorParser struct{}

type kueueWorkloadParser struct{}

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
func NewKueueWorkloadParser() ObjectParser {
	return kueueWorkloadParser{}
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

	queueName, _, _ := unstructured.NestedString(u.Object, "spec", "queueName")
	clusterQueueName, _, _ := unstructured.NestedString(u.Object, "status", "admission", "clusterQueue")

	return &workloadmeta.KubernetesKueueWorkload{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesKueueWorkload,
			ID:   workloadmeta.GenerateKueueWorkloadEntityID(meta.Namespace, meta.Name),
		},
		EntityMeta:        meta,
		QueueName:         queueName,
		ClusterQueueName:  clusterQueueName,
		PodSetAssignments: kueuePodSetAssignments(u),
	}
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
