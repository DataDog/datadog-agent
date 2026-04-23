// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/contlcycle"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	types "github.com/DataDog/datadog-agent/pkg/containerlifecycle"
)

// PodTerminationHandler handles workloadmeta unset-pod events.
type PodTerminationHandler struct{}

// String returns a human-readable name for the handler.
func (h *PodTerminationHandler) String() string {
	return "PodTerminationHandler"
}

// CanHandle reports whether this handler processes the given event.
func (h *PodTerminationHandler) CanHandle(ev workloadmeta.Event) bool {
	return ev.Type == workloadmeta.EventTypeUnset &&
		ev.Entity.GetID().Kind == workloadmeta.KindKubernetesPod
}

// Handle builds a LifecycleEvent for a pod termination.
func (h *PodTerminationHandler) Handle(ev workloadmeta.Event) ([]LifecycleEvent, error) {
	pod, ok := ev.Entity.(*workloadmeta.KubernetesPod)
	if !ok {
		return nil, fmt.Errorf("expected *workloadmeta.KubernetesPod, got %T", ev.Entity)
	}

	podEvent := &contlcycle.PodEvent{
		PodUID: pod.GetID().ID,
		Source: string(workloadmeta.SourceNodeOrchestrator),
	}

	if !pod.FinishedAt.IsZero() {
		ts := pod.FinishedAt.Unix()
		podEvent.ExitTimestamp = &ts
	}

	return []LifecycleEvent{{
		ObjectKind: types.ObjectKindPod,
		ProtoEvent: &contlcycle.Event{
			EventType:  contlcycle.Event_Delete,
			TypedEvent: &contlcycle.Event_Pod{Pod: podEvent},
		},
	}}, nil
}
