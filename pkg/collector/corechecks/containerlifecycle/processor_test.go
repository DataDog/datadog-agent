// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	model "github.com/DataDog/agent-payload/v5/contlcycle"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
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

type fakeStore struct {
	workloadmeta.Component
}

func (f *fakeStore) GetContainer(id string) (*workloadmeta.Container, error) {
	switch id {
	case "cont1":
		return &workloadmeta.Container{
			Owner: &workloadmeta.EntityID{
				ID:   "pod1",
				Kind: workloadmeta.KindKubernetesPod,
			},
		}, nil
	case "cont2":
		return &workloadmeta.Container{
			Owner: &workloadmeta.EntityID{
				ID:   "task1",
				Kind: workloadmeta.KindECSTask,
			},
		}, nil
	default:
		return &workloadmeta.Container{}, nil
	}
}

func TestProcessContainer(t *testing.T) {
	p := &processor{
		containersQueue: &queue{},
		store:           &fakeStore{},
	}

	now := time.Now()
	exitCode := int32(1)
	podContainer := workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   "cont1",
			Kind: workloadmeta.KindContainer,
		},
		State: workloadmeta.ContainerState{
			FinishedAt: now,
			ExitCode:   &exitCode,
		},
	}
	taskContainer := podContainer
	taskContainer.ID = "cont2"
	hostName, _ := hostname.Get(context.TODO())

	err := p.processContainer(&podContainer, []workloadmeta.Source{workloadmeta.SourceRuntime})
	assert.NoError(t, err)

	err = p.processContainer(&taskContainer, []workloadmeta.Source{workloadmeta.SourceRuntime})
	assert.NoError(t, err)

	assert.Len(t, p.containersQueue.data, 2)

	assert.EqualValues(t, []*model.EventsPayload{
		{Version: "v1",
			Host: hostName,
			Events: []*model.Event{
				{
					EventType: model.Event_Delete,
					TypedEvent: &model.Event_Container{
						Container: &model.ContainerEvent{
							ContainerID: "cont1",
							Source:      "runtime",
							OptionalExitTimestamp: &model.ContainerEvent_ExitTimestamp{
								ExitTimestamp: now.Unix(),
							},
							OptionalExitCode: &model.ContainerEvent_ExitCode{
								ExitCode: 1,
							},
							Owner: &model.ContainerEvent_Owner{
								OwnerType: model.ObjectKind_Pod,
								OwnerUID:  "pod1",
							},
						},
					},
				},
			}},
		{Version: "v1",
			Host: hostName,
			Events: []*model.Event{
				{
					EventType: model.Event_Delete,
					TypedEvent: &model.Event_Container{
						Container: &model.ContainerEvent{
							ContainerID: "cont2",
							Source:      "runtime",
							OptionalExitTimestamp: &model.ContainerEvent_ExitTimestamp{
								ExitTimestamp: now.Unix(),
							},
							OptionalExitCode: &model.ContainerEvent_ExitCode{
								ExitCode: 1,
							},
							Owner: &model.ContainerEvent_Owner{
								OwnerType: model.ObjectKind_Task,
								OwnerUID:  "task1",
							},
						},
					},
				},
			}},
	}, p.containersQueue.data)
}
