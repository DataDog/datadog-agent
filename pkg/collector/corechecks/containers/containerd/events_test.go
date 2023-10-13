// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/containerd/containerd"
	containerdevents "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/events"
	"github.com/containerd/typeurl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	containerdutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/containerd/fake"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

type mockEvt struct {
	events.Publisher
	events.Forwarder
	mockSubscribe func(ctx context.Context, filter ...string) (ch <-chan *events.Envelope, errs <-chan error)
}

func (m *mockEvt) Subscribe(ctx context.Context, filters ...string) (ch <-chan *events.Envelope, errs <-chan error) {
	return m.mockSubscribe(ctx)
}

type mockedContainer struct {
	containerd.Container
	mockID func() string
}

func (m *mockedContainer) ID() string {
	return m.mockID()
}

// TestCheckEvent is an integration test as the underlying logic that we test is the listener for events.
func TestCheckEvents(t *testing.T) {
	testNamespace := "test_namespace"
	cha := make(chan *events.Envelope)
	errorsCh := make(chan error)
	me := &mockEvt{
		mockSubscribe: func(ctx context.Context, filter ...string) (ch <-chan *events.Envelope, errs <-chan error) {
			return cha, errorsCh
		},
	}
	itf := &fake.MockedContainerdClient{
		MockEvents: func() containerd.EventService {
			return containerd.EventService(me)
		},
		MockNamespaces: func(ctx context.Context) ([]string, error) {
			return []string{testNamespace}, nil
		},
		MockContainers: func(namespace string) ([]containerd.Container, error) {
			return nil, nil
		},
	}
	// Test the basic listener
	sub := createEventSubscriber("subscriberTest1", containerdutil.ContainerdItf(itf), nil)
	sub.CheckEvents()

	tp := &containerdevents.TaskPaused{
		ContainerID: "42",
	}

	vp, err := typeurl.MarshalAny(tp)
	assert.NoError(t, err)

	en := events.Envelope{
		Timestamp: time.Now(),
		Topic:     "/tasks/paused",
		Event:     vp,
	}
	cha <- &en

	timeout := time.NewTimer(2 * time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	condition := false
	for {
		select {
		case <-ticker.C:
			if !sub.isRunning() {
				continue
			}
			condition = true
		case <-timeout.C:
			require.FailNow(t, "Timeout waiting event listener to be healthy")
		}
		if condition {
			break
		}
	}

	ev := sub.Flush(time.Now().Unix())
	assert.Len(t, ev, 1)
	assert.Equal(t, ev[0].Topic, "/tasks/paused")
	errorsCh <- fmt.Errorf("chan breaker")
	condition = false
	for {
		select {
		case <-ticker.C:
			if sub.isRunning() {
				continue
			}
			condition = true
		case <-timeout.C:
			require.FailNow(t, "Timeout waiting for error")
		}
		if condition {
			break
		}
	}

	// Test the multiple events one unsupported
	sub = createEventSubscriber("subscriberTest2", containerdutil.ContainerdItf(itf), nil)
	sub.CheckEvents()

	tk := &containerdevents.TaskOOM{
		ContainerID: "42",
	}
	vk, err := typeurl.MarshalAny(tk)
	assert.NoError(t, err)

	ek := events.Envelope{
		Timestamp: time.Now(),
		Topic:     "/tasks/oom",
		Event:     vk,
	}

	nd := &containerdevents.NamespaceDelete{
		Name: "k10s.io",
	}
	vnd, err := typeurl.MarshalAny(nd)
	assert.NoError(t, err)

	evnd := events.Envelope{
		Timestamp: time.Now(),
		Topic:     "/namespaces/delete",
		Event:     vnd,
	}

	cha <- &ek
	cha <- &evnd

	condition = false
	for {
		select {
		case <-ticker.C:
			if !sub.isRunning() {
				continue
			}
			condition = true
		case <-timeout.C:
			require.FailNow(t, "Timeout waiting event listener to be healthy")
		}
		if condition {
			break
		}
	}
	ev2 := sub.Flush(time.Now().Unix())
	fmt.Printf("\n\n 2/ Flush %v\n\n", ev2)
	assert.Len(t, ev2, 1)
	assert.Equal(t, ev2[0].Topic, "/tasks/oom")
}

func TestCheckEvents_PauseContainers(t *testing.T) {
	testNamespace := "test_namespace"
	existingPauseContainerID := "existing_pause"
	existingNonPauseContainerID := "existing_non_pause"
	newPauseContainerID := "new_container"

	testTimeout := 1 * time.Second
	testTicker := 5 * time.Millisecond

	cha := make(chan *events.Envelope)
	errorsCh := make(chan error)
	me := &mockEvt{
		mockSubscribe: func(ctx context.Context, filter ...string) (ch <-chan *events.Envelope, errs <-chan error) {
			return cha, errorsCh
		},
	}

	// Define a mocked containerd client. There are 2 containers deployed, one
	// is a pause one and the other is not.
	itf := &fake.MockedContainerdClient{
		MockEvents: func() containerd.EventService {
			return containerd.EventService(me)
		},
		MockNamespaces: func(ctx context.Context) ([]string, error) {
			return []string{testNamespace}, nil
		},
		MockContainers: func(namespace string) ([]containerd.Container, error) {
			if namespace == testNamespace {
				return []containerd.Container{
					&mockedContainer{
						mockID: func() string {
							return existingPauseContainerID
						},
					},
					&mockedContainer{
						mockID: func() string {
							return existingNonPauseContainerID
						},
					},
				}, nil
			}

			return nil, nil
		},
		MockContainer: func(namespace string, id string) (containerd.Container, error) {
			if namespace == testNamespace && id == newPauseContainerID {
				return &mockedContainer{
					mockID: func() string {
						return newPauseContainerID
					},
				}, nil
			}

			return nil, nil
		},
		MockIsSandbox: func(namespace string, ctn containerd.Container) (bool, error) {
			return namespace == testNamespace && (ctn.ID() == existingPauseContainerID || ctn.ID() == newPauseContainerID), nil
		},
	}

	sub := createEventSubscriber("subscriberTestPauseContainers", containerdutil.ContainerdItf(itf), nil)
	sub.CheckEvents()
	assert.Eventually(t, sub.isRunning, testTimeout, testTicker) // Wait until it's processing events

	tests := []struct {
		name                   string
		containerID            string
		excludePauseContainers bool
		expectsEvents          bool
		generateCreateEvent    bool
	}{
		{
			name:                   "existing pause container",
			containerID:            existingPauseContainerID,
			excludePauseContainers: true,
			expectsEvents:          false,
			generateCreateEvent:    false, // existing container
		},
		{
			name:                   "existing non-pause container",
			containerID:            existingNonPauseContainerID,
			excludePauseContainers: true,
			expectsEvents:          true,
			generateCreateEvent:    false, // existing container
		},
		{
			name:                   "new pause container",
			containerID:            newPauseContainerID,
			excludePauseContainers: true,
			expectsEvents:          false,
			generateCreateEvent:    true,
		},
		{
			name:                   "pause container, but pause containers are not excluded",
			containerID:            existingPauseContainerID,
			excludePauseContainers: false,
			expectsEvents:          true,
			generateCreateEvent:    false, // existing container
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defaultExcludePauseContainers := config.Datadog.GetBool("exclude_pause_container")
			config.Datadog.Set("exclude_pause_container", test.excludePauseContainers)

			if test.generateCreateEvent {
				eventCreateContainer, err := createContainerEvent(testNamespace, test.containerID)
				assert.NoError(t, err)
				cha <- &eventCreateContainer
			}

			eventDeleteTask, err := deleteTaskEvent(testNamespace, test.containerID)
			assert.NoError(t, err)
			cha <- &eventDeleteTask

			eventContainerDelete, err := deleteContainerEvent(testNamespace, test.containerID)
			assert.NoError(t, err)
			cha <- &eventContainerDelete

			if test.expectsEvents {
				var flushed []containerdEvent
				assert.Eventually(t, func() bool {
					flushed = sub.Flush(time.Now().Unix())
					if test.generateCreateEvent {
						return len(flushed) == 3 // create container + delete task + delete container
					}
					return len(flushed) == 2 // delete task + delete container
				}, testTimeout, testTicker)
			} else {
				assert.Empty(t, sub.Flush(time.Now().Unix()))
			}

			config.Datadog.Set("exclude_pause_container", defaultExcludePauseContainers)
		})
	}

	errorsCh <- fmt.Errorf("stop subscriber")
}

// TestComputeEvents checks the conversion of Containerd events to Datadog events
func TestComputeEvents(t *testing.T) {
	containerdCheck := &ContainerdCheck{
		instance:  &ContainerdConfig{},
		CheckBase: corechecks.NewCheckBase("containerd"),
	}
	mocked := mocksender.NewMockSender(containerdCheck.ID())
	var err error
	defer containers.ResetSharedFilter()
	containerdCheck.containerFilter, err = containers.GetSharedMetricFilter()
	require.NoError(t, err)

	tests := []struct {
		name          string
		events        []containerdEvent
		expectedTitle string
		expectedTags  []string
		numberEvents  int
	}{
		{
			name:          "No events",
			events:        []containerdEvent{},
			expectedTitle: "",
			numberEvents:  0,
		},
		{
			name: "Events on wrong type",
			events: []containerdEvent{
				{
					Topic: "/containers/delete/extra",
				}, {
					Topic: "containers/delete",
				},
			},
			expectedTitle: "",
			numberEvents:  0,
		},
		{
			name: "High cardinality Events with one invalid",
			events: []containerdEvent{
				{
					Topic:     "/containers/delete",
					Timestamp: time.Now(),
					Extra:     map[string]string{"foo": "bar"},
					Message:   "Container xxx deleted",
					ID:        "xxx",
				}, {
					Topic: "containers/delete",
				},
			},
			expectedTitle: "Event on containers from Containerd",
			expectedTags:  []string{"foo:bar", "event_type:destroy"},
			numberEvents:  1,
		},
		{
			name: "Low cardinality Event",
			events: []containerdEvent{
				{
					Topic:     "/images/update",
					Timestamp: time.Now(),
					Extra:     map[string]string{"foo": "baz"},
					Message:   "Image yyy updated",
					ID:        "yyy",
				},
			},
			expectedTitle: "Event on images from Containerd",
			expectedTags:  []string{"foo:baz"},
			numberEvents:  1,
		},
		{
			name: "Filtered event",
			events: []containerdEvent{
				{
					Topic:     "/images/create",
					Timestamp: time.Now(),
					Extra:     map[string]string{},
					Message:   "Image kubernetes/pause created",
					ID:        "kubernetes/pause",
				},
			},
			expectedTitle: "Event on images from Containerd",
			expectedTags:  nil,
			numberEvents:  0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			computeEvents(test.events, mocked, containerdCheck.containerFilter)
			mocked.On("Event", mock.AnythingOfType("event.Event"))
			if len(mocked.Calls) > 0 {
				res := (mocked.Calls[0].Arguments.Get(0)).(event.Event)
				assert.Contains(t, res.Title, test.expectedTitle)
				assert.ElementsMatch(t, res.Tags, test.expectedTags)
			}
			mocked.AssertNumberOfCalls(t, "Event", test.numberEvents)
			mocked.ResetCalls()
		})
	}
}

func createContainerEvent(namespace string, containerID string) (events.Envelope, error) {
	containerCreate := &containerdevents.ContainerCreate{
		ID: containerID,
	}
	containerCreateMarshal, err := typeurl.MarshalAny(containerCreate)
	if err != nil {
		return events.Envelope{}, err
	}

	return events.Envelope{
		Namespace: namespace,
		Timestamp: time.Now(),
		Topic:     "/containers/create",
		Event:     containerCreateMarshal,
	}, nil
}

func deleteTaskEvent(namespace string, containerID string) (events.Envelope, error) {
	taskDelete := &containerdevents.TaskDelete{
		ContainerID: containerID,
	}
	taskDeleteMarshal, err := typeurl.MarshalAny(taskDelete)
	if err != nil {
		return events.Envelope{}, err
	}

	return events.Envelope{
		Namespace: namespace,
		Timestamp: time.Now(),
		Topic:     "/tasks/delete",
		Event:     taskDeleteMarshal,
	}, nil
}

func deleteContainerEvent(namespace string, containerID string) (events.Envelope, error) {
	containerDelete := &containerdevents.ContainerDelete{
		ID: containerID,
	}
	containerDeleteMarshal, err := typeurl.MarshalAny(containerDelete)
	if err != nil {
		return events.Envelope{}, err
	}

	return events.Envelope{
		Namespace: namespace,
		Timestamp: time.Now(),
		Topic:     "/containers/delete",
		Event:     containerDeleteMarshal,
	}, nil
}
