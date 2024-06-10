// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterBuilder_Build(t *testing.T) {
	dummyEntityFilterFunc := func(entity Entity) bool {
		return len(entity.GetID().ID) == 5
	}

	tests := []struct {
		name           string
		builderFunc    func() *FilterBuilder
		expectedFilter *Filter
	}{
		{
			name:        "filter with default options",
			builderFunc: NewFilterBuilder,
			expectedFilter: &Filter{
				source:    SourceAll,
				kinds:     map[Kind]GenericEntityFilterFunc{},
				eventType: EventTypeAll,
			},
		},
		{
			name: "filter with custom values",
			builderFunc: func() *FilterBuilder {
				return NewFilterBuilder().
					AddKind(KindContainer).
					AddKindWithEntityFilter(KindKubernetesPod, dummyEntityFilterFunc).
					SetSource(SourceRuntime).
					SetEventType(EventTypeSet)
			},
			expectedFilter: &Filter{
				source: SourceRuntime,
				kinds: map[Kind]GenericEntityFilterFunc{
					KindContainer:     EntityFilterFuncAcceptAll,
					KindKubernetesPod: dummyEntityFilterFunc,
				},
				eventType: EventTypeSet,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			filterBuilder := test.builderFunc()
			filter := filterBuilder.Build()
			assert.Equal(tt, filter.source, test.expectedFilter.source)
			assert.Equal(tt, filter.eventType, test.expectedFilter.eventType)

			assert.Equal(tt, len(filter.kinds), len(test.expectedFilter.kinds))
			for kind, entityFilter := range test.expectedFilter.kinds {
				expectedEntityFilter, found := filter.kinds[kind]
				assert.True(tt, found)
				assert.Equal(tt, fmt.Sprintf("%v", entityFilter), fmt.Sprintf("%v", expectedEntityFilter))
			}
		})
	}
}

func TestFilter_MatchSource(t *testing.T) {
	tests := []struct {
		name              string
		filter            *Filter
		source            Source
		expectMatchSource bool
	}{
		{
			name:              "matched due to nil filter",
			filter:            nil,
			source:            "foo",
			expectMatchSource: true,
		},
		{
			name:              "matched exact source",
			filter:            &Filter{source: "foo"},
			source:            "foo",
			expectMatchSource: true,
		},
		{
			name:              "unmatched source",
			filter:            &Filter{source: "foo"},
			source:            "bar",
			expectMatchSource: false,
		},
		{
			name:              "any source should match SourceAll",
			filter:            &Filter{source: SourceAll},
			source:            "foo",
			expectMatchSource: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			if test.expectMatchSource {
				assert.True(tt, test.filter.MatchSource(test.source))
			} else {
				assert.False(tt, test.filter.MatchSource(test.source))
			}

		})
	}
}

func TestFilter_MatchEventType(t *testing.T) {
	tests := []struct {
		name                 string
		filter               *Filter
		eventType            EventType
		expectMatchEventType bool
	}{
		{
			name:                 "any event type should match nil filter",
			filter:               nil,
			eventType:            EventTypeSet,
			expectMatchEventType: true,
		},
		{
			name:                 "matched exact event type",
			filter:               &Filter{eventType: EventTypeSet},
			eventType:            EventTypeSet,
			expectMatchEventType: true,
		},
		{
			name:                 "unmatched event type",
			filter:               &Filter{eventType: EventTypeSet},
			eventType:            EventTypeUnset,
			expectMatchEventType: false,
		},
		{
			name:                 "any event type should match EventTypeAll",
			filter:               &Filter{eventType: EventTypeAll},
			eventType:            EventTypeSet,
			expectMatchEventType: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			if test.expectMatchEventType {
				assert.True(tt, test.filter.MatchEventType(test.eventType))
			} else {
				assert.False(tt, test.filter.MatchEventType(test.eventType))
			}

		})
	}
}

func TestFilter_MatchKind(t *testing.T) {
	tests := []struct {
		name            string
		filter          *Filter
		kind            Kind
		expectMatchKind bool
	}{
		{
			name:            "any kind should match nil filter",
			filter:          nil,
			kind:            KindContainer,
			expectMatchKind: true,
		},
		{
			name:            "matched exact kind",
			filter:          &Filter{kinds: map[Kind]GenericEntityFilterFunc{KindContainer: EntityFilterFuncAcceptAll}},
			kind:            KindContainer,
			expectMatchKind: true,
		},
		{
			name:            "matched kind due to empty kinds map",
			filter:          &Filter{kinds: map[Kind]GenericEntityFilterFunc{}},
			kind:            KindContainer,
			expectMatchKind: true,
		},
		{
			name:            "matched kind due uninitialised kinds map",
			filter:          &Filter{},
			kind:            KindContainer,
			expectMatchKind: true,
		},
		{
			name:            "unmatched kind",
			filter:          &Filter{kinds: map[Kind]GenericEntityFilterFunc{KindContainer: EntityFilterFuncAcceptAll}},
			kind:            KindKubernetesPod,
			expectMatchKind: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			if test.expectMatchKind {
				assert.True(tt, test.filter.MatchKind(test.kind))
			} else {
				assert.False(tt, test.filter.MatchKind(test.kind))
			}

		})
	}
}

func TestFilter_MatchEntity(t *testing.T) {
	tests := []struct {
		name        string
		filter      *Filter
		entity      Entity
		expectMatch bool
	}{
		{
			name:   "nil filter should match any entity",
			filter: nil,
			entity: &Container{
				EntityID: EntityID{
					ID:   "cont-d",
					Kind: KindContainer,
				},
			},
			expectMatch: true,
		},
		{
			name:   "matched entity with match all entity filter func",
			filter: &Filter{kinds: map[Kind]GenericEntityFilterFunc{KindContainer: EntityFilterFuncAcceptAll}},
			entity: &Container{
				EntityID: EntityID{
					ID:   "cont-d",
					Kind: KindContainer,
				},
			},
			expectMatch: true,
		},
		{
			name:   "unmatched entity due to missing key from map",
			filter: &Filter{kinds: map[Kind]GenericEntityFilterFunc{KindKubernetesPod: EntityFilterFuncAcceptAll}},
			entity: &Container{
				EntityID: EntityID{
					ID:   "cont-d",
					Kind: KindContainer,
				},
			},
			expectMatch: false,
		},
		{
			name: "unmatched entity due to entity filter func returning false",
			filter: &Filter{kinds: map[Kind]GenericEntityFilterFunc{KindContainer: func(entity Entity) bool {
				return false
			}}},
			entity: &Container{
				EntityID: EntityID{
					ID:   "cont-d",
					Kind: KindContainer,
				},
			},
			expectMatch: false,
		},
		{
			name: "matched container entity based on container id",
			filter: &Filter{kinds: map[Kind]GenericEntityFilterFunc{KindContainer: func(entity Entity) bool {
				contEntity, ok := entity.(*Container)
				if !ok {
					return false
				}

				return contEntity.ID == "cont-d"
			}}},
			entity: &Container{
				EntityID: EntityID{
					ID:   "cont-d",
					Kind: KindContainer,
				},
			},
			expectMatch: true,
		},
		{
			name: "unmatched container entity based on container id",
			filter: &Filter{kinds: map[Kind]GenericEntityFilterFunc{KindContainer: func(entity Entity) bool {
				contEntity, ok := entity.(*Container)
				if !ok {
					return false
				}

				return contEntity.ID == "cont-a"
			}}},
			entity: &Container{
				EntityID: EntityID{
					ID:   "cont-d",
					Kind: KindContainer,
				},
			},
			expectMatch: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			if test.expectMatch {
				assert.True(tt, test.filter.MatchEntity(&test.entity))
			} else {
				assert.False(tt, test.filter.MatchEntity(&test.entity))
			}
		})
	}
}

func TestFilter_Kinds(t *testing.T) {
	tests := []struct {
		name          string
		filter        *Filter
		expectedKinds []Kind
	}{
		{
			name:          "nil filter",
			filter:        nil,
			expectedKinds: nil,
		},
		{
			name:          "filter with no kinds",
			filter:        &Filter{},
			expectedKinds: nil,
		},
		{
			name: "matched kind due uninitialised kinds map",
			filter: &Filter{kinds: map[Kind]GenericEntityFilterFunc{
				KindContainer:     EntityFilterFuncAcceptAll,
				KindKubernetesPod: EntityFilterFuncAcceptAll,
			},
			},
			expectedKinds: []Kind{KindContainer, KindKubernetesPod},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			assert.Equal(tt, len(test.filter.Kinds()), len(test.expectedKinds))
			for _, item := range test.filter.Kinds() {
				assert.Contains(tt, test.expectedKinds, item)
			}
		})
	}
}
