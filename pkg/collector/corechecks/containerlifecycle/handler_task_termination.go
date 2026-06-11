// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"fmt"
	"time"

	"github.com/DataDog/agent-payload/v5/contlcycle"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	types "github.com/DataDog/datadog-agent/pkg/containerlifecycle"
)

// TaskTerminationHandler handles workloadmeta unset-task events.
type TaskTerminationHandler struct{}

// String returns a human-readable name for the handler.
func (h *TaskTerminationHandler) String() string {
	return "TaskTerminationHandler"
}

// CanHandle reports whether this handler processes the given event.
func (h *TaskTerminationHandler) CanHandle(ev workloadmeta.Event) bool {
	return ev.Type == workloadmeta.EventTypeUnset &&
		ev.Entity.GetID().Kind == workloadmeta.KindECSTask
}

// Handle builds a LifecycleEvent for a task termination.
func (h *TaskTerminationHandler) Handle(ev workloadmeta.Event) ([]LifecycleEvent, error) {
	task, ok := ev.Entity.(*workloadmeta.ECSTask)
	if !ok {
		return nil, fmt.Errorf("expected *workloadmeta.ECSTask, got %T", ev.Entity)
	}

	source := string(workloadmeta.SourceNodeOrchestrator)
	if task.LaunchType == workloadmeta.ECSLaunchTypeFargate {
		source = string(workloadmeta.SourceRuntime)
	}

	// Tasks from the metadata v1 API carry no exit timestamp; use the current time.
	ts := time.Now().Unix()
	taskEvent := &contlcycle.TaskEvent{
		TaskARN:       task.GetID().ID,
		Source:        source,
		ExitTimestamp: &ts,
	}

	return []LifecycleEvent{{
		ObjectKind: types.ObjectKindTask,
		ProtoEvent: &contlcycle.Event{
			EventType:  contlcycle.Event_Delete,
			TypedEvent: &contlcycle.Event_Task{Task: taskEvent},
		},
	}}, nil
}
