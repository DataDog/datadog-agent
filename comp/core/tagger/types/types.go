// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types defines types used by the Tagger component.
package types

import (
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/utils"
)

// ApplyFunc is a generic function applied to an object of type V
type ApplyFunc[V any] func(EntityID, V)

// ObjectStore is a generic interface used as a key-value store in different tagstore implementations
// The key is of type EntityID
type ObjectStore[V any] interface {
	// Get returns an object with the specified entity ID if it exists in the store
	Get(EntityID) (V, bool)
	// Set sets a given entityID to a given object in the store
	Set(EntityID, V)
	// Unset unsets a given entityID in the store
	Unset(EntityID)
	// Size returns the total number of objects in the store
	Size() int
	// ListObjects returns a slice containing objects of the store matching the filter
	ListObjects(*Filter) []V
	// ForEach applies a given function to each object in the store matching the filter
	ForEach(*Filter, ApplyFunc[V])
}

// TaggerListResponse holds the tagger list response
type TaggerListResponse struct {
	Entities map[string]TaggerListEntity
}

// TaggerListEntity holds the tagging info about an entity
type TaggerListEntity struct {
	Tags map[string][]string `json:"tags"`
}

// TagInfo holds the tag information for a given entity and source. It's meant
// to be created from collectors and read by the store.
type TagInfo struct {
	Source               string    // source collector's name
	EntityID             EntityID  // entity id for lookup
	HighCardTags         []string  // high cardinality tags that can create a lot of different timeseries (typically one per container, user request, etc.)
	OrchestratorCardTags []string  // orchestrator cardinality tags that have as many combination as pods/tasks
	LowCardTags          []string  // low cardinality tags safe for every pipeline
	StandardTags         []string  // the discovered standard tags (env, version, service) for the entity
	DeleteEntity         bool      // true if the entity is to be deleted from the store
	ExpiryDate           time.Time // keep in cache until expiryDate
}

// CollectorPriority helps resolving dupe tags from collectors
type CollectorPriority int

// List of collector priorities
const (
	NodeRuntime CollectorPriority = iota
	NodeOrchestrator
	ClusterOrchestrator
)

// TagCardinality indicates the cardinality-level of a tag.
// It can be low cardinality (in the host count order of magnitude)
// orchestrator cardinality (tags that change value for each pod, task, etc.)
// high cardinality (typically tags that change value for each web request, each container, etc.)
type TagCardinality int

// List of possible container cardinality
const (
	LowCardinality TagCardinality = iota
	OrchestratorCardinality
	HighCardinality
)

// Entity is an entity ID + tags.
type Entity struct {
	ID                          EntityID
	HighCardinalityTags         []string
	OrchestratorCardinalityTags []string
	LowCardinalityTags          []string
	StandardTags                []string
}

// GetTags flattens all tags from all cardinalities into a single slice of tag
// strings.
func (e Entity) GetTags(cardinality TagCardinality) []string {
	tagArrays := make([][]string, 0, 3)
	tagArrays = append(tagArrays, e.LowCardinalityTags)

	switch cardinality {
	case OrchestratorCardinality:
		tagArrays = append(tagArrays, e.OrchestratorCardinalityTags)
	case HighCardinality:
		tagArrays = append(tagArrays, e.OrchestratorCardinalityTags)
		tagArrays = append(tagArrays, e.HighCardinalityTags)
	}

	return utils.ConcatenateTags(tagArrays...)
}

// Copy returns a copy of the Entity containing only tags at the supplied
// cardinality.
func (e Entity) Copy(cardinality TagCardinality) Entity {
	newEntity := e

	switch cardinality {
	case OrchestratorCardinality:
		newEntity.HighCardinalityTags = nil
	case LowCardinality:
		newEntity.HighCardinalityTags = nil
		newEntity.OrchestratorCardinalityTags = nil
	}

	return newEntity
}

const (
	// LowCardinalityString is the string representation of the low cardinality
	LowCardinalityString = "low"
	// OrchestratorCardinalityString is the string representation of the orchestrator cardinality
	OrchestratorCardinalityString = "orchestrator"
	// ShortOrchestratorCardinalityString is the short string representation of the orchestrator cardinality
	ShortOrchestratorCardinalityString = "orch"
	// HighCardinalityString is the string representation of the high cardinality
	HighCardinalityString = "high"
	// UnknownCardinalityString represents an unknown level of cardinality
	UnknownCardinalityString = "unknown"
)

// StringToTagCardinality extracts a TagCardinality from a string.
// In case of failure to parse, returns an error and defaults to Low.
func StringToTagCardinality(c string) (TagCardinality, error) {
	switch strings.ToLower(c) {
	case HighCardinalityString:
		return HighCardinality, nil
	case ShortOrchestratorCardinalityString, OrchestratorCardinalityString:
		return OrchestratorCardinality, nil
	case LowCardinalityString:
		return LowCardinality, nil
	default:
		return LowCardinality, fmt.Errorf("unsupported value %s received for tag cardinality", c)
	}
}

// TagCardinalityToString returns a string representation of a TagCardinality
// value.
func TagCardinalityToString(c TagCardinality) string {
	switch c {
	case HighCardinality:
		return HighCardinalityString
	case OrchestratorCardinality:
		return OrchestratorCardinalityString
	case LowCardinality:
		return LowCardinalityString
	default:
		return UnknownCardinalityString
	}
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

// EntityIDPrefix represents the prefix of a TagEntity id
type EntityIDPrefix string

// ToUID builds a unique id from the passed id
// if the passed id is empty, an empty string is returned
// else it returns `{entityPrefix}://{id}`
func (e EntityIDPrefix) ToUID(id string) string {
	if id == "" {
		return ""
	}
	return fmt.Sprintf("%s://%s", e, id)
}

// Subscription can be used by external subscribing components to interact with tagger events
type Subscription interface {
	// EventsChan returns a channel on which the subscriber can receive tagger events
	EventsChan() chan []EntityEvent
	// ID returns the id of the subscription
	ID() string
	// Unsubscribe is used cancel subscription to the tagger
	Unsubscribe()
}
