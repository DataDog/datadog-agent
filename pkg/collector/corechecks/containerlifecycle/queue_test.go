// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"strconv"
	"testing"

	model "github.com/DataDog/agent-payload/v5/contlcycle"
	"github.com/stretchr/testify/assert"

	types "github.com/DataDog/datadog-agent/pkg/containerlifecycle"
)

func TestSingleQueueAdd(t *testing.T) {
	commonChunkSize := 2
	commonTemplate := model.EventsPaylaod{Version: types.PayloadV1}

	tests := []struct {
		name string
		data []model.EventsPaylaod
		ev   *event
		want []model.EventsPaylaod
	}{
		{
			name: "empty queue",
			data: []model.EventsPaylaod{},
			ev:   &event{objectID: "obj1"},
			want: []model.EventsPaylaod{{Version: "v1", Events: []*model.Event{{ObjectID: "obj1"}}}},
		},
		{
			name: "last payload not full",
			data: []model.EventsPaylaod{{Version: "v1", Events: []*model.Event{{ObjectID: "obj1"}}}},
			ev:   &event{objectID: "obj2"},
			want: []model.EventsPaylaod{{Version: "v1", Events: []*model.Event{{ObjectID: "obj1"}, {ObjectID: "obj2"}}}},
		},
		{
			name: "last payload full",
			data: []model.EventsPaylaod{{Version: "v1", Events: []*model.Event{{ObjectID: "obj1"}, {ObjectID: "obj2"}}}},
			ev:   &event{objectID: "obj3"},
			want: []model.EventsPaylaod{
				{Version: "v1", Events: []*model.Event{{ObjectID: "obj1"}, {ObjectID: "obj2"}}},
				{Version: "v1", Events: []*model.Event{{ObjectID: "obj3"}}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queue := newQueue(commonChunkSize, commonTemplate)
			queue.data = tt.data

			queue.add(tt.ev)
			assert.EqualValues(t, tt.want, queue.data)
		})
	}
}

func TestBatching(t *testing.T) {
	commonChunkSize := 2
	commonTemplate := model.EventsPaylaod{Version: types.PayloadV1}
	queue := newQueue(commonChunkSize, commonTemplate)

	for i := int64(0); i < 10; i++ {
		queue.add(&event{objectID: strconv.FormatInt(i, 10)})
	}

	data := queue.dump()
	assert.Len(t, data, 5)

	for i := int64(0); i < 5; i++ {
		assert.Len(t, data[i].Events, commonChunkSize)
		assert.EqualValues(t, data[i].Events[0], &model.Event{ObjectID: strconv.FormatInt(2*i, 10)})
		assert.EqualValues(t, data[i].Events[1], &model.Event{ObjectID: strconv.FormatInt(2*i+1, 10)})
	}
}
