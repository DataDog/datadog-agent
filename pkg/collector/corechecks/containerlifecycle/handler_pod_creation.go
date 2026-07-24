// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"fmt"
	"sync"

	"github.com/DataDog/agent-payload/v5/contlcycle"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	types "github.com/DataDog/datadog-agent/pkg/containerlifecycle"
)

// PodCreationHandler handles new pod creation events.
type PodCreationHandler struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

// NewPodCreationHandler returns a PodCreationHandler.
func NewPodCreationHandler() *PodCreationHandler {
	return &PodCreationHandler{seen: make(map[string]struct{})}
}

// String returns a human-readable name for the handler.
func (h *PodCreationHandler) String() string {
	return "PodCreationHandler"
}

// CanHandle reports whether this handler processes the given event.
func (h *PodCreationHandler) CanHandle(ev workloadmeta.Event) bool {
	return ev.Entity.GetID().Kind == workloadmeta.KindKubernetesPod
}

// Handle builds a LifecycleEvent for a newly created pod, and clears local
// state once workloadmeta reports the pod as unset.
func (h *PodCreationHandler) Handle(ev workloadmeta.Event) ([]LifecycleEvent, error) {
	podUID := ev.Entity.GetID().ID

	if ev.Type == workloadmeta.EventTypeUnset {
		h.mu.Lock()
		delete(h.seen, podUID)
		h.mu.Unlock()
		return nil, nil
	}

	pod, ok := ev.Entity.(*workloadmeta.KubernetesPod)
	if !ok {
		return nil, fmt.Errorf("expected *workloadmeta.KubernetesPod, got %T", ev.Entity)
	}

	h.mu.Lock()
	_, alreadySeen := h.seen[podUID]
	h.seen[podUID] = struct{}{}
	h.mu.Unlock()

	if alreadySeen {
		return nil, nil
	}

	podEvent := &contlcycle.PodEvent{
		PodUID: podUID,
		Source: string(workloadmeta.SourceNodeOrchestrator),
	}

	if !pod.CreationTimestamp.IsZero() {
		ts := pod.CreationTimestamp.Unix()
		podEvent.CreationTimestamp = &ts
	}

	return []LifecycleEvent{{
		ObjectKind: types.ObjectKindPod,
		ProtoEvent: &contlcycle.Event{
			EventType:  contlcycle.Event_Create,
			TypedEvent: &contlcycle.Event_Pod{Pod: podEvent},
		},
	}}, nil
}
