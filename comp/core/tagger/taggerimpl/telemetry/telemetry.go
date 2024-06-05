// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry defines the telemetry for the Tagger component.
package telemetry

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
)

const (
	subsystem = "tagger"
	// queryEmptyEntityID refers to a query made with an empty entity id
	queryEmptyEntityID = "empty_entity_id"
	// queryEmptyTags refers to a query that returned no tags
	queryEmptyTags = "empty_tags"
	// querySuccess refers to a successful query
	querySuccess = "success"
)

// CardinalityTelemetry contains the telemetry for a specific cardinality level.
type CardinalityTelemetry struct {
	EmptyEntityID telemetry.SimpleCounter
	EmptyTags     telemetry.SimpleCounter
	Success       telemetry.SimpleCounter
}

// Store contains all the telemetry for the Tagger component.
type Store struct {
	// / StoredEntities tracks how many entities are stored in the tagger.
	StoredEntities telemetry.Gauge
	// UpdatedEntities tracks the number of updates to tagger entities.
	UpdatedEntities telemetry.Counter

	// PrunedEntities tracks the number of pruned tagger entities.
	PrunedEntities telemetry.Gauge

	// ClientStreamErrors tracks how many errors were received when streaming
	// tagger events.
	ClientStreamErrors telemetry.Counter

	// ServerStreamErrors tracks how many errors happened when streaming
	// out tagger events.
	ServerStreamErrors telemetry.Counter

	// Subscribers tracks how many subscribers the tagger has.
	Subscribers telemetry.Gauge
	// Events tracks the number of tagger events being sent out.
	Events telemetry.Counter

	// Sends tracks the number of times the tagger has sent a
	// notification with a group of events.
	Sends telemetry.Counter

	// Receives tracks the number of times the tagger has received a
	// notification with a group of events.
	Receives telemetry.Counter

	LowCardinalityQueries          CardinalityTelemetry
	OrchestratorCardinalityQueries CardinalityTelemetry
	HighCardinalityQueries         CardinalityTelemetry
	UnknownCardinalityQueries      CardinalityTelemetry
}

// NewStore returns a new Store.
func NewStore(telemetryComp telemetry.Component) *Store {
	// queries tracks the number of queries made against the tagger.
	queries := telemetryComp.NewCounterWithOpts(subsystem, "queries",
		[]string{"cardinality", "status"}, "Queries made against the tagger.",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	return &Store{
		StoredEntities: telemetryComp.NewGaugeWithOpts(subsystem, "stored_entities",
			[]string{"source", "prefix"}, "Number of entities in the store.",
			telemetry.Options{NoDoubleUnderscoreSep: true}),

		// UpdatedEntities tracks the number of updates to tagger entities.
		// Remote
		UpdatedEntities: telemetryComp.NewCounterWithOpts(subsystem, "updated_entities",
			[]string{}, "Number of updates made to entities.",
			telemetry.Options{NoDoubleUnderscoreSep: true}),

		// PrunedEntities tracks the number of pruned tagger entities.
		// Remote
		PrunedEntities: telemetryComp.NewGaugeWithOpts(subsystem, "pruned_entities",
			[]string{}, "Number of pruned tagger entities.",
			telemetry.Options{NoDoubleUnderscoreSep: true}),

		// ClientStreamErrors tracks how many errors were received when streaming
		// tagger events.
		// Remote
		ClientStreamErrors: telemetryComp.NewCounterWithOpts(subsystem, "client_stream_errors",
			[]string{}, "Errors received when streaming tagger events",
			telemetry.Options{NoDoubleUnderscoreSep: true}),

		// Subscribers tracks how many subscribers the tagger has.
		Subscribers: telemetryComp.NewGaugeWithOpts(subsystem, "subscribers",
			[]string{}, "Number of channels subscribing to tagger events",
			telemetry.Options{NoDoubleUnderscoreSep: true}),

		// Events tracks the number of tagger events being sent out.
		Events: telemetryComp.NewCounterWithOpts(subsystem, "events",
			[]string{"cardinality"}, "Number of tagger events being sent out",
			telemetry.Options{NoDoubleUnderscoreSep: true}),

		// Sends tracks the number of times the tagger has sent a
		// notification with a group of events.
		Sends: telemetryComp.NewCounterWithOpts(subsystem, "sends",
			[]string{}, "Number of of times the tagger has sent a notification with a group of events",
			telemetry.Options{NoDoubleUnderscoreSep: true}),

		// Receives tracks the number of times the tagger has received a
		// notification with a group of events.
		// Remote
		Receives: telemetryComp.NewCounterWithOpts(subsystem, "receives",
			[]string{}, "Number of of times the tagger has received a notification with a group of events",
			telemetry.Options{NoDoubleUnderscoreSep: true}),

		LowCardinalityQueries:          newCardinalityTelemetry(queries, types.LowCardinalityString),
		OrchestratorCardinalityQueries: newCardinalityTelemetry(queries, types.OrchestratorCardinalityString),
		HighCardinalityQueries:         newCardinalityTelemetry(queries, types.HighCardinalityString),
		UnknownCardinalityQueries:      newCardinalityTelemetry(queries, types.UnknownCardinalityString),
	}
}

// QueriesByCardinality returns a set of counters for a given cardinality level.
func (s *Store) QueriesByCardinality(card types.TagCardinality) *CardinalityTelemetry {
	switch card {
	case types.LowCardinality:
		return &s.LowCardinalityQueries
	case types.OrchestratorCardinality:
		return &s.OrchestratorCardinalityQueries
	case types.HighCardinality:
		return &s.HighCardinalityQueries
	default:
		return &s.UnknownCardinalityQueries
	}
}

func newCardinalityTelemetry(queries telemetry.Counter, name string) CardinalityTelemetry {
	return CardinalityTelemetry{
		EmptyEntityID: queries.WithValues(name, queryEmptyEntityID),
		EmptyTags:     queries.WithValues(name, queryEmptyTags),
		Success:       queries.WithValues(name, querySuccess),
	}
}
