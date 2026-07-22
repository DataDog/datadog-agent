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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ContainerCreationHandler handles new container creation events.
type ContainerCreationHandler struct {
	store workloadmeta.Component

	mu sync.Mutex
	// seen maps a container ID to whether the event emitted for it carried owner info.
	seen map[string]bool
}

// NewContainerCreationHandler returns a ContainerCreationHandler backed by the given store.
func NewContainerCreationHandler(store workloadmeta.Component) *ContainerCreationHandler {
	return &ContainerCreationHandler{store: store, seen: make(map[string]bool)}
}

// String returns a human-readable name for the handler.
func (h *ContainerCreationHandler) String() string {
	return "ContainerCreationHandler"
}

// CanHandle reports whether this handler processes the given event.
func (h *ContainerCreationHandler) CanHandle(ev workloadmeta.Event) bool {
	return ev.Entity.GetID().Kind == workloadmeta.KindContainer
}

// Handle builds a LifecycleEvent for a newly created container, and clears
// local state once workloadmeta reports the container as unset.
func (h *ContainerCreationHandler) Handle(ev workloadmeta.Event) ([]LifecycleEvent, error) {
	containerID := ev.Entity.GetID().ID

	if ev.Type == workloadmeta.EventTypeUnset {
		// We could get an unset event for removal at the runtime source, but
		// not yet at the node orchestrator source. If the container still exists,
		// we don't delete yet.
		if _, err := h.store.GetContainer(containerID); err == nil {
			return nil, nil
		}

		h.mu.Lock()
		delete(h.seen, containerID)
		h.mu.Unlock()
		return nil, nil
	}

	container, ok := ev.Entity.(*workloadmeta.Container)
	if !ok {
		return nil, fmt.Errorf("expected *workloadmeta.Container, got %T", ev.Entity)
	}

	name := container.EntityMeta.Name
	ctr := &contlcycle.ContainerEvent{
		ContainerID:   containerID,
		Source:        string(workloadmeta.SourceRuntime),
		ContainerName: &name,
	}

	if !container.State.CreatedAt.IsZero() {
		ts := container.State.CreatedAt.Unix()
		ctr.OptionalCreationTimestamp = &contlcycle.ContainerEvent_CreationTimestamp{CreationTimestamp: ts}
	}

	// We need to query the store to get the container owner.
	if c, err := h.store.GetContainer(containerID); err == nil {
		if c.Owner != nil {
			switch c.Owner.Kind {
			case workloadmeta.KindKubernetesPod:
				if ownerKind, err := kindToModel(types.ObjectKindPod); err == nil {
					ctr.Owner = &contlcycle.ContainerEvent_Owner{OwnerType: ownerKind, OwnerUID: c.Owner.ID}
				}
			case workloadmeta.KindECSTask:
				if ownerKind, err := kindToModel(types.ObjectKindTask); err == nil {
					ctr.Owner = &contlcycle.ContainerEvent_Owner{OwnerType: ownerKind, OwnerUID: c.Owner.ID}
				}
			default:
				log.Tracef("Cannot handle owner for container %q with type %q", containerID, c.Owner.Kind)
			}
		}
	}

	h.mu.Lock()
	hadOwner, alreadySeen := h.seen[containerID]
	if alreadySeen && (hadOwner || ctr.Owner == nil) {
		h.mu.Unlock()
		return nil, nil
	}
	h.seen[containerID] = ctr.Owner != nil
	h.mu.Unlock()

	return []LifecycleEvent{{
		ObjectKind: types.ObjectKindContainer,
		ProtoEvent: &contlcycle.Event{
			EventType:  contlcycle.Event_Create,
			TypedEvent: &contlcycle.Event_Container{Container: ctr},
		},
	}}, nil
}
