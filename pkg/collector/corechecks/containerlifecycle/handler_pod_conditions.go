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

// PodConditionsHandler handles pod conditions events.
// It works by maintaining a shadow copy from pods to current conditions used to detect changes.
type PodConditionsHandler struct {
	// shadow maps pod entity ID to a map of condition type to condition
	shadow map[workloadmeta.EntityID]map[string]workloadmeta.KubernetesPodCondition
}

// NewPodConditionsHandler returns an initialized PodConditionsHandler.
func NewPodConditionsHandler() *PodConditionsHandler {
	return &PodConditionsHandler{
		shadow: make(map[workloadmeta.EntityID]map[string]workloadmeta.KubernetesPodCondition),
	}
}

// String returns a human-readable name for the handler.
func (h *PodConditionsHandler) String() string {
	return "PodConditionsHandler"
}

// CanHandle reports whether this handler processes the given event.
func (h *PodConditionsHandler) CanHandle(ev workloadmeta.Event) bool {
	return ev.Type == workloadmeta.EventTypeSet &&
		ev.Entity.GetID().Kind == workloadmeta.KindKubernetesPod
}

// Handle builds a LifecycleEvent for a pod conditions event.
func (h *PodConditionsHandler) Handle(ev workloadmeta.Event) ([]LifecycleEvent, error) {
	pod, ok := ev.Entity.(*workloadmeta.KubernetesPod)
	if !ok {
		return nil, fmt.Errorf("expected *workloadmeta.KubernetesPod, got %T", ev.Entity)
	}

	podID := pod.GetID()
	if _, ok := h.shadow[podID]; !ok {
		h.shadow[podID] = make(map[string]workloadmeta.KubernetesPodCondition)
	}

	lifecycleEvents := make([]LifecycleEvent, 0)
	for _, condition := range pod.Conditions {
		conditionType := condition.Type
		if _, ok := h.shadow[podID][conditionType]; !ok {
			// First time we see this condition
			h.shadow[podID][conditionType] = condition

			// Still want to build a LifecycleEvent
			newTs := condition.LastTransitionTime.Unix()
			protoPodEvent := &contlcycle.PodEvent{
				PodUID: pod.GetID().ID,
				Source: string(workloadmeta.SourceNodeOrchestrator),
				ConditionUpdate: &contlcycle.PodConditionUpdate{
					NewCondition: &contlcycle.PodCondition{
						Type:               &condition.Type,
						Status:             &condition.Status,
						Reason:             &condition.Reason,
						LastTransitionTime: &newTs,
					},
				},
			}
			lifecycleEvent := LifecycleEvent{
				ObjectKind: types.ObjectKindPod,
				ProtoEvent: &contlcycle.Event{
					EventType:  contlcycle.Event_ConditionUpdate,
					TypedEvent: &contlcycle.Event_Pod{Pod: protoPodEvent},
				},
			}
			lifecycleEvents = append(lifecycleEvents, lifecycleEvent)

			// Update the shadow
			h.shadow[podID][conditionType] = condition
			continue
		}

		if h.shadow[podID][conditionType] != condition {
			// Condition changed in some way, build a LifecycleEvent
			priorCondition := h.shadow[podID][conditionType]

			priorTs := priorCondition.LastTransitionTime.Unix()
			newTs := condition.LastTransitionTime.Unix()
			protoPodEvent := &contlcycle.PodEvent{
				PodUID: pod.GetID().ID,
				Source: string(workloadmeta.SourceNodeOrchestrator),
				ConditionUpdate: &contlcycle.PodConditionUpdate{
					PriorCondition: &contlcycle.PodCondition{
						Type:               &priorCondition.Type,
						Status:             &priorCondition.Status,
						Reason:             &priorCondition.Reason,
						LastTransitionTime: &priorTs,
					},
					NewCondition: &contlcycle.PodCondition{
						Type:               &condition.Type,
						Status:             &condition.Status,
						Reason:             &condition.Reason,
						LastTransitionTime: &newTs,
					},
				},
			}

			lifecycleEvent := LifecycleEvent{
				ObjectKind: types.ObjectKindPod,
				ProtoEvent: &contlcycle.Event{
					EventType:  contlcycle.Event_ConditionUpdate,
					TypedEvent: &contlcycle.Event_Pod{Pod: protoPodEvent},
				},
			}
			lifecycleEvents = append(lifecycleEvents, lifecycleEvent)

			// Update the shadow
			h.shadow[podID][conditionType] = condition
		}

	}

	return lifecycleEvents, nil
}
