// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/errors"
	"gotest.tools/assert"
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
			expected: []EventBundle{},
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
			expected: []EventBundle{},
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

			assert.DeepEqual(t, tt.expected, actual)
		})
	}
}

func newTestStore() *store {
	return &store{
		store: make(map[Kind]map[string]*cachedEntity),
	}
}
