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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ContainerTerminationHandler handles workloadmeta unset-container events.
type ContainerTerminationHandler struct {
	store workloadmeta.Component
}

// NewContainerTerminationHandler returns a ContainerTerminationHandler backed by the given store.
func NewContainerTerminationHandler(store workloadmeta.Component) *ContainerTerminationHandler {
	return &ContainerTerminationHandler{store: store}
}

// String returns a human-readable name for the handler.
func (h *ContainerTerminationHandler) String() string {
	return "ContainerTerminationHandler"
}

// CanHandle reports whether this handler processes the given event.
func (h *ContainerTerminationHandler) CanHandle(ev workloadmeta.Event) bool {
	return ev.Type == workloadmeta.EventTypeUnset &&
		ev.Entity.GetID().Kind == workloadmeta.KindContainer
}

// Handle builds a LifecycleEvent for a container termination.
func (h *ContainerTerminationHandler) Handle(ev workloadmeta.Event) ([]LifecycleEvent, error) {
	container, ok := ev.Entity.(*workloadmeta.Container)
	if !ok {
		return nil, fmt.Errorf("expected *workloadmeta.Container, got %T", ev.Entity)
	}

	ctr := &contlcycle.ContainerEvent{
		ContainerID: container.ID,
		Source:      string(workloadmeta.SourceRuntime),
	}

	if !container.State.FinishedAt.IsZero() {
		ts := container.State.FinishedAt.Unix()
		ctr.OptionalExitTimestamp = &contlcycle.ContainerEvent_ExitTimestamp{ExitTimestamp: ts}
	}

	if container.State.ExitCode != nil {
		code := int32(*container.State.ExitCode)
		ctr.OptionalExitCode = &contlcycle.ContainerEvent_ExitCode{ExitCode: code}
	}

	// The container runtime has no owner knowledge; query the store.
	if c, err := h.store.GetContainer(container.ID); err == nil {
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
				log.Tracef("Cannot handle owner for container %q with type %q", container.ID, c.Owner.Kind)
			}
		}
	}

	return []LifecycleEvent{{
		ObjectKind: types.ObjectKindContainer,
		ProtoEvent: &contlcycle.Event{
			EventType:  contlcycle.Event_Delete,
			TypedEvent: &contlcycle.Event_Container{Container: ctr},
		},
	}}, nil
}
