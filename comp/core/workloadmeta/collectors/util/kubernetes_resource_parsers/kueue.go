// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

type kueueQueueParser struct {
	queueType workloadmeta.KueueQueueType
}

// NewKueueQueueParser returns a parser for Kueue queue resources.
func NewKueueQueueParser(queueType workloadmeta.KueueQueueType) ObjectParser {
	return kueueQueueParser{queueType: queueType}
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
			ID:   GenerateKueueQueueEntityID(p.queueType, meta.Namespace, meta.Name),
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

// GenerateKueueQueueEntityID returns the workloadmeta entity ID for a Kueue queue.
func GenerateKueueQueueEntityID(queueType workloadmeta.KueueQueueType, namespace, name string) string {
	switch queueType {
	case workloadmeta.KueueLocalQueue:
		return string(queueType) + "/" + namespace + "/" + name
	case workloadmeta.KueueClusterQueue:
		return string(queueType) + "//" + name
	default:
		return string(queueType) + "/" + namespace + "/" + name
	}
}

// QueueTypeForKueueResource returns the workloadmeta queue type for a Kueue resource name.
func QueueTypeForKueueResource(resource string) (workloadmeta.KueueQueueType, bool) {
	switch resource {
	case kubernetes.KueueLocalQueueResourceName:
		return workloadmeta.KueueLocalQueue, true
	case kubernetes.KueueClusterQueueResourceName:
		return workloadmeta.KueueClusterQueue, true
	default:
		return "", false
	}
}

// KueueQueueDeletionMeta returns Kubernetes object metadata used by generic reflector stores.
func KueueQueueDeletionMeta(obj interface{}) metav1.Object {
	return obj.(metav1.Object)
}
