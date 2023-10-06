// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

const (
	dummySubscriber = "subscriber"
	fooSource       = "foo"
	barSource       = "bar"
)

func TestHandleEvents(t *testing.T) {
	s := newTestStore()

	container := &Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "deadbeef",
		},
	}

	s.handleEvents([]CollectorEvent{
		{
			Type:   EventTypeSet,
			Source: fooSource,
			Entity: container,
		},
	})

	gotContainer, err := s.GetContainer(container.ID)
	if err != nil {
		t.Errorf("expected to find container %q, not found", container.ID)
	}

	if !reflect.DeepEqual(container, gotContainer) {
		t.Errorf("expected container %q to match the one in the store", container.ID)
	}

	s.handleEvents([]CollectorEvent{
		{
			Type:   EventTypeUnset,
			Source: fooSource,
			Entity: container,
		},
	})

	_, err = s.GetContainer(container.ID)
	if err == nil || !errors.IsNotFound(err) {
		t.Errorf("expected container %q to be absent. found or had errors. err: %q", container.ID, err)
	}
}

func TestSubscribe(t *testing.T) {
	fooContainer := &Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "foo",
		},
		EntityMeta: EntityMeta{
			Name: "foo-name-might-be-overridden",
		},
		Hostname: "foo",
	}

	fooContainerToMerge := &Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "foo",
		},
		EntityMeta: EntityMeta{
			Name: "foo-name-override",
		},
		PID: 1001001,
	}

	fooContainerMerged := &Container{
		EntityID: fooContainer.EntityID,
		EntityMeta: EntityMeta{
			Name: fooContainerToMerge.Name,
		},
		Hostname: fooContainer.Hostname,
		PID:      fooContainerToMerge.PID,
	}

	barContainer := &Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "bar",
		},
	}

	bazContainer := &Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "baz",
		},
	}

	tests := []struct {
		name       string
		preEvents  []CollectorEvent
		postEvents [][]CollectorEvent
		filter     *Filter
		expected   []EventBundle
	}{
		{
			// will receive events for entities that are currently
			// in the store. entities that were deleted before the
			// subscription should not generate events.
			name: "receive events for entities in the store pre-subscription",
			preEvents: []CollectorEvent{
				{
					Type:   EventTypeSet,
					Source: fooSource,
					Entity: fooContainer,
				},
				{
					Type:   EventTypeSet,
					Source: fooSource,
					Entity: barContainer,
				},
				{
					Type:   EventTypeUnset,
					Source: fooSource,
					Entity: barContainer,
				},
			},
			expected: []EventBundle{
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
			},
		},
		{
			// if the filter has type "EventTypeUnset", it does not receive
			// events for entities that are currently in the store.
			name:   "do not receive events for entities in the store pre-subscription if filter type is EventTypeUnset",
			filter: NewFilter(nil, fooSource, EventTypeUnset),
			preEvents: []CollectorEvent{
				{
					Type:   EventTypeSet,
					Source: fooSource,
					Entity: fooContainer,
				},
			},
			expected: []EventBundle{
				{},
			},
		},
		{
			// will receive events for entities that are currently
			// in the store, and match a filter by source. entities
			// that don't match the filter at all should not
			// generate an event.
			name:   "receive events for entities in the store pre-subscription with filter",
			filter: NewFilter(nil, fooSource, EventTypeAll),
			preEvents: []CollectorEvent{
				// set container with two sources, delete one source
				{
					Type:   EventTypeSet,
					Source: fooSource,
					Entity: fooContainer,
				},
				{
					Type:   EventTypeSet,
					Source: barSource,
					Entity: fooContainer,
				},
				{
					Type:   EventTypeUnset,
					Source: barSource,
					Entity: fooContainer,
				},

				// set container with two sources, keep them
				{
					Type:   EventTypeSet,
					Source: fooSource,
					Entity: barContainer,
				},
				{
					Type:   EventTypeSet,
					Source: barSource,
					Entity: barContainer,
				},

				// set a container for source that should be
				// filtered out
				{
					Type:   EventTypeSet,
					Source: barSource,
					Entity: bazContainer,
				},
			},
			expected: []EventBundle{
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: barContainer,
						},
						{
							Type:   EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
			},
		},
		{
			// same as previous test, but now after the subscription started
			name: "merges entities from different sources post-subscription",
			postEvents: [][]CollectorEvent{
				{
					{
						Type:   EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   EventTypeSet,
						Source: barSource,
						Entity: fooContainerToMerge,
					},
				},
			},
			expected: []EventBundle{
				{},
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: fooContainerMerged,
						},
					},
				},
			},
		},
		{
			// an event about an entity generated from two
			// different sources gets merged into a single entity
			// containing data from both events
			name: "merges entities from different sources pre-subscription",
			preEvents: []CollectorEvent{
				{
					Type:   EventTypeSet,
					Source: fooSource,
					Entity: fooContainer,
				},
				{
					Type:   EventTypeSet,
					Source: barSource,
					Entity: fooContainerToMerge,
				},
			},
			expected: []EventBundle{
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: fooContainerMerged,
						},
					},
				},
			},
		},
		{
			name: "sets and unsets an entity",
			postEvents: [][]CollectorEvent{
				{
					{
						Type:   EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   EventTypeUnset,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
			},
			expected: []EventBundle{
				{},
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
				{
					Events: []Event{
						{
							Type:   EventTypeUnset,
							Entity: fooContainer,
						},
					},
				},
			},
		},
		{
			// setting an entity from two different sources, but
			// unsetting from only one (that matches the filter)
			// correctly generates an unset event
			name:   "sets and unsets an entity with source filters",
			filter: NewFilter(nil, fooSource, EventTypeAll),
			postEvents: [][]CollectorEvent{
				{
					{
						Type:   EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   EventTypeSet,
						Source: barSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   EventTypeUnset,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
			},
			expected: []EventBundle{
				{},
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
				{
					Events: []Event{
						{
							Type:   EventTypeUnset,
							Entity: fooContainer,
						},
					},
				},
			},
		},
		{
			// setting an entity from two different sources, and
			// unsetting one of them, correctly generates a three
			// sets and no unsets
			name:   "sets and unsets an entity from different sources",
			filter: nil,
			postEvents: [][]CollectorEvent{
				{
					{
						Type:   EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   EventTypeSet,
						Source: barSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   EventTypeUnset,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
			},
			expected: []EventBundle{
				{},
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
			},
		},
		{
			// unsetting an unknown entity should generate no events
			name:   "unsets unknown entity",
			filter: nil,
			postEvents: [][]CollectorEvent{
				{
					{
						Type:   EventTypeUnset,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
			},
			expected: []EventBundle{
				{},
			},
		},
		{
			// unsetting an entity with a non-empty state (as in,
			// emitting data in other fields instead of just a
			// wrapped EntityID) merges that with the last known
			// state of the entity before deletion.
			name:   "unsetting entity merges last known state",
			filter: nil,
			postEvents: [][]CollectorEvent{
				{
					{
						Type:   EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
					{
						Type:   EventTypeUnset,
						Source: fooSource,
						Entity: fooContainerToMerge,
					},
				},
			},
			expected: []EventBundle{
				{},
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: fooContainer,
						},
						{
							Type: EventTypeUnset,
							Entity: &Container{
								EntityID: fooContainer.EntityID,
								EntityMeta: EntityMeta{
									Name: fooContainer.Name,
								},
								Hostname: fooContainer.Hostname,
								PID:      fooContainerToMerge.PID,
							},
						},
					},
				},
			},
		},
		{
			name:   "filters by event type",
			filter: NewFilter(nil, SourceAll, EventTypeUnset),
			postEvents: [][]CollectorEvent{
				{
					{
						Type:   EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   EventTypeUnset,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
			},
			expected: []EventBundle{
				{},
				{
					Events: []Event{
						{
							Type:   EventTypeUnset,
							Entity: fooContainer,
						},
					},
				},
			},
		},
		{
			name:      "sets unchanged entity twice",
			preEvents: []CollectorEvent{},
			postEvents: [][]CollectorEvent{
				{
					{
						Type:   EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
					{
						Type:   EventTypeSet,
						Source: fooSource,
						// DeepCopy to ensure we're not
						// just comparing pointers, as
						// collectors return a freshly
						// built object every time
						Entity: fooContainer.DeepCopy(),
					},
				},
			},
			filter: nil,
			expected: []EventBundle{
				{},
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore()

			s.handleEvents(tt.preEvents)

			ch := s.Subscribe(dummySubscriber, NormalPriority, tt.filter)
			doneCh := make(chan struct{})

			actual := []EventBundle{}
			go func() {
				for bundle := range ch {
					close(bundle.Ch)

					// nil the bundle's Ch so we can
					// deep-equal just the events later
					bundle.Ch = nil

					actual = append(actual, bundle)
				}

				close(doneCh)
			}()

			for _, events := range tt.postEvents {
				s.handleEvents(events)
			}

			s.Unsubscribe(ch)

			<-doneCh
			assert.Equal(t, tt.expected, actual)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetKubernetesDeployment(t *testing.T) {
	s := newTestStore()

	deployment := &KubernetesDeployment{
		EntityID: EntityID{
			Kind: KindKubernetesDeployment,
			ID:   "datadog-cluster-agent",
		},
	}

	s.handleEvents([]CollectorEvent{
		{
			Type:   EventTypeSet,
			Source: fooSource,
			Entity: deployment,
		},
	})

	retrievedDeployment, err := s.GetKubernetesDeployment("datadog-cluster-agent")
	assert.NoError(t, err)

	if !reflect.DeepEqual(deployment, retrievedDeployment) {
		t.Errorf("expected deployment %q to match the one in the store", retrievedDeployment.ID)
	}

	s.handleEvents([]CollectorEvent{
		{
			Type:   EventTypeUnset,
			Source: fooSource,
			Entity: deployment,
		},
	})

	_, err = s.GetKubernetesDeployment("datadog-cluster-agent")
	assert.True(t, errors.IsNotFound(err))
}

func TestGetProcess(t *testing.T) {
	s := newTestStore()

	process := &Process{
		EntityID: EntityID{
			Kind: KindProcess,
			ID:   "123",
		},
	}

	s.handleEvents([]CollectorEvent{
		{
			Type:   EventTypeSet,
			Source: fooSource,
			Entity: process,
		},
	})

	gotProcess, err := s.GetProcess(123)
	if err != nil {
		t.Errorf("expected to find process %q, not found", process.ID)
	}

	if !reflect.DeepEqual(process, gotProcess) {
		t.Errorf("expected process %q to match the one in the store", process.ID)
	}

	s.handleEvents([]CollectorEvent{
		{
			Type:   EventTypeUnset,
			Source: fooSource,
			Entity: process,
		},
	})

	_, err = s.GetProcess(123)
	if err == nil || !errors.IsNotFound(err) {
		t.Errorf("expected process %q to be absent. found or had errors. err: %q", process.ID, err)
	}
}

func TestListContainers(t *testing.T) {
	container := &Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "abc",
		},
	}

	tests := []struct {
		name               string
		preEvents          []CollectorEvent
		expectedContainers []*Container
	}{
		{
			name: "some containers stored",
			preEvents: []CollectorEvent{
				{
					Type:   EventTypeSet,
					Source: fooSource,
					Entity: container,
				},
			},
			expectedContainers: []*Container{container},
		},
		{
			name:               "no containers stored",
			preEvents:          nil,
			expectedContainers: []*Container{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testStore := newTestStore()
			testStore.handleEvents(test.preEvents)

			containers := testStore.ListContainers()

			assert.Equal(t, test.expectedContainers, containers)
		})
	}
}

func TestListContainersWithFilter(t *testing.T) {
	runningContainer := &Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "1",
		},
		State: ContainerState{
			Running: true,
		},
	}

	nonRunningContainer := &Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "2",
		},
		State: ContainerState{
			Running: false,
		},
	}

	testStore := newTestStore()

	testStore.handleEvents([]CollectorEvent{
		{
			Type:   EventTypeSet,
			Source: fooSource,
			Entity: runningContainer,
		},
		{
			Type:   EventTypeSet,
			Source: fooSource,
			Entity: nonRunningContainer,
		},
	})

	runningContainers := testStore.ListContainersWithFilter(GetRunningContainers)

	assert.Equal(t, []*Container{runningContainer}, runningContainers)
}

func TestListProcesses(t *testing.T) {
	process := &Process{
		EntityID: EntityID{
			Kind: KindProcess,
			ID:   "123",
		},
	}

	tests := []struct {
		name              string
		preEvents         []CollectorEvent
		expectedProcesses []*Process
	}{
		{
			name: "some processes stored",
			preEvents: []CollectorEvent{
				{
					Type:   EventTypeSet,
					Source: fooSource,
					Entity: process,
				},
			},
			expectedProcesses: []*Process{process},
		},
		{
			name:              "no processes stored",
			preEvents:         nil,
			expectedProcesses: []*Process{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testStore := newTestStore()
			testStore.handleEvents(test.preEvents)

			processes := testStore.ListProcesses()

			assert.Equal(t, test.expectedProcesses, processes)
		})
	}
}

func TestListProcessesWithFilter(t *testing.T) {
	javaProcess := &Process{
		EntityID: EntityID{
			Kind: KindProcess,
			ID:   "123",
		},
		Language: &languagemodels.Language{
			Name: languagemodels.Java,
		},
	}

	nodeProcess := &Process{
		EntityID: EntityID{
			Kind: KindProcess,
			ID:   "2",
		},
		Language: &languagemodels.Language{
			Name: languagemodels.Node,
		},
	}

	testStore := newTestStore()

	testStore.handleEvents([]CollectorEvent{
		{
			Type:   EventTypeSet,
			Source: fooSource,
			Entity: javaProcess,
		},
		{
			Type:   EventTypeSet,
			Source: fooSource,
			Entity: nodeProcess,
		},
	})

	retrievedProcesses := testStore.ListProcessesWithFilter(func(p *Process) bool {
		return p.Language.Name == languagemodels.Java
	})

	assert.Equal(t, []*Process{javaProcess}, retrievedProcesses)
}

func TestListImages(t *testing.T) {
	image := &ContainerImageMetadata{
		EntityID: EntityID{
			Kind: KindContainerImageMetadata,
			ID:   "abc",
		},
	}

	tests := []struct {
		name           string
		preEvents      []CollectorEvent
		expectedImages []*ContainerImageMetadata
	}{
		{
			name: "some images stored",
			preEvents: []CollectorEvent{
				{
					Type:   EventTypeSet,
					Source: fooSource,
					Entity: image,
				},
			},
			expectedImages: []*ContainerImageMetadata{image},
		},
		{
			name:           "no containers stored",
			preEvents:      nil,
			expectedImages: []*ContainerImageMetadata{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testStore := newTestStore()
			testStore.handleEvents(test.preEvents)

			assert.Equal(t, test.expectedImages, testStore.ListImages())
		})
	}
}

func TestGetImage(t *testing.T) {
	image := &ContainerImageMetadata{
		EntityID: EntityID{
			Kind: KindContainerImageMetadata,
			ID:   "abc",
		},
	}

	tests := []struct {
		name          string
		imageID       string
		preEvents     []CollectorEvent
		expectedImage *ContainerImageMetadata
		expectsError  bool
	}{
		{
			name:    "image exists",
			imageID: image.ID,
			preEvents: []CollectorEvent{
				{
					Type:   EventTypeSet,
					Source: fooSource,
					Entity: image,
				},
			},
			expectedImage: image,
		},
		{
			name:         "image does not exist",
			imageID:      "non_existing_ID",
			preEvents:    nil,
			expectsError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testStore := newTestStore()
			testStore.handleEvents(test.preEvents)

			actualImage, err := testStore.GetImage(test.imageID)

			if test.expectsError {
				assert.Error(t, err, errors.NewNotFound(string(KindContainerImageMetadata)).Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedImage, actualImage)
			}
		})
	}
}

func TestResetProcesses(t *testing.T) {
	tests := []struct {
		name         string
		preEvents    []CollectorEvent
		newProcesses []*Process
	}{
		{
			name:      "initially empty",
			preEvents: []CollectorEvent{},
			newProcesses: []*Process{
				{
					EntityID: EntityID{
						Kind: KindProcess,
						ID:   "123",
					},
				},
			},
		},
		{
			name: "old process to be removed",
			preEvents: []CollectorEvent{
				{
					Type:   EventTypeSet,
					Source: SourceRemoteProcessCollector,
					Entity: &Process{
						EntityID: EntityID{
							Kind: KindProcess,
							ID:   "123",
						},
					},
				},
			},
			newProcesses: []*Process{
				{
					EntityID: EntityID{
						Kind: KindProcess,
						ID:   "345",
					},
				},
			},
		},
		{
			name: "old process to be updated",
			preEvents: []CollectorEvent{
				{
					Type:   EventTypeSet,
					Source: SourceRemoteProcessCollector,
					Entity: &Process{
						EntityID: EntityID{
							Kind: KindProcess,
							ID:   "123",
						},
					},
				},
			},
			newProcesses: []*Process{
				{
					EntityID: EntityID{
						Kind: KindProcess,
						ID:   "123",
					},
					NsPid: 345,
				},
				{
					EntityID: EntityID{
						Kind: KindProcess,
						ID:   "12",
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testStore := newTestStore()
			testStore.handleEvents(test.preEvents)

			ch := testStore.Subscribe(dummySubscriber, NormalPriority, nil)
			doneCh := make(chan struct{})

			go func() {
				for bundle := range ch {
					close(bundle.Ch)

					// nil the bundle's Ch so we can deep-equal just the events
					// later
					bundle.Ch = nil
				}

				close(doneCh)
			}()

			entities := make([]Entity, len(test.newProcesses))
			for i := range test.newProcesses {
				entities[i] = test.newProcesses[i]
			}
			testStore.ResetProcesses(entities, SourceRemoteProcessCollector)
			// Force handling of events generated by the reset
			if len(testStore.eventCh) > 0 {
				testStore.handleEvents(<-testStore.eventCh)
			}

			testStore.Unsubscribe(ch)

			<-doneCh

			processes := testStore.ListProcesses()
			assert.ElementsMatch(t, processes, test.newProcesses)
		})
	}

}

func TestReset(t *testing.T) {
	fooContainer := &Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "foo",
		},
		EntityMeta: EntityMeta{
			Name: "foo",
		},
		Hostname: "foo",
	}

	fooSetEvent := CollectorEvent{
		Type:   EventTypeSet,
		Source: fooSource,
		Entity: fooContainer,
	}

	// Same ID as fooContainer but with different values
	updatedFooContainer := &Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "foo",
		},
		EntityMeta: EntityMeta{
			Name: "foo",
			Labels: map[string]string{ // Added
				"test-label": "1",
			},
		},
		Hostname: "foo",
	}

	barContainer := &Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "bar",
		},
		EntityMeta: EntityMeta{
			Name: "bar",
		},
		Hostname: "bar",
	}

	tests := []struct {
		name                   string
		preEvents              []CollectorEvent
		newEntities            []Entity
		expectedEventsReceived []EventBundle
	}{
		{
			name: "new entity already exists without changes",
			preEvents: []CollectorEvent{
				fooSetEvent,
			},
			newEntities: []Entity{
				fooContainer,
			},
			expectedEventsReceived: []EventBundle{
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
			},
		},
		{
			name: "new entity exists but it has been updated",
			preEvents: []CollectorEvent{
				fooSetEvent,
			},
			newEntities: []Entity{
				updatedFooContainer,
			},
			expectedEventsReceived: []EventBundle{
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: updatedFooContainer,
						},
					},
				},
			},
		},
		{
			name: "new event does not exist",
			preEvents: []CollectorEvent{
				fooSetEvent,
			},
			newEntities: []Entity{
				fooContainer,
				barContainer,
			},
			expectedEventsReceived: []EventBundle{
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: barContainer,
						},
					},
				},
			},
		},
		{
			name: "an event that exists is not included in the list of new ones",
			preEvents: []CollectorEvent{
				fooSetEvent,
			},
			newEntities: []Entity{},
			expectedEventsReceived: []EventBundle{
				{
					Events: []Event{
						{
							Type:   EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
				{
					Events: []Event{
						{
							Type:   EventTypeUnset,
							Entity: fooContainer,
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := newTestStore()

			s.handleEvents(test.preEvents)

			ch := s.Subscribe(dummySubscriber, NormalPriority, nil)
			doneCh := make(chan struct{})

			var actualEventsReceived []EventBundle
			go func() {
				for bundle := range ch {
					close(bundle.Ch)

					// nil the bundle's Ch so we can deep-equal just the events
					// later
					bundle.Ch = nil

					actualEventsReceived = append(actualEventsReceived, bundle)
				}

				close(doneCh)
			}()

			s.Reset(test.newEntities, fooSource)

			// Force handling of events generated by the reset
			if len(s.eventCh) > 0 {
				s.handleEvents(<-s.eventCh)
			}

			s.Unsubscribe(ch)

			<-doneCh

			assert.Equal(t, test.expectedEventsReceived, actualEventsReceived)
		})
	}
}

func TestNoDataRace(t *testing.T) {
	// This test ensures that no race conditions are encountered when the "--race" flag is passed
	// to the test process and an entity is accessed in a different thread than the one handling events
	s := newTestStore()

	container := &Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "123",
		},
	}

	go func() {
		_, _ = s.GetContainer("456")
	}()

	s.handleEvents([]CollectorEvent{
		{
			Type:   EventTypeSet,
			Source: fooSource,
			Entity: container,
		},
	})
}

func newTestStore() *store {
	return &store{
		store:   make(map[Kind]map[string]*cachedEntity),
		eventCh: make(chan []CollectorEvent, eventChBufferSize),
	}
}
