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

var commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}

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

	// OriginInfoRequests tracks the number of requests to the Tagger
	// to generate a container ID from Origin Info.
	OriginInfoRequests telemetry.Counter

	LowCardinalityQueries          CardinalityTelemetry
	OrchestratorCardinalityQueries CardinalityTelemetry
	HighCardinalityQueries         CardinalityTelemetry
	NoneCardinalityQueries         CardinalityTelemetry
	UnknownCardinalityQueries      CardinalityTelemetry
}

// NewStore returns a new Store.
func NewStore(telemetryComp telemetry.Component) *Store {
	// queries tracks the number of queries made against the tagger.
	queries := telemetryComp.NewCounterWithOpts(subsystem, "queries",
		[]string{"cardinality", "status"}, "Queries made against the tagger.",
		commonOpts)

	return &Store{
		StoredEntities: telemetryComp.NewGaugeWithOpts(subsystem, "stored_entities",
			[]string{"source", "prefix"}, "Number of entities in the store.",
			commonOpts),

		// UpdatedEntities tracks the number of updates to tagger entities.
		// Remote
		UpdatedEntities: telemetryComp.NewCounterWithOpts(subsystem, "updated_entities",
			[]string{}, "Number of updates made to entities.",
			commonOpts),

		// PrunedEntities tracks the number of pruned tagger entities.
		// Remote
		PrunedEntities: telemetryComp.NewGaugeWithOpts(subsystem, "pruned_entities",
			[]string{}, "Number of pruned tagger entities.",
			commonOpts),

		// ClientStreamErrors tracks how many errors were received when streaming
		// tagger events.
		// Remote
		ClientStreamErrors: telemetryComp.NewCounterWithOpts(subsystem, "client_stream_errors",
			[]string{}, "Errors received when streaming tagger events",
			commonOpts),

		// Subscribers tracks how many subscribers the tagger has.
		Subscribers: telemetryComp.NewGaugeWithOpts(subsystem, "subscribers",
			[]string{}, "Number of channels subscribing to tagger events",
			commonOpts),

		// Events tracks the number of tagger events being sent out.
		Events: telemetryComp.NewCounterWithOpts(subsystem, "events",
			[]string{"cardinality"}, "Number of tagger events being sent out",
			commonOpts),

		// Sends tracks the number of times the tagger has sent a
		// notification with a group of events.
		Sends: telemetryComp.NewCounterWithOpts(subsystem, "sends",
			[]string{}, "Number of of times the tagger has sent a notification with a group of events",
			commonOpts),

		// Receives tracks the number of times the tagger has received a
		// notification with a group of events.
		// Remote
		Receives: telemetryComp.NewCounterWithOpts(subsystem, "receives",
			[]string{}, "Number of of times the tagger has received a notification with a group of events",
			commonOpts),

		// OriginInfoRequests tracks the number of requests to the tagger
		// to generate a container ID from origin info.
		OriginInfoRequests: telemetryComp.NewCounterWithOpts(subsystem, "origin_info_requests",
			[]string{"status"}, "Number of requests to the tagger to generate a container ID from origin info.",
			commonOpts),

		LowCardinalityQueries:          newCardinalityTelemetry(queries, types.LowCardinalityString),
		OrchestratorCardinalityQueries: newCardinalityTelemetry(queries, types.OrchestratorCardinalityString),
		HighCardinalityQueries:         newCardinalityTelemetry(queries, types.HighCardinalityString),
		NoneCardinalityQueries:         newCardinalityTelemetry(queries, types.NoneCardinalityString),
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
	case types.NoneCardinality:
		return &s.NoneCardinalityQueries
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
