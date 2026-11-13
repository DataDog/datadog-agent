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
)

// containerShadow is the last-observed state of one container slot in a pod.
type containerShadow struct {
	// id is the last-seen container ID of the slot; it is empty until the container starts.
	id string
	// state is the last-observed state value; it is nil before the first observation of a run.
	state *contlcycle.ContainerStateValue
}

// podContainersShadow is the last-observed state of a pod's container slots, keyed by slot.
type podContainersShadow struct {
	mu         sync.Mutex
	containers map[string]*containerShadow
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
// status of the pod, and clears the pod's shadow state once workloadmeta reports
// it as unset. Handlers are not mutually exclusive, so this runs alongside the
// pod state handler on the same pod events.
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

	for _, slot := range []struct {
		kind     contlcycle.ContainerKind
		statuses []workloadmeta.KubernetesContainerStatus
	}{
		{contlcycle.ContainerKind_CONTAINER_KIND_REGULAR, pod.ContainerStatuses},
		{contlcycle.ContainerKind_CONTAINER_KIND_INIT, pod.InitContainerStatuses},
		{contlcycle.ContainerKind_CONTAINER_KIND_EPHEMERAL, pod.EphemeralContainerStatuses},
	} {
		for _, status := range slot.statuses {
			newState, exactTime := containerStateToModel(status.State)
			if newState == nil {
				continue
			}

			key := slotKey(slot.kind, status.Name)
			cs, seen := shadow.containers[key]
			if !seen {
				cs = &containerShadow{}
				shadow.containers[key] = cs
			} else if cs.id != "" && cs.id != status.ContainerID {
				// A container ID that differs from the last-seen one marks a new
				// run (restart); forget the prior run so it is a first observation.
				cs.id = ""
				cs.state = nil
			}

			if cs.state != nil && proto.Equal(cs.state, newState) {
				cs.id = status.ContainerID
				continue
			}

			var lastObserved *contlcycle.ContainerStateValue
			if cs.state != nil {
				lastObserved = cs.state
			}

			transitionTimestamp, precision := transitionTime(exactTime)

			events = append(events, &contlcycle.ContainerEvent{
				ContainerID: status.ContainerID,
				Source:      string(workloadmeta.SourceNodeOrchestrator),
				Transition: &contlcycle.ContainerStateTransition{
					ContainerKind:       slot.kind,
					LastObservedState:   lastObserved,
					NewState:            newState,
					TransitionTimestamp: transitionTimestamp,
					Precision:           precision,
					MissedIntermediate:  containerMissedIntermediate(cs.state, newState),
				},
			})

			cs.id = status.ContainerID
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

// containerStateToModel converts a workloadmeta container state into its contlcycle
// value, along with the exact time the state was entered, if known.
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

// transitionTime returns the timestamp and precision for a transition into a
// state entered at exactTime; unknown times fall back to now, approximately.
func transitionTime(exactTime time.Time) (int64, contlcycle.Precision) {
	if exactTime.IsZero() {
		return time.Now().Unix(), contlcycle.Precision_PRECISION_APPROXIMATE
	}
	return exactTime.Unix(), contlcycle.Precision_PRECISION_EXACT
}

// containerMissedIntermediate reports PROVEN when a container skips from waiting
// to terminated within a single run, since states only advance waiting to running
// to terminated; all other transitions report unknowable.
func containerMissedIntermediate(prior, current *contlcycle.ContainerStateValue) contlcycle.MissedIntermediate {
	if prior != nil &&
		prior.GetKind() == contlcycle.ContainerStateKind_CONTAINER_STATE_KIND_WAITING &&
		current.GetKind() == contlcycle.ContainerStateKind_CONTAINER_STATE_KIND_TERMINATED {
		return contlcycle.MissedIntermediate_MISSED_INTERMEDIATE_PROVEN
	}
	return contlcycle.MissedIntermediate_MISSED_INTERMEDIATE_UNKNOWABLE
}

// slotKey identifies a container slot within a pod, by kind and name, so init,
// regular, and ephemeral containers never collide.
func slotKey(kind contlcycle.ContainerKind, name string) string {
	return fmt.Sprintf("%d/%s", kind, name)
}
