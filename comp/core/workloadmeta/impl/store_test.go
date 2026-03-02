// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package workloadmetaimpl

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

const (
	dummySubscriber = "subscriber"
	fooSource       = "foo"
	barSource       = "bar"
)

func newWorkloadmetaObject(t *testing.T) *workloadmeta {
	deps := Dependencies{
		Lc:     compdef.NewTestLifecycle(t),
		Log:    logmock.New(t),
		Config: config.NewMock(t),
		Params: wmdef.NewParams(),
	}

	return NewWorkloadMeta(deps).Comp.(*workloadmeta)
}

func TestHandleEvents(t *testing.T) {
	s := newWorkloadmetaObject(t)

	container := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "deadbeef",
		},
	}

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
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

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeUnset,
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
	fooContainer := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "foo",
		},
		EntityMeta: wmdef.EntityMeta{
			Name: "foo-name-might-be-overridden",
		},
		Hostname: "foo",
	}

	fooContainerToMerge := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "foo",
		},
		EntityMeta: wmdef.EntityMeta{
			Name: "foo-name-override",
		},
		PID: 1001001,
	}

	fooContainerMerged := &wmdef.Container{
		EntityID: fooContainer.EntityID,
		EntityMeta: wmdef.EntityMeta{
			Name: fooContainerToMerge.Name,
		},
		Hostname: fooContainer.Hostname,
		PID:      fooContainerToMerge.PID,
	}

	barContainer := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "bar",
		},
	}

	bazContainer := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "baz",
		},
	}

	testNodeMetadata := wmdef.KubernetesMetadata{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesMetadata,
			ID:   string(util.GenerateKubeMetadataEntityID("", "nodes", "", "test-node")),
		},
		EntityMeta: wmdef.EntityMeta{
			Name: "test-node",
			Labels: map[string]string{
				"test-label": "test-value",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
		},
		GVR: &schema.GroupVersionResource{
			Version:  "v1",
			Resource: "nodes",
		},
	}

	tests := []struct {
		name       string
		preEvents  []wmdef.CollectorEvent
		postEvents [][]wmdef.CollectorEvent
		filter     *wmdef.Filter
		expected   []wmdef.EventBundle
	}{
		{
			// will receive events for entities that are currently
			// in the store. entities that were deleted before the
			// subscription should not generate events.
			name: "receive events for entities in the store pre-subscription",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: fooContainer,
				},
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: barContainer,
				},
				{
					Type:   wmdef.EventTypeUnset,
					Source: fooSource,
					Entity: barContainer,
				},
			},
			expected: []wmdef.EventBundle{
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     fooContainer,
							IsComplete: true,
						},
					},
				},
			},
		},
		{
			// if the filter has type "wmdef.EventTypeUnset", it does not receive
			// events for entities that are currently in the store.
			name:   "do not receive events for entities in the store pre-subscription if filter type is EventTypeUnset",
			filter: wmdef.NewFilterBuilder().SetSource(fooSource).SetEventType(wmdef.EventTypeUnset).Build(),
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: fooContainer,
				},
			},
			expected: []wmdef.EventBundle{
				{},
			},
		},
		{
			// will receive events for entities that are currently
			// in the store, and match a filter by source. entities
			// that don't match the filter at all should not
			// generate an event.
			name:   "receive events for entities in the store pre-subscription with filter",
			filter: wmdef.NewFilterBuilder().SetSource(fooSource).Build(),
			preEvents: []wmdef.CollectorEvent{
				// set container with two sources, delete one source
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: fooContainer,
				},
				{
					Type:   wmdef.EventTypeSet,
					Source: barSource,
					Entity: fooContainer,
				},
				{
					Type:   wmdef.EventTypeUnset,
					Source: barSource,
					Entity: fooContainer,
				},

				// set container with two sources, keep them
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: barContainer,
				},
				{
					Type:   wmdef.EventTypeSet,
					Source: barSource,
					Entity: barContainer,
				},

				// set a container for source that should be
				// filtered out
				{
					Type:   wmdef.EventTypeSet,
					Source: barSource,
					Entity: bazContainer,
				},
			},
			expected: []wmdef.EventBundle{
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     barContainer,
							IsComplete: true,
						},
						{
							Type:       wmdef.EventTypeSet,
							Entity:     fooContainer,
							IsComplete: true,
						},
					},
				},
			},
		},
		{
			// same as previous test, but now after the subscription started
			name: "merges entities from different sources post-subscription",
			postEvents: [][]wmdef.CollectorEvent{
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: barSource,
						Entity: fooContainerToMerge,
					},
				},
			},
			expected: []wmdef.EventBundle{
				{},
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     fooContainer,
							IsComplete: true,
						},
					},
				},
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     fooContainerMerged,
							IsComplete: true,
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
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: fooContainer,
				},
				{
					Type:   wmdef.EventTypeSet,
					Source: barSource,
					Entity: fooContainerToMerge,
				},
			},
			expected: []wmdef.EventBundle{
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     fooContainerMerged,
							IsComplete: true,
						},
					},
				},
			},
		},
		{
			name: "sets and unsets an entity",
			postEvents: [][]wmdef.CollectorEvent{
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   wmdef.EventTypeUnset,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
			},
			expected: []wmdef.EventBundle{
				{},
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     fooContainer,
							IsComplete: true,
						},
					},
				},
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeUnset,
							Entity:     fooContainer,
							IsComplete: true,
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
			filter: wmdef.NewFilterBuilder().SetSource(fooSource).Build(),
			postEvents: [][]wmdef.CollectorEvent{
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: barSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   wmdef.EventTypeUnset,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
			},
			expected: []wmdef.EventBundle{
				{},
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     fooContainer,
							IsComplete: true,
						},
					},
				},
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeUnset,
							Entity:     fooContainer,
							IsComplete: true,
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
			postEvents: [][]wmdef.CollectorEvent{
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: barSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   wmdef.EventTypeUnset,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
			},
			expected: []wmdef.EventBundle{
				{},
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     fooContainer,
							IsComplete: true,
						},
					},
				},
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     fooContainer,
							IsComplete: true,
						},
					},
				},
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     fooContainer,
							IsComplete: true,
						},
					},
				},
			},
		},
		{
			// unsetting an unknown entity should generate no events
			name:   "unsets unknown entity",
			filter: nil,
			postEvents: [][]wmdef.CollectorEvent{
				{
					{
						Type:   wmdef.EventTypeUnset,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
			},
			expected: []wmdef.EventBundle{
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
			postEvents: [][]wmdef.CollectorEvent{
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
					{
						Type:   wmdef.EventTypeUnset,
						Source: fooSource,
						Entity: fooContainerToMerge,
					},
				},
			},
			expected: []wmdef.EventBundle{
				{},
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     fooContainer,
							IsComplete: true,
						},
						{
							Type: wmdef.EventTypeUnset,
							Entity: &wmdef.Container{
								EntityID: fooContainer.EntityID,
								EntityMeta: wmdef.EntityMeta{
									Name: fooContainer.Name,
								},
								Hostname: fooContainer.Hostname,
								PID:      fooContainerToMerge.PID,
							},
							IsComplete: true,
						},
					},
				},
			},
		},
		{
			name:   "filters by event type",
			filter: wmdef.NewFilterBuilder().SetEventType(wmdef.EventTypeUnset).Build(),
			postEvents: [][]wmdef.CollectorEvent{
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   wmdef.EventTypeUnset,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
			},
			expected: []wmdef.EventBundle{
				{},
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeUnset,
							Entity:     fooContainer,
							IsComplete: true,
						},
					},
				},
			},
		},
		{
			name:      "sets unchanged entity twice",
			preEvents: []wmdef.CollectorEvent{},
			postEvents: [][]wmdef.CollectorEvent{
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
					{
						Type:   wmdef.EventTypeSet,
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
			expected: []wmdef.EventBundle{
				{},
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     fooContainer,
							IsComplete: true,
						},
					},
				},
			},
		},
		{
			name: "set and unset with a filter that matches entities",
			// The purpose of this test is to check that a filter that matches
			// entities using an attribute that is not present in the unset
			// event still works.
			// We need to support this case because some collectors do not send
			// the whole entity in "unset" events and only send an ID instead.
			preEvents: []wmdef.CollectorEvent{},
			postEvents: [][]wmdef.CollectorEvent{
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: &testNodeMetadata,
					},
					{
						Type:   wmdef.EventTypeUnset,
						Source: fooSource,
						// Notice that this unset event does not contain the
						// full entity.
						Entity: &wmdef.KubernetesMetadata{
							EntityID: wmdef.EntityID{
								Kind: wmdef.KindKubernetesMetadata,
								ID:   testNodeMetadata.ID,
							},
						},
					},
				},
			},
			filter: wmdef.NewFilterBuilder().AddKindWithEntityFilter(
				wmdef.KindKubernetesMetadata,
				func(entity wmdef.Entity) bool {
					metadata := entity.(*wmdef.KubernetesMetadata)
					// Notice that this filter relies on data that is not
					// available in the unset event.
					return wmdef.IsNodeMetadata(metadata)
				},
			).Build(),
			expected: []wmdef.EventBundle{
				{},
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     &testNodeMetadata,
							IsComplete: true,
						},
						{
							Type:       wmdef.EventTypeUnset,
							Entity:     &testNodeMetadata,
							IsComplete: true,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newWorkloadmetaObject(t)

			s.handleEvents(tt.preEvents)

			ch := s.Subscribe(dummySubscriber, wmdef.NormalPriority, tt.filter)
			doneCh := make(chan struct{})

			actual := []wmdef.EventBundle{}
			go func() {
				for bundle := range ch {
					bundle.Acknowledge()

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
		})
	}
}

func TestGetKubernetesDeployment(t *testing.T) {
	s := newWorkloadmetaObject(t)

	deployment := &wmdef.KubernetesDeployment{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesDeployment,
			ID:   "datadog-cluster-agent",
		},
	}

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: deployment,
		},
	})

	retrievedDeployment, err := s.GetKubernetesDeployment("datadog-cluster-agent")
	assert.NoError(t, err)

	if !reflect.DeepEqual(deployment, retrievedDeployment) {
		t.Errorf("expected deployment %q to match the one in the store", retrievedDeployment.ID)
	}

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeUnset,
			Source: fooSource,
			Entity: deployment,
		},
	})

	_, err = s.GetKubernetesDeployment("datadog-cluster-agent")
	assert.True(t, errors.IsNotFound(err))
}

func TestGetProcess(t *testing.T) {
	s := newWorkloadmetaObject(t)

	process := &wmdef.Process{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindProcess,
			ID:   "123",
		},
	}

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
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

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeUnset,
			Source: fooSource,
			Entity: process,
		},
	})

	_, err = s.GetProcess(123)
	if err == nil || !errors.IsNotFound(err) {
		t.Errorf("expected process %q to be absent. found or had errors. err: %q", process.ID, err)
	}
}

func TestGetContainerForProcess(t *testing.T) {
	for _, tc := range []struct {
		description       string
		processData       []*wmdef.Process
		containerData     []*wmdef.Container
		pidToQuery        string
		expectedContainer *wmdef.Container
		expectedError     error
	}{
		{
			description: "process with container",
			processData: []*wmdef.Process{
				{
					EntityID: wmdef.EntityID{
						Kind: wmdef.KindProcess,
						ID:   "123",
					},
					Owner: &wmdef.EntityID{
						Kind: wmdef.KindContainer,
						ID:   "container_id1",
					},
				},
				{
					EntityID: wmdef.EntityID{
						Kind: wmdef.KindProcess,
						ID:   "234",
					},
					Owner: &wmdef.EntityID{
						Kind: wmdef.KindContainer,
						ID:   "container_id2",
					},
				},
			},
			containerData: []*wmdef.Container{
				{
					EntityID: wmdef.EntityID{
						Kind: wmdef.KindContainer,
						ID:   "container_id1",
					},
				},
				{
					EntityID: wmdef.EntityID{
						Kind: wmdef.KindContainer,
						ID:   "container_id2",
					},
				},
			},
			pidToQuery: "123",
			expectedContainer: &wmdef.Container{
				EntityID: wmdef.EntityID{
					Kind: wmdef.KindContainer,
					ID:   "container_id1",
				},
			},
		},
		{
			description: "process with no container id",
			processData: []*wmdef.Process{
				{
					EntityID: wmdef.EntityID{
						Kind: wmdef.KindProcess,
						ID:   "123",
					},
				},
			},
			containerData:     []*wmdef.Container{},
			pidToQuery:        "123",
			expectedContainer: nil,
			expectedError:     errors.NewNotFound("123"),
		},
		{
			description: "process and container does not exist",
			processData: []*wmdef.Process{
				{
					EntityID: wmdef.EntityID{
						Kind: wmdef.KindProcess,
						ID:   "123",
					},
					Owner: &wmdef.EntityID{
						Kind: wmdef.KindContainer,
						ID:   "container_id1",
					},
				},
			},
			containerData:     []*wmdef.Container{},
			pidToQuery:        "123",
			expectedContainer: nil,
			expectedError:     errors.NewNotFound("container_id1"),
		},
		{
			description:   "process does not exist",
			processData:   []*wmdef.Process{},
			containerData: []*wmdef.Container{},
			pidToQuery:    "123",
			expectedError: errors.NewNotFound("process"),
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			s := newWorkloadmetaObject(t)

			// store process data into wlm
			for _, proc := range tc.processData {
				s.handleEvents([]wmdef.CollectorEvent{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: proc,
					},
				})
			}

			// store container data into wlm
			for _, container := range tc.containerData {
				s.handleEvents([]wmdef.CollectorEvent{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: container,
					},
				})
			}

			// Testing
			container, err := s.GetContainerForProcess(tc.pidToQuery)
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, *tc.expectedContainer, *container)
			}
		})
	}
}

func TestListContainers(t *testing.T) {
	container := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "abc",
		},
	}

	tests := []struct {
		name               string
		preEvents          []wmdef.CollectorEvent
		expectedContainers []*wmdef.Container
	}{
		{
			name: "some containers stored",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: container,
				},
			},
			expectedContainers: []*wmdef.Container{container},
		},
		{
			name:               "no containers stored",
			preEvents:          nil,
			expectedContainers: []*wmdef.Container{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := newWorkloadmetaObject(t)

			s.handleEvents(test.preEvents)

			containers := s.ListContainers()

			assert.Equal(t, test.expectedContainers, containers)
		})
	}
}

func TestListContainersWithFilter(t *testing.T) {
	runningContainer := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "1",
		},
		State: wmdef.ContainerState{
			Running: true,
		},
	}

	nonRunningContainer := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "2",
		},
		State: wmdef.ContainerState{
			Running: false,
		},
	}

	s := newWorkloadmetaObject(t)

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: runningContainer,
		},
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: nonRunningContainer,
		},
	})

	runningContainers := s.ListContainersWithFilter(wmdef.GetRunningContainers)

	assert.Equal(t, []*wmdef.Container{runningContainer}, runningContainers)
}

func TestListProcesses(t *testing.T) {
	process := &wmdef.Process{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindProcess,
			ID:   "123",
		},
	}

	tests := []struct {
		name              string
		preEvents         []wmdef.CollectorEvent
		expectedProcesses []*wmdef.Process
	}{
		{
			name: "some processes stored",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: process,
				},
			},
			expectedProcesses: []*wmdef.Process{process},
		},
		{
			name:              "no processes stored",
			preEvents:         nil,
			expectedProcesses: []*wmdef.Process{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := newWorkloadmetaObject(t)

			s.handleEvents(test.preEvents)

			processes := s.ListProcesses()

			assert.Equal(t, test.expectedProcesses, processes)
		})
	}
}

func TestListProcessesWithFilter(t *testing.T) {
	javaProcess := &wmdef.Process{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindProcess,
			ID:   "123",
		},
		Language: &languagemodels.Language{
			Name: languagemodels.Java,
		},
	}

	nodeProcess := &wmdef.Process{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindProcess,
			ID:   "2",
		},
		Language: &languagemodels.Language{
			Name: languagemodels.Node,
		},
	}

	s := newWorkloadmetaObject(t)

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: javaProcess,
		},
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: nodeProcess,
		},
	})

	retrievedProcesses := s.ListProcessesWithFilter(func(p *wmdef.Process) bool {
		return p.Language.Name == languagemodels.Java
	})

	assert.Equal(t, []*wmdef.Process{javaProcess}, retrievedProcesses)
}

func TestGetKubernetesPodByName(t *testing.T) {
	pod1 := &wmdef.KubernetesPod{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesPod,
			ID:   "123",
		},
		EntityMeta: wmdef.EntityMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
		},
	}
	pod2 := &wmdef.KubernetesPod{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesPod,
			ID:   "234",
		},
		EntityMeta: wmdef.EntityMeta{
			Name:      "test-pod-other",
			Namespace: "test-namespace",
		},
	}
	pod3 := &wmdef.KubernetesPod{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesPod,
			ID:   "345",
		},
		EntityMeta: wmdef.EntityMeta{
			Name:      "test-pod",
			Namespace: "test-namespace-other",
		},
	}

	type want struct {
		pod *wmdef.KubernetesPod
		err error
	}
	type args struct {
		podName      string
		podNamespace string
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "test-pod/test-namespace returns correct pod",
			args: args{
				podName:      "test-pod",
				podNamespace: "test-namespace",
			},
			want: want{
				pod: pod1,
			},
		},
		{
			name: "test-pod-other/test-namespace returns correct pod",
			args: args{
				podName:      "test-pod-other",
				podNamespace: "test-namespace",
			},
			want: want{
				pod: pod2,
			},
		},
		{
			name: "test-pod/test-namespace-other returns correct pod",
			args: args{
				podName:      "test-pod",
				podNamespace: "test-namespace-other",
			},
			want: want{
				pod: pod3,
			},
		},
		{
			name: "missing pod returns error",
			args: args{
				podName:      "test-pod-other",
				podNamespace: "test-namespace-other",
			},
			want: want{
				err: errors.NewNotFound("test-pod-other"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := newWorkloadmetaObject(t)

			for _, pod := range []*wmdef.KubernetesPod{pod1, pod2, pod3} {
				s.handleEvents([]wmdef.CollectorEvent{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: pod,
					},
				})
			}

			pod, err := s.GetKubernetesPodByName(test.args.podName, test.args.podNamespace)

			assert.Equal(t, test.want.pod, pod)
			if test.want.err != nil {
				assert.Error(t, err, test.want.err.Error())
			}
		})
	}
}

func TestListKubernetesPods(t *testing.T) {
	pod1 := &wmdef.KubernetesPod{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesPod,
			ID:   "123",
		},
	}
	pod2 := &wmdef.KubernetesPod{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesPod,
			ID:   "456",
		},
	}

	tests := []struct {
		name      string
		preEvents []wmdef.CollectorEvent
		expected  []*wmdef.KubernetesPod
	}{
		{
			name: "some pods stored",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: pod1,
				},
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: pod2,
				},
			},
			expected: []*wmdef.KubernetesPod{pod1, pod2},
		},
		{
			name:      "no pods stored",
			preEvents: nil,
			expected:  []*wmdef.KubernetesPod{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wmeta := newWorkloadmetaObject(t)
			wmeta.handleEvents(test.preEvents)

			assert.ElementsMatch(t, test.expected, wmeta.ListKubernetesPods())
		})
	}
}

// TestKubernetesPodMergeOrder confirms that when the same pod is
// reported by both kubelet (node_orchestrator) and kubemetadata (cluster_orchestrator),
// the fresher data from the kubemetadata collector should win.
func TestKubernetesPodMergeOrder(t *testing.T) {
	podID := "pod-uid-123"
	podNamespace := "default"
	podName := "my-pod"

	freshNamespaceLabels := map[string]string{
		"key": "fresh",
	}
	staleNamespaceLabels := map[string]string{
		"key": "stale",
	}

	fromKubemetadata := &wmdef.KubernetesPod{
		EntityID:        wmdef.EntityID{Kind: wmdef.KindKubernetesPod, ID: podID},
		EntityMeta:      wmdef.EntityMeta{Name: podName, Namespace: podNamespace},
		NamespaceLabels: freshNamespaceLabels,
	}
	fromKubelet := &wmdef.KubernetesPod{
		EntityID:        wmdef.EntityID{Kind: wmdef.KindKubernetesPod, ID: podID},
		EntityMeta:      wmdef.EntityMeta{Name: podName, Namespace: podNamespace},
		NamespaceLabels: staleNamespaceLabels,
	}

	s := newWorkloadmetaObject(t)
	// Simulate the kubelet collector reporting the pod with stale namespace labels first
	s.handleEvents([]wmdef.CollectorEvent{
		{Type: wmdef.EventTypeSet, Source: wmdef.SourceNodeOrchestrator, Entity: fromKubelet},
	})
	// Followed by the kubemetadata collector with fresh labels afterwards
	s.handleEvents([]wmdef.CollectorEvent{
		{Type: wmdef.EventTypeSet, Source: wmdef.SourceClusterOrchestrator, Entity: fromKubemetadata},
	})

	got, err := s.GetKubernetesPodByName(podName, podNamespace)
	assert.NoError(t, err)
	assert.Equal(t, freshNamespaceLabels, got.NamespaceLabels,
		"with alphabetical merge order, cluster_orchestrator (kubemetadata) should win")
}

func TestGetKubeletMetrics(t *testing.T) {
	testKubeletMetrics := &wmdef.KubeletMetrics{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubeletMetrics,
			ID:   wmdef.KubeletMetricsID,
		},
		ExpiredPodCount: 10,
	}

	tests := []struct {
		name       string
		preEvents  []wmdef.CollectorEvent
		expected   *wmdef.KubeletMetrics
		expectsErr bool
	}{
		{
			name: "kubelet metrics stored",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: testKubeletMetrics,
				},
			},
			expected:   testKubeletMetrics,
			expectsErr: false,
		},
		{
			name:       "no kubelet metrics stored",
			preEvents:  nil,
			expectsErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wmeta := newWorkloadmetaObject(t)
			wmeta.handleEvents(test.preEvents)

			kubeletMetrics, err := wmeta.GetKubeletMetrics()
			if test.expectsErr {
				assert.Error(t, err, errors.NewNotFound(string(wmdef.KindKubeletMetrics)).Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expected, kubeletMetrics)
			}
		})
	}
}

func TestListImages(t *testing.T) {
	image := &wmdef.ContainerImageMetadata{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainerImageMetadata,
			ID:   "abc",
		},
	}

	tests := []struct {
		name           string
		preEvents      []wmdef.CollectorEvent
		expectedImages []*wmdef.ContainerImageMetadata
	}{
		{
			name: "some images stored",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: image,
				},
			},
			expectedImages: []*wmdef.ContainerImageMetadata{image},
		},
		{
			name:           "no containers stored",
			preEvents:      nil,
			expectedImages: []*wmdef.ContainerImageMetadata{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := newWorkloadmetaObject(t)

			s.handleEvents(test.preEvents)

			assert.ElementsMatch(t, test.expectedImages, s.ListImages())
		})
	}
}

func TestGetImage(t *testing.T) {
	image := &wmdef.ContainerImageMetadata{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainerImageMetadata,
			ID:   "abc",
		},
	}

	tests := []struct {
		name          string
		imageID       string
		preEvents     []wmdef.CollectorEvent
		expectedImage *wmdef.ContainerImageMetadata
		expectsError  bool
	}{
		{
			name:    "image exists",
			imageID: image.ID,
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
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
			s := newWorkloadmetaObject(t)
			s.handleEvents(test.preEvents)

			actualImage, err := s.GetImage(test.imageID)

			if test.expectsError {
				assert.Error(t, err, errors.NewNotFound(string(wmdef.KindContainerImageMetadata)).Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedImage, actualImage)
			}
		})
	}
}

func TestListECSTasks(t *testing.T) {
	task1 := &wmdef.ECSTask{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindECSTask,
			ID:   "task-id-1",
		},
		VPCID: "123",
	}
	task2 := &wmdef.ECSTask{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindECSTask,
			ID:   "task-id-1",
		},
	}
	task3 := &wmdef.ECSTask{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindECSTask,
			ID:   "task-id-2",
		},
	}

	tests := []struct {
		name          string
		preEvents     []wmdef.CollectorEvent
		expectedTasks []*wmdef.ECSTask
	}{
		{
			name: "some tasks stored",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: task1,
				},
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: task2,
				},
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: task3,
				},
			},
			// task2 replaces task1
			expectedTasks: []*wmdef.ECSTask{task2, task3},
		},
		{
			name:          "no task stored",
			preEvents:     nil,
			expectedTasks: []*wmdef.ECSTask{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := newWorkloadmetaObject(t)

			s.handleEvents(test.preEvents)

			tasks := s.ListECSTasks()

			assert.ElementsMatch(t, test.expectedTasks, tasks)
		})
	}
}

func TestResetProcesses(t *testing.T) {
	tests := []struct {
		name         string
		preEvents    []wmdef.CollectorEvent
		newProcesses []*wmdef.Process
	}{
		{
			name:      "initially empty",
			preEvents: []wmdef.CollectorEvent{},
			newProcesses: []*wmdef.Process{
				{
					EntityID: wmdef.EntityID{
						Kind: wmdef.KindProcess,
						ID:   "123",
					},
				},
			},
		},
		{
			name: "old process to be removed",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: wmdef.SourceRemoteProcessCollector,
					Entity: &wmdef.Process{
						EntityID: wmdef.EntityID{
							Kind: wmdef.KindProcess,
							ID:   "123",
						},
					},
				},
			},
			newProcesses: []*wmdef.Process{
				{
					EntityID: wmdef.EntityID{
						Kind: wmdef.KindProcess,
						ID:   "345",
					},
				},
			},
		},
		{
			name: "old process to be updated",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: wmdef.SourceRemoteProcessCollector,
					Entity: &wmdef.Process{
						EntityID: wmdef.EntityID{
							Kind: wmdef.KindProcess,
							ID:   "123",
						},
					},
				},
			},
			newProcesses: []*wmdef.Process{
				{
					EntityID: wmdef.EntityID{
						Kind: wmdef.KindProcess,
						ID:   "123",
					},
					NsPid: 345,
				},
				{
					EntityID: wmdef.EntityID{
						Kind: wmdef.KindProcess,
						ID:   "12",
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := newWorkloadmetaObject(t)
			s.handleEvents(test.preEvents)

			ch := s.Subscribe(dummySubscriber, wmdef.NormalPriority, nil)
			doneCh := make(chan struct{})

			go func() {
				for bundle := range ch {
					bundle.Acknowledge()

					// nil the bundle's Ch so we can deep-equal just the events
					// later
					bundle.Ch = nil
				}

				close(doneCh)
			}()

			entities := make([]wmdef.Entity, len(test.newProcesses))
			for i := range test.newProcesses {
				entities[i] = test.newProcesses[i]
			}
			s.ResetProcesses(entities, wmdef.SourceRemoteProcessCollector)
			// Force handling of events generated by the reset
			if len(s.eventCh) > 0 {
				s.handleEvents(<-s.eventCh)
			}

			s.Unsubscribe(ch)

			<-doneCh

			processes := s.ListProcesses()
			assert.ElementsMatch(t, processes, test.newProcesses)
		})
	}
}

func TestGetKubernetesMetadata(t *testing.T) {
	s := newWorkloadmetaObject(t)

	kubemetadata := &wmdef.KubernetesMetadata{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesMetadata,
			ID:   "deployments/default/app",
		},
	}

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: kubemetadata,
		},
	})

	retrievedMetadata, err := s.GetKubernetesMetadata("deployments/default/app")
	assert.NoError(t, err)

	if !reflect.DeepEqual(kubemetadata, retrievedMetadata) {
		t.Errorf("expected metadata %q to match the one in the store", retrievedMetadata.ID)
	}

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeUnset,
			Source: fooSource,
			Entity: kubemetadata,
		},
	})

	_, err = s.GetKubernetesMetadata("deployments/default/app")
	assert.True(t, errors.IsNotFound(err))
}

func TestGetKubernetesNodeByName(t *testing.T) {
	node1Metadata := &wmdef.KubernetesMetadata{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesMetadata,
			ID:   string(util.GenerateKubeMetadataEntityID("", "nodes", "", "node1")),
		},
		EntityMeta: wmdef.EntityMeta{
			Name:        "node1",
			Annotations: map[string]string{"a1": "v1"},
			Labels:      map[string]string{"l1": "v2"},
		},
		GVR: &schema.GroupVersionResource{
			Version:  "v1",
			Resource: "nodes",
		},
	}

	node2Metadata := &wmdef.KubernetesMetadata{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesMetadata,
			ID:   string(util.GenerateKubeMetadataEntityID("", "nodes", "", "node2")),
		},
		EntityMeta: wmdef.EntityMeta{
			Name:        "node2",
			Annotations: map[string]string{"a1": "v1"},
			Labels:      map[string]string{"l1": "v2"},
		},
		GVR: &schema.GroupVersionResource{
			Version:  "v1",
			Resource: "nodes",
		},
	}

	nonNodeMetadata := &wmdef.KubernetesMetadata{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesMetadata,
			ID:   "deployments/default/app",
		},
		EntityMeta: wmdef.EntityMeta{
			Name:        "node3",
			Namespace:   "default",
			Annotations: map[string]string{"a1": "v1"},
			Labels:      map[string]string{"l1": "v2"},
		},
		GVR: &schema.GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		},
	}
	node3Metadata := &wmdef.KubernetesMetadata{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesMetadata,
			ID:   string(util.GenerateKubeMetadataEntityID("", "nodes", "", "node3")),
		},
		EntityMeta: wmdef.EntityMeta{
			Name:        "node3",
			Annotations: map[string]string{"a1": "v1"},
			Labels:      map[string]string{"l1": "v2"},
		},
		GVR: &schema.GroupVersionResource{
			Version:  "v1",
			Resource: "nodes",
		},
	}

	type want struct {
		nodeMetadata *wmdef.KubernetesMetadata
		err          error
	}
	type args struct {
		nodeName string
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "test-node returns correct node",
			args: args{
				nodeName: "node1",
			},
			want: want{
				nodeMetadata: node1Metadata,
			},
		},
		{
			name: "test-node/other-node returns correct node",
			args: args{
				nodeName: "node2",
			},
			want: want{
				nodeMetadata: node2Metadata,
			},
		},
		{
			name: "test-node/ignores non-node metadata with same name",
			args: args{
				nodeName: "node3",
			},
			want: want{
				nodeMetadata: node3Metadata,
			},
		},
		{
			name: "test-node/missing node returns error",
			args: args{
				nodeName: "node4",
			},
			want: want{
				err: errors.NewNotFound("test-node/node4"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := newWorkloadmetaObject(t)

			for _, nodeish := range []*wmdef.KubernetesMetadata{node1Metadata, node2Metadata, nonNodeMetadata, node3Metadata} {
				s.handleEvents([]wmdef.CollectorEvent{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: nodeish,
					},
				})
			}

			nodeEntityID := util.GenerateKubeMetadataEntityID("", "nodes", "", test.args.nodeName)
			nodeMetadata, err := s.GetKubernetesMetadata(nodeEntityID)

			assert.Equal(t, test.want.nodeMetadata, nodeMetadata)
			if test.want.err != nil {
				assert.Error(t, err, test.want.err.Error())
			}
		})
	}
}

func TestListKubernetesMetadata(t *testing.T) {
	wmeta := newWorkloadmetaObject(t)

	nodeMetadata := wmdef.KubernetesMetadata{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesMetadata,
			ID:   string(util.GenerateKubeMetadataEntityID("", "nodes", "", "node1")),
		},
		EntityMeta: wmdef.EntityMeta{
			Name:        "node1",
			Annotations: map[string]string{"a1": "v1"},
			Labels:      map[string]string{"l1": "v2"},
		},
		GVR: &schema.GroupVersionResource{
			Version:  "v1",
			Resource: "nodes",
		},
	}

	deploymentMetadata := wmdef.KubernetesMetadata{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesMetadata,
			ID:   "deployments/default/app",
		},
		EntityMeta: wmdef.EntityMeta{
			Name:        "app",
			Namespace:   "default",
			Annotations: map[string]string{"a1": "v1"},
			Labels:      map[string]string{"l1": "v2"},
		},
		GVR: &schema.GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		},
	}

	wmeta.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: &nodeMetadata,
		},
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: &deploymentMetadata,
		},
	})

	assert.ElementsMatch(t, []*wmdef.KubernetesMetadata{&nodeMetadata}, wmeta.ListKubernetesMetadata(wmdef.IsNodeMetadata))
}

func TestReset(t *testing.T) {
	fooContainer := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "foo",
		},
		EntityMeta: wmdef.EntityMeta{
			Name: "foo",
		},
		Hostname: "foo",
	}

	fooSetEvent := wmdef.CollectorEvent{
		Type:   wmdef.EventTypeSet,
		Source: fooSource,
		Entity: fooContainer,
	}

	// Same ID as fooContainer but with different values
	updatedFooContainer := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "foo",
		},
		EntityMeta: wmdef.EntityMeta{
			Name: "foo",
			Labels: map[string]string{ // Added
				"test-label": "1",
			},
		},
		Hostname: "foo",
	}

	barContainer := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "bar",
		},
		EntityMeta: wmdef.EntityMeta{
			Name: "bar",
		},
		Hostname: "bar",
	}

	tests := []struct {
		name                   string
		preEvents              []wmdef.CollectorEvent
		newEntities            []wmdef.Entity
		expectedEventsReceived []wmdef.EventBundle
	}{
		{
			name: "new entity already exists without changes",
			preEvents: []wmdef.CollectorEvent{
				fooSetEvent,
			},
			newEntities: []wmdef.Entity{
				fooContainer,
			},
			expectedEventsReceived: []wmdef.EventBundle{
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     fooContainer,
							IsComplete: true,
						},
					},
				},
			},
		},
		{
			name: "new entity exists but it has been updated",
			preEvents: []wmdef.CollectorEvent{
				fooSetEvent,
			},
			newEntities: []wmdef.Entity{
				updatedFooContainer,
			},
			expectedEventsReceived: []wmdef.EventBundle{
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     fooContainer,
							IsComplete: true,
						},
					},
				},
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     updatedFooContainer,
							IsComplete: true,
						},
					},
				},
			},
		},
		{
			name: "new event does not exist",
			preEvents: []wmdef.CollectorEvent{
				fooSetEvent,
			},
			newEntities: []wmdef.Entity{
				fooContainer,
				barContainer,
			},
			expectedEventsReceived: []wmdef.EventBundle{
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     fooContainer,
							IsComplete: true,
						},
					},
				},
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     barContainer,
							IsComplete: true,
						},
					},
				},
			},
		},
		{
			name: "an event that exists is not included in the list of new ones",
			preEvents: []wmdef.CollectorEvent{
				fooSetEvent,
			},
			newEntities: []wmdef.Entity{},
			expectedEventsReceived: []wmdef.EventBundle{
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeSet,
							Entity:     fooContainer,
							IsComplete: true,
						},
					},
				},
				{
					Events: []wmdef.Event{
						{
							Type:       wmdef.EventTypeUnset,
							Entity:     fooContainer,
							IsComplete: true,
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := newWorkloadmetaObject(t)

			s.handleEvents(test.preEvents)

			ch := s.Subscribe(dummySubscriber, wmdef.NormalPriority, nil)
			doneCh := make(chan struct{})

			var actualEventsReceived []wmdef.EventBundle
			go func() {
				for bundle := range ch {
					bundle.Acknowledge()

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
	s := newWorkloadmetaObject(t)

	container := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "123",
		},
	}

	go func() {
		_, _ = s.GetContainer("456")
	}()

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: container,
		},
	})
}

func TestPushEvents(t *testing.T) {
	wlm := newWorkloadmetaObject(t)

	mockSource := wmdef.Source("mockSource")

	tests := []struct {
		name        string
		events      []wmdef.Event
		source      wmdef.Source
		expectError bool
	}{
		{
			name:        "empty push events slice",
			events:      []wmdef.Event{},
			source:      mockSource,
			expectError: false,
		},
		{
			name: "push events with valid types",
			events: []wmdef.Event{
				{
					Type: wmdef.EventTypeSet,
				},
				{
					Type: wmdef.EventTypeUnset,
				},
				{
					Type: wmdef.EventTypeSet,
				},
			},
			source:      mockSource,
			expectError: false,
		},
		{
			name: "push events with invalid types",
			events: []wmdef.Event{
				{
					Type: wmdef.EventTypeSet,
				},
				{
					Type: wmdef.EventTypeAll,
				},
			},
			source:      mockSource,
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := wlm.Push(mockSource, test.events...)

			if test.expectError {
				assert.Error(t, err, "Expected Push operation to fail and return error")
			} else {
				assert.NoError(t, err, "Expected Push operation to succeed and return nil")
			}
		})
	}
}

func TestIsComplete_Kubernetes(t *testing.T) {
	// Enable Kubernetes and Containerd features to simulate a Kubernetes environment
	env.SetFeatures(t, env.Kubernetes, env.Containerd)

	s := newWorkloadmetaObject(t)

	container := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "test-container",
		},
	}

	pod := &wmdef.KubernetesPod{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesPod,
			ID:   "test-pod",
		},
	}

	ch := s.Subscribe(dummySubscriber, wmdef.NormalPriority, nil)
	var actual []wmdef.EventBundle

	doneCh := make(chan struct{})
	go func() {
		for bundle := range ch {
			close(bundle.Ch)
			actual = append(actual, wmdef.EventBundle{Events: bundle.Events})
		}
		close(doneCh)
	}()

	// Container reported by runtime only (incomplete in Kubernetes)
	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: wmdef.SourceRuntime,
			Entity: container,
		},
	})

	// Container also reported by kubelet (now complete)
	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: wmdef.SourceNodeOrchestrator,
			Entity: container,
		},
	})

	// Pod reported by kubelet only (incomplete)
	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: wmdef.SourceNodeOrchestrator,
			Entity: pod,
		},
	})

	// Pod also reported by kubemetadata (now complete)
	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: wmdef.SourceClusterOrchestrator,
			Entity: pod,
		},
	})

	s.Unsubscribe(ch)
	<-doneCh

	expected := []wmdef.EventBundle{
		{}, // Initial empty bundle
		{
			Events: []wmdef.Event{
				{
					Type:       wmdef.EventTypeSet,
					Entity:     container,
					IsComplete: false, // Only runtime reported, kubelet not yet
				},
			},
		},
		{
			Events: []wmdef.Event{
				{
					Type:       wmdef.EventTypeSet,
					Entity:     container,
					IsComplete: true, // Both runtime and kubelet reported
				},
			},
		},
		{
			Events: []wmdef.Event{
				{
					Type:       wmdef.EventTypeSet,
					Entity:     pod,
					IsComplete: false, // Only kubelet reported, kubemetadata not yet
				},
			},
		},
		{
			Events: []wmdef.Event{
				{
					Type:       wmdef.EventTypeSet,
					Entity:     pod,
					IsComplete: true, // Both kubelet and kubemetadata reported
				},
			},
		},
	}

	assert.Equal(t, expected, actual)
}

// This test checks completeness in Kubernetes environments when the container
// runtime is not accessible
func TestIsComplete_KubernetesContainerRuntimeNotAccessible(t *testing.T) {
	// Set Kubernetes feature, but no container runtime
	env.SetFeatures(t, env.Kubernetes)

	s := newWorkloadmetaObject(t)

	container := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "test-container",
		},
	}

	pod := &wmdef.KubernetesPod{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesPod,
			ID:   "test-pod",
		},
	}

	ch := s.Subscribe(dummySubscriber, wmdef.NormalPriority, nil)
	var actual []wmdef.EventBundle

	doneCh := make(chan struct{})
	go func() {
		for bundle := range ch {
			close(bundle.Ch)
			actual = append(actual, wmdef.EventBundle{Events: bundle.Events})
		}
		close(doneCh)
	}()

	// Container reported by kubelet (complete because container runtime not
	// accessible so will not report)
	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: wmdef.SourceNodeOrchestrator,
			Entity: container,
		},
	})

	// Pod reported by kubelet only (incomplete)
	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: wmdef.SourceNodeOrchestrator,
			Entity: pod,
		},
	})

	// Pod also reported by kubemetadata (now complete)
	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: wmdef.SourceClusterOrchestrator,
			Entity: pod,
		},
	})

	s.Unsubscribe(ch)
	<-doneCh

	expected := []wmdef.EventBundle{
		{}, // Initial empty bundle
		{
			Events: []wmdef.Event{
				{
					Type:       wmdef.EventTypeSet,
					Entity:     container,
					IsComplete: true, // Kubelet reported, container runtime not expected to report
				},
			},
		},
		{
			Events: []wmdef.Event{
				{
					Type:       wmdef.EventTypeSet,
					Entity:     pod,
					IsComplete: false, // Only kubelet reported, kubemetadata not yet
				},
			},
		},
		{
			Events: []wmdef.Event{
				{
					Type:       wmdef.EventTypeSet,
					Entity:     pod,
					IsComplete: true, // Both kubelet and kubemetadata reported
				},
			},
		},
	}

	assert.Equal(t, expected, actual)
}
