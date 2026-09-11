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
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
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
				tagger:          taggerfxmock.SetupFakeTagger(t),
			}

			sender := mocksender.NewMockSender(t, checkid.ID(tt.name))
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
	now := time.Now()
	exitCode := int64(1)
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

	p := &processor{
		containersQueue: &queue{},
		handlers:        []Handler{NewContainerTerminationHandler(&fakeStore{})},
		tagger:          taggerfxmock.SetupFakeTagger(t),
	}

	p.processEvents(workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{Type: workloadmeta.EventTypeUnset, Entity: &podContainer},
			{Type: workloadmeta.EventTypeUnset, Entity: &taskContainer},
		},
	}, workloadmeta.SourceRuntime)

	hostName, _ := hostname.Get(context.TODO())

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

// TestFlushTags tests tag enrichment at flush time.
// Test partitions:
// - entity kind: container | pod | task
// - tagger state: known entity | unknown entity
// - entity ID: set | empty
func TestFlushTags(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.ContainerID, "cont1"), "kubelet", []string{"kube_namespace:default"}, nil, []string{"kube_deployment:ben"}, nil)
	fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.KubernetesPodUID, "pod1"), "kubelet", []string{"kube_namespace:default"}, nil, nil, nil)

	hostName, _ := hostname.Get(context.TODO())

	tests := []struct {
		name         string
		kind         string
		events       []*model.Event
		expectedTags [][]string
	}{
		{
			name: "container with tags in tagger",
			kind: "container",
			events: []*model.Event{{
				EventType:  model.Event_Delete,
				TypedEvent: &model.Event_Container{Container: &model.ContainerEvent{ContainerID: "cont1"}},
			}},
			expectedTags: [][]string{{"kube_deployment:ben", "kube_namespace:default"}},
		},
		{
			name: "pod with tags in tagger",
			kind: "pod",
			events: []*model.Event{{
				EventType:  model.Event_Delete,
				TypedEvent: &model.Event_Pod{Pod: &model.PodEvent{PodUID: "pod1"}},
			}},
			expectedTags: [][]string{{"kube_namespace:default"}},
		},
		{
			name: "container unknown to tagger",
			kind: "container",
			events: []*model.Event{{
				EventType:  model.Event_Delete,
				TypedEvent: &model.Event_Container{Container: &model.ContainerEvent{ContainerID: "louis-reasoner"}},
			}},
			expectedTags: [][]string{nil},
		},
		{
			name: "container with empty ID",
			kind: "container",
			events: []*model.Event{{
				EventType:  model.Event_Delete,
				TypedEvent: &model.Event_Container{Container: &model.ContainerEvent{}},
			}},
			expectedTags: [][]string{nil},
		},
		{
			name: "task events are not enriched",
			kind: "task",
			events: []*model.Event{{
				EventType:  model.Event_Delete,
				TypedEvent: &model.Event_Task{Task: &model.TaskEvent{TaskARN: "arn:aws:ecs:us-east-1:1:task/ben"}},
			}},
			expectedTags: [][]string{nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := mocksender.NewMockSender(t, checkid.ID(tt.name))
			var sentPayloads []*model.EventsPayload
			sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return().Run(func(args mock.Arguments) {
				raw := args.Get(0).([]byte)
				payload := &model.EventsPayload{}
				err := proto.Unmarshal(raw, payload)
				require.NoError(t, err)
				sentPayloads = append(sentPayloads, payload)
			})

			p := &processor{
				containersQueue: &queue{},
				podsQueue:       &queue{},
				tasksQueue:      &queue{},
				tagger:          fakeTagger,
				sender:          sender,
			}

			payload := &model.EventsPayload{Version: "v1", Host: hostName, Events: tt.events}
			switch tt.kind {
			case "container":
				p.containersQueue.data = []*model.EventsPayload{payload}
				p.flushContainers()
			case "pod":
				p.podsQueue.data = []*model.EventsPayload{payload}
				p.flushPods()
			case "task":
				p.tasksQueue.data = []*model.EventsPayload{payload}
				p.flushTasks()
			}

			require.Len(t, sentPayloads, 1)
			require.Len(t, sentPayloads[0].Events, len(tt.expectedTags))
			for i, expected := range tt.expectedTags {
				var actual []string
				switch typed := sentPayloads[0].Events[i].TypedEvent.(type) {
				case *model.Event_Container:
					actual = typed.Container.GetDdTags()
				case *model.Event_Pod:
					actual = typed.Pod.GetDdTags()
				case *model.Event_Task:
					// TaskEvent has no dd_tags field; nothing to assert.
				}
				assert.ElementsMatch(t, expected, actual)
			}
		})
	}
}
