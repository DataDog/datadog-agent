// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

// EntityFilterFunc provides a filter on the entity object level
//
// Given an entity instance, it returns true if the object should be
// included in the output, and false if it should be filtered out.
type EntityFilterFunc[T Entity] func(T) bool

// GenericEntityFilterFunc is a filter function applicable to any object
// of a struct implementing the Entity interface
type GenericEntityFilterFunc EntityFilterFunc[Entity]

// EntityFilterFuncAcceptAll is an entity filter function that accepts
// any entity.
func EntityFilterFuncAcceptAll(_ Entity) bool { return true }

// IsNodeMetadata is a filter function that returns true if the metadata
// belongs to a node.
var IsNodeMetadata = func(metadata *KubernetesMetadata) bool {
	return metadata.GVR.Resource == "nodes"
}

// Filter allows a subscriber to filter events by entity kind, event source, and
// event type.
//
// A nil filter matches all events.
type Filter struct {
	kinds     map[Kind]GenericEntityFilterFunc
	source    Source
	eventType EventType
}

// FilterBuilder is used to build a filter object for subscribers.
type FilterBuilder struct {
	filter *Filter
}

// NewFilterBuilder creates and returns a new filter builder for a given
// event type for subscribing to workloadmeta events.
//
// Only events for entities with one of the added kinds and matching the
// associated entity filter function will be delivered.
//
// If no kind is added, events for entities of any kind will be delivered.
//
// Similarly, only events for entities collected from the given source will be
// delivered, and the entities in the events will contain data only from that
// source.  For example, if source is SourceRuntime, then only events from the
// runtime will be delivered, and they will not contain any additional metadata
// from orchestrators or cluster orchestrators. Use SourceAll to collect data
// from all sources. SourceAll is the default.
//
// Only events of the given type will be delivered. Use EventTypeAll to collect
// data from all the event types. EventTypeAll is the default.
func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{
		filter: &Filter{
			source:    SourceAll,
			eventType: EventTypeAll,
			kinds:     map[Kind]GenericEntityFilterFunc{},
		},
	}
}

// Build builds the filter and returns it.
func (fb *FilterBuilder) Build() *Filter {
	return fb.filter
}

// AddKind adds a specific kind to the built filter.
// The built filter will match any entity of the added kind.
func (fb *FilterBuilder) AddKind(kind Kind) *FilterBuilder {
	fb.filter.kinds[kind] = EntityFilterFuncAcceptAll
	return fb
}

// AddKindWithEntityFilter adds an entity kind with an associated entity filter function.
// The built filter will match all entities of the added kind for which the entity filter function returns true.
func (fb *FilterBuilder) AddKindWithEntityFilter(kind Kind, entityFilterFunc GenericEntityFilterFunc) *FilterBuilder {
	fb.filter.kinds[kind] = entityFilterFunc
	return fb
}

// SetSource sets the source for the filter.
func (fb *FilterBuilder) SetSource(source Source) *FilterBuilder {
	fb.filter.source = source
	return fb
}

// SetEventType sets the event type for the filter
func (fb *FilterBuilder) SetEventType(eventType EventType) *FilterBuilder {
	fb.filter.eventType = eventType
	return fb
}

// MatchEntity returns true if the filter matches the passed entity.
// If the filter is nil, or has no kinds, it always matches.
func (f *Filter) MatchEntity(entity *Entity) bool {
	if len(f.Kinds()) == 0 {
		return true
	}

	if entity == nil {
		return false
	}

	entityKind := (*entity).GetID().Kind

	if entityFilterFunc, found := f.kinds[entityKind]; found {
		// A nil filter should match
		return entityFilterFunc == nil || entityFilterFunc(*entity)
	}

	return false
}

// MatchSource returns true if the filter matches the passed source. If the
// filter is nil, or has SourceAll, it always matches.
func (f *Filter) MatchSource(source Source) bool {
	return f.Source() == SourceAll || f.Source() == source
}

// MatchEventType returns true if the filter matches the passed EventType. If
// the filter is nil, or has EventTypeAll, it always matches.
func (f *Filter) MatchEventType(eventType EventType) bool {
	return f.EventType() == EventTypeAll || f.EventType() == eventType
}

// MatchKind returns false if the filter can never match entities
// of the specified kind.
func (f *Filter) MatchKind(kind Kind) bool {
	if len(f.Kinds()) == 0 {
		return true
	}

	_, found := f.kinds[kind]

	return found
}

// Kinds returns the kinds this filter is filtering by.
func (f *Filter) Kinds() []Kind {
	if f == nil || len(f.kinds) == 0 {
		return nil
	}

	kinds := make([]Kind, 0, len(f.kinds))
	for kind := range f.kinds {
		kinds = append(kinds, kind)
	}

	return kinds
}

// Source returns the source this filter is filtering by. If the filter is nil,
// returns SourceAll.
func (f *Filter) Source() Source {
	if f == nil {
		return SourceAll
	}

	return f.source
}

// EventType returns the event type this filter is filtering by. If the filter
// is nil, it returns EventTypeAll.
func (f *Filter) EventType() EventType {
	if f == nil {
		return EventTypeAll
	}

	return f.eventType
}
