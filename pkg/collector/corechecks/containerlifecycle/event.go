// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"fmt"

	model "github.com/DataDog/agent-payload/v5/contlcycle"

	types "github.com/DataDog/datadog-agent/pkg/containerlifecycle"
)

type event interface {
	withObjectKind(string)
	withObjectID(string)
	withEventType(string)
	withSource(string)
	withContainerExitCode(*int32)
	withContainerExitTimestamp(*int64)
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

func (e *eventTransformer) toPayloadModel() (*model.EventsPayload, error) {
	payload := &model.EventsPayload{Version: types.PayloadV1}
	kind, err := e.kind()
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

		event.TypedEvent = &model.Event_Container{Container: container}
	case types.ObjectKindPod:
		pod := &model.PodEvent{
			PodUID: e.objectID,
			Source: e.source,
		}

		event.TypedEvent = &model.Event_Pod{Pod: pod}
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

func (e *eventTransformer) kind() (model.EventsPayload_ObjectKind, error) {
	switch e.objectKind {
	case types.ObjectKindContainer:
		return model.EventsPayload_Container, nil
	case types.ObjectKindPod:
		return model.EventsPayload_Pod, nil
	default:
		return -1, fmt.Errorf("unknown object kind %q", e.objectKind)
	}
}
