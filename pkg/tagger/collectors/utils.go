package collectors

import (
	"fmt"
	"strings"
)

const (
	lowCardinalityString               = "low"
	orchestratorCardinalityString      = "orchestrator"
	shortOrchestratorCardinalityString = "orch"
	highCardinalityString              = "high"
	unknownCardinalityString           = "unknown"
)

// StringToTagCardinality extracts a TagCardinality from a string.
// In case of failure to parse, returns an error and defaults to Low.
func StringToTagCardinality(c string) (TagCardinality, error) {
	switch strings.ToLower(c) {
	case highCardinalityString:
		return HighCardinality, nil
	case shortOrchestratorCardinalityString, orchestratorCardinalityString:
		return OrchestratorCardinality, nil
	case lowCardinalityString:
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
		return highCardinalityString
	case OrchestratorCardinality:
		return orchestratorCardinalityString
	case LowCardinality:
		return lowCardinalityString
	default:
		return unknownCardinalityString
	}
}
