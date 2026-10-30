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
	"kai.scheduler/last-start-timestamp",
}

type kaiPodGroupParser struct{}

// NewKAIPodGroupParser returns a parser for KAI Scheduler PodGroup resources.
func NewKAIPodGroupParser() ObjectParser {
	return kaiPodGroupParser{}
}

func (p kaiPodGroupParser) Parse(obj interface{}) workloadmeta.Entity {
	u := obj.(*unstructured.Unstructured)
	annotations := u.GetAnnotations()
	for _, annotation := range kaiPodGroupExcludedAnnotations {
		delete(annotations, annotation)
	}

	queueName, _, _ := unstructured.NestedString(u.Object, "spec", "queue")

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
		QueueName: queueName,
	}
}
