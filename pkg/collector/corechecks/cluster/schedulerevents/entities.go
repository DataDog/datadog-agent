// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package schedulerevents

import (
	kubernetesresourceparsers "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

type (
	workloadCondition   = kubernetesresourceparsers.KueueWorkloadCondition
	podGroupCondition   = kubernetesresourceparsers.KAISchedulingCondition
	podGroupExplanation = kubernetesresourceparsers.KAISchedulingExplanation
)

// kueueView is a Kueue Workload entity with the data of its SchedulerEvents.
// Both are embedded, so the check reads their fields as one object.
type kueueView struct {
	*workloadmeta.KubernetesKueueWorkload
	*kubernetesresourceparsers.KueueWorkloadEvents
}

// newKueueView returns the view of a Workload. The events are empty when the
// entity has none, which is the case when it did not come from the parser of
// the Cluster Agent.
func newKueueView(w *workloadmeta.KubernetesKueueWorkload) *kueueView {
	events, ok := w.SchedulerEvents.(*kubernetesresourceparsers.KueueWorkloadEvents)
	if !ok || events == nil {
		events = &kubernetesresourceparsers.KueueWorkloadEvents{}
	}
	return &kueueView{w, events}
}

// kaiView is a KAI PodGroup entity with the data of its SchedulerEvents.
type kaiView struct {
	*workloadmeta.KubernetesKAIPodGroup
	*kubernetesresourceparsers.KAIPodGroupEvents
}

// newKAIView returns the view of a PodGroup. The events are empty when the
// entity has none.
func newKAIView(pg *workloadmeta.KubernetesKAIPodGroup) *kaiView {
	events, ok := pg.SchedulerEvents.(*kubernetesresourceparsers.KAIPodGroupEvents)
	if !ok || events == nil {
		events = &kubernetesresourceparsers.KAIPodGroupEvents{}
	}
	return &kaiView{pg, events}
}
