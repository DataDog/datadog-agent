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

// PodCreationHandler handles pod creation events.
// It works by maintaining a shadow set of pod entity IDs we've already emitted
// a Create event for, so that subsequent workloadmeta Set events for the same
// pod don't re-fire. The shadow is pruned on Unset so it tracks live pods.
type PodCreationHandler struct {
	shadow map[workloadmeta.EntityID]struct{}
}

// NewPodCreationHandler returns an initialized PodCreationHandler.
func NewPodCreationHandler() *PodCreationHandler {
	return &PodCreationHandler{shadow: make(map[workloadmeta.EntityID]struct{})}
}

// String returns a human-readable name for the handler.
func (h *PodCreationHandler) String() string {
	return "PodCreationHandler"
}

// CanHandle reports whether this handler processes the given event.
// We accept Set events (to emit creation) and Unset events (to prune shadow).
func (h *PodCreationHandler) CanHandle(ev workloadmeta.Event) bool {
	if ev.Entity.GetID().Kind != workloadmeta.KindKubernetesPod {
		return false
	}
	return ev.Type == workloadmeta.EventTypeSet || ev.Type == workloadmeta.EventTypeUnset
}

// Handle builds a LifecycleEvent for a pod creation, or prunes the shadow on Unset.
func (h *PodCreationHandler) Handle(ev workloadmeta.Event) ([]LifecycleEvent, error) {
	pod, ok := ev.Entity.(*workloadmeta.KubernetesPod)
	if !ok {
		return nil, fmt.Errorf("expected *workloadmeta.KubernetesPod, got %T", ev.Entity)
	}

	podID := pod.GetID()

	if ev.Type == workloadmeta.EventTypeUnset {
		delete(h.shadow, podID)
		return nil, nil
	}

	// EventTypeSet: emit Create only the first time we see this pod.
	if _, seen := h.shadow[podID]; seen {
		return nil, nil
	}
	h.shadow[podID] = struct{}{}

	protoPodEvent := &contlcycle.PodEvent{
		PodUID: podID.ID,
		Source: string(workloadmeta.SourceNodeOrchestrator),
	}
	if !pod.CreationTimestamp.IsZero() {
		ts := pod.CreationTimestamp.Unix()
		protoPodEvent.CreationTimestamp = &ts
	}

	return []LifecycleEvent{{
		ObjectKind: types.ObjectKindPod,
		ProtoEvent: &contlcycle.Event{
			EventType:  contlcycle.Event_Create,
			TypedEvent: &contlcycle.Event_Pod{Pod: protoPodEvent},
		},
	}}, nil
}
