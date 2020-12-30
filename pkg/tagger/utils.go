package tagger

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
)

const (
	lowCardinalityString          = "low"
	orchestratorCardinalityString = "orchestrator"
	highCardinalityString         = "high"
	unknownCardinalityString      = "unknown"
)

// stringToTagCardinality extracts a TagCardinality from a string.
// In case of failure to parse, returns an error and defaults to Low.
func stringToTagCardinality(c string) (collectors.TagCardinality, error) {
	switch strings.ToLower(c) {
	case highCardinalityString:
		return collectors.HighCardinality, nil
	case orchestratorCardinalityString:
		return collectors.OrchestratorCardinality, nil
	case lowCardinalityString:
		return collectors.LowCardinality, nil
	default:
		return collectors.LowCardinality, fmt.Errorf("unsupported value %s received for tag cardinality", c)
	}
}

// tagCardinalityToString returns a string representation of a TagCardinality
// value.
func tagCardinalityToString(c collectors.TagCardinality) string {
	switch c {
	case collectors.HighCardinality:
		return highCardinalityString
	case collectors.OrchestratorCardinality:
		return orchestratorCardinalityString
	case collectors.LowCardinality:
		return lowCardinalityString
	default:
		return unknownCardinalityString
	}
}
