// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"fmt"

	model "github.com/DataDog/agent-payload/v5/contlcycle"
	types "github.com/DataDog/datadog-agent/pkg/containerlifecycle"
	"github.com/DataDog/datadog-agent/pkg/security/log"
)

type event interface {
	withObjectKind(string)
	withObjectID(string)
	withEventType(string)
	withSource(string)
	withExitCode(*int32)
	withExitTimestamp(*int64)
	toPayloadModel() model.EventsPayload
	toEventModel() *model.Event
}

type eventTransformer struct {
	tpl        model.EventsPayload
	objectKind string
	objectID   string
	eventType  string
	source     string
	exitCode   *int32
	exitTS     *int64
}

func newEvent() event {
	return &eventTransformer{
		tpl: model.EventsPayload{Version: types.PayloadV1},
	}
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

func (e *eventTransformer) withExitCode(exitCode *int32) {
	e.exitCode = exitCode
}

func (e *eventTransformer) withExitTimestamp(exitTS *int64) {
	e.exitTS = exitTS
}

func (e *eventTransformer) toPayloadModel() model.EventsPayload {
	payload := e.tpl
	kind, err := e.kind()
	if err != nil {
		log.Debugf("Error getting object kind: %w", err)
	} else {
		payload.ObjectKind = kind
	}

	payload.Events = []*model.Event{e.toEventModel()}

	return payload
}

func (e *eventTransformer) toEventModel() *model.Event {
	event := &model.Event{
		ObjectID: e.objectID,
		Source:   e.source,
	}

	typ, err := e.typ()
	if err != nil {
		log.Debugf("Error getting event type: %w", err)
	} else {
		event.EventType = typ
	}

	if e.exitCode != nil {
		event.OptionalExitCode = &model.Event_ExitCode{ExitCode: *e.exitCode}
	}

	if e.exitTS != nil {
		event.OptionalExitTimestamp = &model.Event_ExitTimestamp{ExitTimestamp: *e.exitTS}
	}

	return event
}

func (e *eventTransformer) typ() (model.Event_EventType, error) {
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
		return model.EventsPayload_Cont, nil
	case types.ObjectKindPod:
		return model.EventsPayload_Pod, nil
	default:
		return -1, fmt.Errorf("unknown object kind %s", e.objectKind)
	}
}
