// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"context"
	"fmt"

	model "github.com/DataDog/agent-payload/v5/contlcycle"

	types "github.com/DataDog/datadog-agent/pkg/containerlifecycle"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type event interface {
	withObjectKind(string)
	withObjectID(string)
	withEventType(string)
	withSource(string)
	withContainerExitCode(*int32)
	withContainerExitTimestamp(*int64)
	withPodExitTimestamp(*int64)
	withTaskExitTimestamp(*int64)
	withOwnerType(string)
	withOwnerID(string)
	toPayloadModel() (*model.EventsPayload, error)
	toEventModel() (*model.Event, error)
}

type eventTransformer struct {
	objectKind   string
	objectID     string
	eventType    string
	source       string
	contExitCode *int32
	contExitTS   *int64
	podExitTS    *int64
	taskExitTS   *int64
	ownerType    string
	ownerID      string
}

func newEvent() event {
	return &eventTransformer{}
}

func (e *eventTransformer) withObjectKind(kind string) {
	e.objectKind = kind
}

func (e *eventTransformer) withObjectID(id string) {
	e.objectID = id
}

func (e *eventTransformer) withEventType(typ string) {
	e.eventType = typ
}

func (e *eventTransformer) withSource(source string) {
	e.source = source
}

func (e *eventTransformer) withContainerExitCode(exitCode *int32) {
	e.contExitCode = exitCode
}

func (e *eventTransformer) withContainerExitTimestamp(exitTS *int64) {
	e.contExitTS = exitTS
}

func (e *eventTransformer) withPodExitTimestamp(exitTS *int64) {
	e.podExitTS = exitTS
}

func (e *eventTransformer) withTaskExitTimestamp(exitTS *int64) {
	e.taskExitTS = exitTS
}

func (e *eventTransformer) withOwnerType(t string) {
	e.ownerType = t
}

func (e *eventTransformer) withOwnerID(id string) {
	e.ownerID = id
}

func (e *eventTransformer) toPayloadModel() (*model.EventsPayload, error) {
	hname, err := hostname.Get(context.TODO())
	if err != nil {
		log.Warnf("Error getting hostname: %v", err)
	}

	payload := &model.EventsPayload{
		Version: types.PayloadV1,
		Host:    hname,
	}
	kind, err := e.kind(e.objectKind)
	if err != nil {
		return nil, err
	}

	payload.ObjectKind = kind

	event, err := e.toEventModel()
	if err != nil {
		return nil, err
	}

	payload.Events = []*model.Event{event}

	return payload, nil
}

func (e *eventTransformer) toEventModel() (*model.Event, error) {
	event := &model.Event{}

	evType, err := e.evType()
	if err != nil {
		return nil, err
	}

	event.EventType = evType

	switch e.objectKind {
	case types.ObjectKindContainer:
		container := &model.ContainerEvent{
			ContainerID: e.objectID,
			Source:      e.source,
		}

		if e.contExitCode != nil {
			container.OptionalExitCode = &model.ContainerEvent_ExitCode{ExitCode: *e.contExitCode}
		}

		if e.contExitTS != nil {
			container.OptionalExitTimestamp = &model.ContainerEvent_ExitTimestamp{ExitTimestamp: *e.contExitTS}
		}

		if e.ownerID != "" && e.ownerType != "" {
			if ownerType, err := e.kind(e.ownerType); err == nil {
				container.Owner = &model.ContainerEvent_Owner{
					OwnerType: ownerType,
					OwnerUID:  e.ownerID,
				}
			}
		}

		event.TypedEvent = &model.Event_Container{Container: container}
	case types.ObjectKindPod:
		pod := &model.PodEvent{
			PodUID: e.objectID,
			Source: e.source,
		}

		if e.podExitTS != nil {
			pod.ExitTimestamp = e.podExitTS
		}

		event.TypedEvent = &model.Event_Pod{Pod: pod}
	case types.ObjectKindTask:
		task := &model.TaskEvent{
			TaskARN: e.objectID,
			Source:  e.source,
		}

		if e.taskExitTS != nil {
			task.ExitTimestamp = e.taskExitTS
		}

		event.TypedEvent = &model.Event_Task{Task: task}
	default:
		return nil, fmt.Errorf("unknown kind %q", e.objectKind)
	}

	return event, nil
}

func (e *eventTransformer) evType() (model.Event_EventType, error) {
	switch e.eventType {
	case types.EventNameDelete:
		return model.Event_Delete, nil
	default:
		return -1, fmt.Errorf("unknown event type %s", e.eventType)
	}
}

func (e *eventTransformer) kind(kind string) (model.ObjectKind, error) {
	switch kind {
	case types.ObjectKindContainer:
		return model.ObjectKind_Container, nil
	case types.ObjectKindPod:
		return model.ObjectKind_Pod, nil
	case types.ObjectKindTask:
		return model.ObjectKind_Task, nil
	default:
		return -1, fmt.Errorf("unknown object kind %q", e.objectKind)
	}
}
