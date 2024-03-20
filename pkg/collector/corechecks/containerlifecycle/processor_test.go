// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"context"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/contlcycle"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"

	"github.com/stretchr/testify/mock"
)

func TestProcessQueues(t *testing.T) {
	hname, _ := hostname.Get(context.TODO())

	tests := []struct {
		name            string
		containersQueue *queue
		podsQueue       *queue
		tasksQueue      *queue
		wantFunc        func(t *testing.T, s *mocksender.MockSender)
	}{
		{
			name:            "empty queues",
			containersQueue: &queue{},
			podsQueue:       &queue{},
			tasksQueue:      &queue{},
			wantFunc:        func(t *testing.T, s *mocksender.MockSender) { s.AssertNotCalled(t, "ContainerLifecycleEvent") },
		},
		{
			name: "one container",
			containersQueue: &queue{data: []*model.EventsPayload{
				{Version: "v1", Host: hname, Events: modelEvents("cont1")},
			}},
			podsQueue:  &queue{},
			tasksQueue: &queue{},
			wantFunc: func(t *testing.T, s *mocksender.MockSender) {
				s.AssertNumberOfCalls(t, "EventPlatformEvent", 1)
			},
		},
		{
			name: "multiple chunks per types",
			containersQueue: &queue{data: []*model.EventsPayload{
				{Version: "v1", Host: hname, Events: modelEvents("cont1", "cont2")},
				{Version: "v1", Host: hname, Events: modelEvents("cont3")},
			}},
			podsQueue: &queue{data: []*model.EventsPayload{
				{Version: "v1", Host: hname, Events: modelEvents("pod1", "pod2")},
				{Version: "v1", Host: hname, Events: modelEvents("pod3")},
			}},
			tasksQueue: &queue{data: []*model.EventsPayload{
				{Version: "v1", Host: hname, Events: modelEvents("task1", "task2")},
				{Version: "v1", Host: hname, Events: modelEvents("task3")},
			}},
			wantFunc: func(t *testing.T, s *mocksender.MockSender) {
				s.AssertNumberOfCalls(t, "EventPlatformEvent", 6)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &processor{
				containersQueue: tt.containersQueue,
				podsQueue:       tt.podsQueue,
				tasksQueue:      tt.tasksQueue,
			}

			sender := mocksender.NewMockSender(checkid.ID(tt.name))
			sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
			p.sender = sender

			ctx, cancel := context.WithCancel(context.Background())
			cancel() // To force the flush in p.processQueues

			p.processQueues(ctx, 500*time.Millisecond)

			tt.wantFunc(t, sender)
		})
	}
}
