// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/agent-payload/v5/contlcycle"
	"google.golang.org/protobuf/proto"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	types "github.com/DataDog/datadog-agent/pkg/containerlifecycle"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// containerShadow is the last-observed state of one container slot in a pod.
type containerShadow struct {
	// id is the last-seen container ID of the slot; empty until the container starts.
	id string
	// state is nil until a run of the slot has been observed.
	state *contlcycle.ContainerStateValue
}

// podContainersShadow is the last-observed state of a pod's container slots.
type podContainersShadow struct {
	mu         sync.Mutex
	containers map[string]*containerShadow
}

// containerSlot pairs one of a pod's status lists with the container kind it describes.
type containerSlot struct {
	kind     contlcycle.ContainerKind
	statuses []workloadmeta.KubernetesContainerStatus
}

// podContainerSlots returns the regular, init, and ephemeral status slots of the pod.
func podContainerSlots(pod *workloadmeta.KubernetesPod) []containerSlot {
	return []containerSlot{
		{contlcycle.ContainerKind_CONTAINER_KIND_REGULAR, pod.ContainerStatuses},
		{contlcycle.ContainerKind_CONTAINER_KIND_INIT, pod.InitContainerStatuses},
		{contlcycle.ContainerKind_CONTAINER_KIND_EPHEMERAL, pod.EphemeralContainerStatuses},
	}
}

// ContainerStateHandler handles container state transition events.
type ContainerStateHandler struct {
	mu sync.Mutex
	// shadow maps pod UID to the last-observed state of its container slots.
	shadow map[string]*podContainersShadow
}

// NewContainerStateHandler returns an initialized ContainerStateHandler.
func NewContainerStateHandler() *ContainerStateHandler {
	return &ContainerStateHandler{shadow: make(map[string]*podContainersShadow)}
}

// String returns a human-readable name for the handler.
func (h *ContainerStateHandler) String() string {
	return "ContainerStateHandler"
}

// CanHandle reports whether this handler processes the given event.
func (h *ContainerStateHandler) CanHandle(ev workloadmeta.Event, _ workloadmeta.Source) bool {
	return ev.Entity.GetID().Kind == workloadmeta.KindKubernetesPod
}

// Handle builds a LifecycleEvent per detected state transition for each container
// slot of the pod, and clears the pod's shadow state once workloadmeta reports it
// as unset.
func (h *ContainerStateHandler) Handle(ev workloadmeta.Event) ([]LifecycleEvent, error) {
	podUID := ev.Entity.GetID().ID

	if ev.Type == workloadmeta.EventTypeUnset {
		h.mu.Lock()
		delete(h.shadow, podUID)
		h.mu.Unlock()
		return nil, nil
	}

	pod, ok := ev.Entity.(*workloadmeta.KubernetesPod)
	if !ok {
		return nil, fmt.Errorf("expected *workloadmeta.KubernetesPod, got %T", ev.Entity)
	}

	h.mu.Lock()
	shadow, ok := h.shadow[podUID]
	if !ok {
		shadow = &podContainersShadow{containers: make(map[string]*containerShadow)}
		h.shadow[podUID] = shadow
	}
	h.mu.Unlock()

	shadow.mu.Lock()
	defer shadow.mu.Unlock()

	var events []*contlcycle.ContainerEvent

	for _, slot := range podContainerSlots(pod) {
		for _, status := range slot.statuses {
			newState, exactTime := containerStateToModel(status.State)
			if newState == nil {
				continue
			}

			containerID := normalizeContainerID(status.ContainerID)

			key := slotKey(slot.kind, status.Name)
			cs, seen := shadow.containers[key]
			if !seen {
				cs = &containerShadow{}
				shadow.containers[key] = cs
			} else if cs.id != "" && cs.id != containerID {
				// A different container ID marks a new run (restart); forget the prior run.
				cs.id = ""
				cs.state = nil
			}

			if cs.state != nil && proto.Equal(cs.state, newState) {
				cs.id = containerID
				continue
			}

			var lastObserved *contlcycle.ContainerStateValue
			if cs.state != nil {
				lastObserved = cs.state
			}

			transitionTimestamp, precision := transitionTime(exactTime)
			containerName := status.Name

			events = append(events, &contlcycle.ContainerEvent{
				ContainerID:   containerID,
				Source:        string(workloadmeta.SourceNodeOrchestrator),
				ContainerName: &containerName,
				Owner: &contlcycle.ContainerEvent_Owner{
					OwnerType: contlcycle.ObjectKind_Pod,
					OwnerUID:  podUID,
				},
				Transition: &contlcycle.ContainerStateTransition{
					ContainerKind:       slot.kind,
					LastObservedState:   lastObserved,
					NewState:            newState,
					TransitionTimestamp: transitionTimestamp,
					Precision:           precision,
					// The kubelet polls the CRI, so a skipped state cannot prove a missed
					// intermediate: a run that went waiting-running-terminated between polls
					// looks identical to a start failure that never ran.
					MissedIntermediate: contlcycle.MissedIntermediate_MISSED_INTERMEDIATE_UNKNOWABLE,
				},
			})

			cs.id = containerID
			cs.state = newState
		}
	}

	if len(events) == 0 {
		return nil, nil
	}

	les := make([]LifecycleEvent, 0, len(events))
	for _, event := range events {
		les = append(les, LifecycleEvent{
			ObjectKind: types.ObjectKindContainer,
			ProtoEvent: &contlcycle.Event{
				EventType:  contlcycle.Event_Transition,
				TypedEvent: &contlcycle.Event_Container{Container: event},
			},
		})
	}

	return les, nil
}

// containerStateToModel converts a workloadmeta container state into its contlcycle value, with the exact time the state was entered, if known.
func containerStateToModel(s workloadmeta.KubernetesContainerState) (*contlcycle.ContainerStateValue, time.Time) {
	switch {
	case s.Running != nil:
		return &contlcycle.ContainerStateValue{Kind: contlcycle.ContainerStateKind_CONTAINER_STATE_KIND_RUNNING}, s.Running.StartedAt
	case s.Waiting != nil:
		return &contlcycle.ContainerStateValue{Kind: contlcycle.ContainerStateKind_CONTAINER_STATE_KIND_WAITING, Reason: &s.Waiting.Reason}, time.Time{}
	case s.Terminated != nil:
		return &contlcycle.ContainerStateValue{
			Kind:     contlcycle.ContainerStateKind_CONTAINER_STATE_KIND_TERMINATED,
			Reason:   &s.Terminated.Reason,
			ExitCode: &s.Terminated.ExitCode,
		}, s.Terminated.FinishedAt
	default:
		return nil, time.Time{}
	}
}

// transitionTime returns the timestamp and precision for a state entered at exactTime; unknown times fall back to now.
func transitionTime(exactTime time.Time) (int64, contlcycle.Precision) {
	if exactTime.IsZero() {
		return time.Now().Unix(), contlcycle.Precision_PRECISION_APPROXIMATE
	}
	return exactTime.Unix(), contlcycle.Precision_PRECISION_EXACT
}

// slotKey identifies a container slot within a pod by kind and name, so init, regular, and ephemeral containers never collide.
func slotKey(kind contlcycle.ContainerKind, name string) string {
	return fmt.Sprintf("%d/%s", kind, name)
}

// normalizeContainerID strips the runtime prefix (e.g. "containerd://") from a Kubernetes container ID.
func normalizeContainerID(id string) string {
	if _, cid := containers.SplitEntityName(id); cid != "" {
		return cid
	}
	return id
}
