// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package external

import (
	"fmt"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func newFakeWLMPodEvent(ns, deployment, podName string, containerNames []string) workloadmeta.Event {
	containers := []workloadmeta.OrchestratorContainer{}
	for _, c := range containerNames {
		containers = append(containers, workloadmeta.OrchestratorContainer{
			ID:   fmt.Sprintf("%s-id", c),
			Name: c,
			Resources: workloadmeta.ContainerResources{
				CPURequest:    func(f float64) *float64 { return &f }(25), // 250m
				MemoryRequest: func(f uint64) *uint64 { return &f }(2048),
			},
		})
	}

	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   podName,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      podName,
			Namespace: ns,
		},
		Owners:     []workloadmeta.KubernetesPodOwner{{Kind: kubernetes.ReplicaSetKind, Name: fmt.Sprintf("%s-766dbb7846", deployment)}},
		Containers: containers,
		Ready:      true,
	}

	return workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: pod,
	}
}
