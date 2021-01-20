package telemetry

import "github.com/DataDog/datadog-agent/pkg/telemetry"

var (
	// StoredEntities tracks how many entities are stored in the tagger.
	StoredEntities = telemetry.NewGaugeWithOpts("tagger", "stored_entities",
		[]string{"source", "prefix"}, "Number of entities in the store.",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	// UpdatedEntities tracks the number of updates to tagger entities.
	UpdatedEntities = telemetry.NewCounterWithOpts("tagger", "updated_entities",
		[]string{}, "Number of updates made to entities.",
		telemetry.Options{NoDoubleUnderscoreSep: true})

	// Queries tracks the number of queries made against the tagger.
	Queries = telemetry.NewCounterWithOpts("tagger", "queries",
		[]string{"cardinality"}, "Queries made against the tagger.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
)
