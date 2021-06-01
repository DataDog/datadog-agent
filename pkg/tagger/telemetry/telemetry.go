package telemetry

import "github.com/DataDog/datadog-agent/pkg/telemetry"

// PruneType represents the `prune_type` tag for the pruned_entities metric
type PruneType string

const (
	// DeletedEntity refers to deleting entities due to a deletion event
	DeletedEntity PruneType = "deletion_event"
	// EmptyEntry refers to deleting entities with empty entries
	EmptyEntry PruneType = "empty_entries"
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
		[]string{"prune_type"}, "Number of pruned tagger entities.",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	// Queries tracks the number of queries made against the tagger.
	Queries = telemetry.NewCounterWithOpts("tagger", "queries",
		[]string{"cardinality"}, "Queries made against the tagger.",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	// CacheSkipped tracks the number of times the tagger cache was skipped
	// due to partial errors.
	CacheSkipped = telemetry.NewCounterWithOpts("tagger", "cache_skipped",
		[]string{}, "Times the tagger cache was skipped due to partial errors",
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
