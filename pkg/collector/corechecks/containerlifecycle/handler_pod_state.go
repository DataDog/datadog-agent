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

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	types "github.com/DataDog/datadog-agent/pkg/containerlifecycle"
)

// podShadow is the last-observed state of a pod's phase and conditions
type podShadow struct {
	phase      string
	conditions map[string]workloadmeta.KubernetesPodCondition
}

// PodStateHandler handles pod phase and condition transition events.
type PodStateHandler struct {
	mu sync.Mutex
	// shadow maps pod UID to its last-observed phase and conditions.
	shadow map[string]*podShadow
}

// NewPodStateHandler returns an initialized PodStateHandler.
func NewPodStateHandler() *PodStateHandler {
	return &PodStateHandler{shadow: make(map[string]*podShadow)}
}

// String returns a human-readable name for the handler.
func (h *PodStateHandler) String() string {
	return "PodStateHandler"
}

// CanHandle reports whether this handler processes the given event.
func (h *PodStateHandler) CanHandle(ev workloadmeta.Event) bool {
	return ev.Entity.GetID().Kind == workloadmeta.KindKubernetesPod
}

// Handle builds a LifecycleEvent per detected phase or condition transition for the pod,
// and clears the pod's shadow state once workloadmeta reports it as unset.
func (h *PodStateHandler) Handle(ev workloadmeta.Event) ([]LifecycleEvent, error) {
	podUID := ev.Entity.GetID().ID

	h.mu.Lock()
	defer h.mu.Unlock()

	if ev.Type == workloadmeta.EventTypeUnset {
		delete(h.shadow, podUID)
		return nil, nil
	}

	pod, ok := ev.Entity.(*workloadmeta.KubernetesPod)
	if !ok {
		return nil, fmt.Errorf("expected *workloadmeta.KubernetesPod, got %T", ev.Entity)
	}

	shadow, ok := h.shadow[podUID]
	if !ok {
		shadow = &podShadow{conditions: make(map[string]workloadmeta.KubernetesPodCondition)}
		h.shadow[podUID] = shadow
	}

	var transitions []*contlcycle.PodStateTransition

	if pod.Phase != "" && pod.Phase != shadow.phase {
		var lastObserved *contlcycle.PodStatusValue
		if shadow.phase != "" {
			lastObserved = &contlcycle.PodStatusValue{Value: &contlcycle.PodStatusValue_Phase{Phase: shadow.phase}}
		}

		transitions = append(transitions, &contlcycle.PodStateTransition{
			Field:               contlcycle.PodStatusField_POD_STATUS_FIELD_PHASE,
			LastObservedState:   lastObserved,
			NewState:            &contlcycle.PodStatusValue{Value: &contlcycle.PodStatusValue_Phase{Phase: pod.Phase}},
			TransitionTimestamp: time.Now().Unix(),
			Precision:           contlcycle.Precision_PRECISION_APPROXIMATE,
			MissedIntermediate:  contlcycle.MissedIntermediate_MISSED_INTERMEDIATE_UNKNOWABLE,
		})

		shadow.phase = pod.Phase
	}

	for _, condition := range pod.Conditions {
		priorCondition, seen := shadow.conditions[condition.Type]
		if seen && priorCondition == condition {
			continue
		}

		missedIntermediate := contlcycle.MissedIntermediate_MISSED_INTERMEDIATE_UNKNOWABLE
		if seen && priorCondition.Status == condition.Status &&
			!priorCondition.LastTransitionTime.Equal(condition.LastTransitionTime) {
			// LastTransitionTime only advances on a real status flip, so an unchanged
			// status with an advanced timestamp proves it transitioned away and back
			// in the gap since our last observation.
			missedIntermediate = contlcycle.MissedIntermediate_MISSED_INTERMEDIATE_PROVEN
		}

		var lastObserved *contlcycle.PodStatusValue
		if seen {
			lastObserved = &contlcycle.PodStatusValue{Value: &contlcycle.PodStatusValue_Condition{
				Condition: conditionToModel(priorCondition),
			}}
		}

		transitions = append(transitions, &contlcycle.PodStateTransition{
			Field:             contlcycle.PodStatusField_POD_STATUS_FIELD_CONDITION,
			LastObservedState: lastObserved,
			NewState: &contlcycle.PodStatusValue{Value: &contlcycle.PodStatusValue_Condition{
				Condition: conditionToModel(condition),
			}},
			TransitionTimestamp: condition.LastTransitionTime.Unix(),
			Precision:           contlcycle.Precision_PRECISION_EXACT,
			MissedIntermediate:  missedIntermediate,
		})

		shadow.conditions[condition.Type] = condition
	}

	if len(transitions) == 0 {
		return nil, nil
	}

	les := make([]LifecycleEvent, 0, len(transitions))
	for _, transition := range transitions {
		les = append(les, LifecycleEvent{
			ObjectKind: types.ObjectKindPod,
			ProtoEvent: &contlcycle.Event{
				EventType: contlcycle.Event_Transition,
				TypedEvent: &contlcycle.Event_Pod{Pod: &contlcycle.PodEvent{
					PodUID:     podUID,
					Source:     string(workloadmeta.SourceNodeOrchestrator),
					Transition: transition,
				}},
			},
		})
	}

	return les, nil
}

// conditionToModel converts a workloadmeta pod condition into its contlcycle proto representation.
func conditionToModel(c workloadmeta.KubernetesPodCondition) *contlcycle.ConditionValue {
	return &contlcycle.ConditionValue{
		Type:   c.Type,
		Status: c.Status,
		Reason: &c.Reason,
	}
}
