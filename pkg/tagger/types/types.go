// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
)

// Entity is an entity ID + tags.
type Entity struct {
	ID                          string
	HighCardinalityTags         []string
	OrchestratorCardinalityTags []string
	LowCardinalityTags          []string
	StandardTags                []string

	hash string
}

// GetTags flattens all tags from all cardinalities into a single slice of tag
// strings.
func (e Entity) GetTags(cardinality collectors.TagCardinality) []string {
	tagArrays := make([][]string, 0, 3)
	tagArrays = append(tagArrays, e.LowCardinalityTags)

	switch cardinality {
	case collectors.OrchestratorCardinality:
		tagArrays = append(tagArrays, e.OrchestratorCardinalityTags)
	case collectors.HighCardinality:
		tagArrays = append(tagArrays, e.OrchestratorCardinalityTags)
		tagArrays = append(tagArrays, e.HighCardinalityTags)
	}

	return utils.ConcatenateTags(tagArrays...)
}

// GetHash returns a computed hash of all of the entity's tags.
func (e Entity) GetHash() string {
	if e.hash == "" {
		e.hash = utils.ComputeTagsHash(e.GetTags(collectors.HighCardinality))
	}

	return e.hash
}

// Copy returns a copy of the Entity containing only tags at the supplied
// cardinality.
func (e Entity) Copy(cardinality collectors.TagCardinality) Entity {
	newEntity := e

	switch cardinality {
	case collectors.OrchestratorCardinality:
		newEntity.HighCardinalityTags = nil
	case collectors.LowCardinality:
		newEntity.HighCardinalityTags = nil
		newEntity.OrchestratorCardinalityTags = nil
	}

	return newEntity
}

// EventType is a type of event, triggered when an entity is added, modified or
// deleted.
type EventType int

const (
	// EventTypeAdded means an entity was added.
	EventTypeAdded EventType = iota
	// EventTypeModified means an entity was modified.
	EventTypeModified
	// EventTypeDeleted means an entity was deleted.
	EventTypeDeleted
)

// EntityEvent is an event generated when an entity is added, modified or
// deleted. It contains the event type and the new entity.
type EntityEvent struct {
	EventType EventType
	Entity    Entity
}
