package telemetry

import (
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const (
	// queryEmptyEntityID refers to a query made with an empty entity id
	queryEmptyEntityID = "empty_entity_id"
	// queryEmptyTags refers to a query that returned no tags
	queryEmptyTags = "empty_tags"
	// querySuccess refers to a successful query
	querySuccess = "success"

	// fetchNotFound refers to a tagger fetch that did not find an entity
	fetchNotFound = "not_found"
	// fetchError refers to a tagger fetch that returned an error
	fetchError = "error"
	// fetchSuccess refers to a tagger fetch that was successful
	fetchSuccess = "success"
)

var (
	// StoredEntities tracks how many entities are stored in the tagger.
	StoredEntities = telemetry.NewGaugeWithOpts("tagger", "stored_entities",
		[]string{"source", "prefix"}, "Number of entities in the store.",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	// UpdatedEntities tracks the number of updates to tagger entities.
	UpdatedEntities = telemetry.NewCounterWithOpts("tagger", "updated_entities",
		[]string{}, "Number of updates made to entities.",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	// PrunedEntities tracks the number of pruned tagger entities.
	PrunedEntities = telemetry.NewGaugeWithOpts("tagger", "pruned_entities",
		[]string{}, "Number of pruned tagger entities.",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	// queries tracks the number of queries made against the tagger.
	queries = telemetry.NewCounterWithOpts("tagger", "queries",
		[]string{"cardinality", "status"}, "Queries made against the tagger.",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	// fetches tracks the number of fetches from the underlying collectors.
	fetches = telemetry.NewCounterWithOpts("tagger", "fetches",
		[]string{"collector", "status"}, "Fetches from collectors.",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	// ClientStreamErrors tracks how many errors were received when streaming
	// tagger events.
	ClientStreamErrors = telemetry.NewCounterWithOpts("tagger", "client_stream_errors",
		[]string{}, "Errors received when streaming tagger events",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	// ServerStreamErrors tracks how many errors happened when streaming
	// out tagger events.
	ServerStreamErrors = telemetry.NewCounterWithOpts("tagger", "server_stream_errors",
		[]string{}, "Errors when streaming out tagger events",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	// Subscribers tracks how many subscribers the tagger has.
	Subscribers = telemetry.NewGaugeWithOpts("tagger", "subscribers",
		[]string{}, "Number of channels subscribing to tagger events",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	// Events tracks the number of tagger events being sent out.
	Events = telemetry.NewCounterWithOpts("tagger", "events",
		[]string{"cardinality"}, "Number of tagger events being sent out",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	// Sends tracks the number of times the tagger has sent a
	// notification with a group of events.
	Sends = telemetry.NewCounterWithOpts("tagger", "sends",
		[]string{}, "Number of of times the tagger has sent a notification with a group of events",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	// Receives tracks the number of times the tagger has received a
	// notification with a group of events.
	Receives = telemetry.NewCounterWithOpts("tagger", "receives",
		[]string{}, "Number of of times the tagger has received a notification with a group of events",
		telemetry.Options{NoDoubleUnderscoreSep: true})
)

// FetcherTelemetry stores telemetry counters for a single fetcher.
type FetcherTelemetry struct {
	Success  telemetry.SimpleCounter
	NotFound telemetry.SimpleCounter
	Error    telemetry.SimpleCounter
}

// NewFetcherTelemetry returns new instance of counters for the given fetcher name.
func NewFetcherTelemetry(name string) FetcherTelemetry {
	return FetcherTelemetry{
		Success:  fetches.WithValues(name, fetchSuccess),
		NotFound: fetches.WithValues(name, fetchNotFound),
		Error:    fetches.WithValues(name, fetchError),
	}
}

// CardinalityTelemetry contains queries counters for a single cardinality level.
type CardinalityTelemetry struct {
	EmptyEntityID telemetry.SimpleCounter
	EmptyTags     telemetry.SimpleCounter
	Success       telemetry.SimpleCounter
}

// NewCardinalityTelemetry creates new set of counters for a cardinality level.
func NewCardinalityTelemetry(name string) CardinalityTelemetry {
	return CardinalityTelemetry{
		EmptyEntityID: queries.WithValues(name, queryEmptyEntityID),
		EmptyTags:     queries.WithValues(name, queryEmptyTags),
		Success:       queries.WithValues(name, querySuccess),
	}
}

var lowCardinalityQueries = NewCardinalityTelemetry(collectors.LowCardinalityString)
var orchestratorCardinalityQueries = NewCardinalityTelemetry(collectors.OrchestratorCardinalityString)
var highCardinalityQueries = NewCardinalityTelemetry(collectors.HighCardinalityString)
var unknownCardinalityQueries = NewCardinalityTelemetry(collectors.UnknownCardinalityString)

// QueriesByCardinality returns a set of counters for a given cardinality level.
func QueriesByCardinality(card collectors.TagCardinality) *CardinalityTelemetry {
	switch card {
	case collectors.LowCardinality:
		return &lowCardinalityQueries
	case collectors.OrchestratorCardinality:
		return &orchestratorCardinalityQueries
	case collectors.HighCardinality:
		return &highCardinalityQueries
	default:
		return &unknownCardinalityQueries
	}
}
