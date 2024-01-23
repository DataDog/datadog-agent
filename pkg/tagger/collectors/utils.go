// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

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
	panic("not called")
}

// TagCardinalityToString returns a string representation of a TagCardinality
// value.
func TagCardinalityToString(c TagCardinality) string {
	panic("not called")
}
