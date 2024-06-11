package def

import (
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
)

// Concentrator accepts stats input, 'concentrating' them together into buckets before flushing them
type Concentrator interface {
	// Start starts the Concentrator
	Start()
	// Stop stops the Concentrator and attempts to flush whatever is left in the buffers
	Stop()
	// Add a stats Input to be concentrated and flushed
	//Add(t stats.Input)
	In() chan<- stats.Input
}
