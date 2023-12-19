// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"context"
	"strconv"
	"testing"

	model "github.com/DataDog/agent-payload/v5/contlcycle"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/stretchr/testify/assert"
)

func fakeContainerEvent(objID string) event {
	event := newEvent()
	event.withObjectID(objID)
	event.withObjectKind("container")
	event.withEventType("delete")

	return event
}

func modelEvents(objIDs ...string) []*model.Event {
	events := []*model.Event{}
	for _, id := range objIDs {
		events = append(events, &model.Event{TypedEvent: &model.Event_Container{Container: &model.ContainerEvent{ContainerID: id}}})
	}

	return events
}

func TestSingleQueueAdd(t *testing.T) {
	hname, _ := hostname.Get(context.TODO())
	commonChunkSize := 2

	tests := []struct {
		name string
		data []*model.EventsPayload
		ev   event
		want []*model.EventsPayload
	}{
		{
			name: "empty queue",
			data: []*model.EventsPayload{},
			ev:   fakeContainerEvent("obj1"),
			want: []*model.EventsPayload{{Version: "v1", Host: hname, Events: modelEvents("obj1")}},
		},
		{
			name: "last payload not full",
			data: []*model.EventsPayload{{Version: "v1", Host: hname, Events: modelEvents("obj1")}},
			ev:   fakeContainerEvent("obj2"),
			want: []*model.EventsPayload{{Version: "v1", Host: hname, Events: modelEvents("obj1", "obj2")}},
		},
		{
			name: "last payload full",
			data: []*model.EventsPayload{{Version: "v1", Host: hname, Events: modelEvents("obj1", "obj2")}},
			ev:   fakeContainerEvent("obj3"),
			want: []*model.EventsPayload{
				{Version: "v1", Host: hname, Events: modelEvents("obj1", "obj2")},
				{Version: "v1", Host: hname, Events: modelEvents("obj3")},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queue := newQueue(commonChunkSize)
			queue.data = tt.data

			err := queue.add(tt.ev)

			assert.Nil(t, err)
			assert.EqualValues(t, tt.want, queue.data)
		})
	}
}

func TestBatching(t *testing.T) {
	commonChunkSize := 2
	queue := newQueue(commonChunkSize)

	for i := int64(0); i < 10; i++ {
		queue.add(fakeContainerEvent(strconv.FormatInt(i, 10)))
	}

	data := queue.flush()
	assert.Len(t, data, 5)
	assert.Len(t, queue.data, 0)

	for i := int64(0); i < 5; i++ {
		assert.Len(t, data[i].Events, commonChunkSize)
		assert.EqualValues(t, data[i].Events, modelEvents(strconv.FormatInt(2*i, 10), strconv.FormatInt(2*i+1, 10)))
	}
}
